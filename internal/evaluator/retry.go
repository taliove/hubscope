package evaluator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// RetryFailedResults re-evaluates every failed (null-score) result of a
// campaign (GH #28) — the unrestricted form of retryNullScoreCells.
func (e *Evaluator) RetryFailedResults(ctx context.Context, campaignID int64) {
	e.retryNullScoreCells(ctx, campaignID, nil)
}

// RetryUnits re-evaluates an explicit set of (model, case) units of a
// campaign (retry-units, the targeted sibling of GH #28's retry-failed).
// The caller reopens the campaign with ReopenCampaignForUnitRetry before
// invoking it — only runs holding at least one requested null-score unit
// rejoin execution — and this method drives those runs' requested failed
// cells through the same bounded pool, run finishing and settle path as a
// batch retry. Units whose score is no longer null by execution time simply
// match no null row and produce no cell, so a judged result is never
// re-asked (W7); units the campaign never recorded behave the same way.
func (e *Evaluator) RetryUnits(ctx context.Context, campaignID int64, units []store.RetryUnit) {
	set := make(map[store.RetryUnit]struct{}, len(units))
	for _, u := range units {
		set[u] = struct{}{}
	}
	e.retryNullScoreCells(ctx, campaignID, set)
}

// retryNullScoreCells is the shared retry driver behind RetryFailedResults
// (units == nil: every null-score unit) and RetryUnits (an explicit unit
// set). The caller reopens the campaign to running before invoking it —
// reopening also migrates every run holding a retrievable null-score result
// back to running (GH #39) — and this method fans the failed (run × model)
// cells out through the same bounded pool as a normal batch, finishes each
// reopened run when its cells complete, and settles the campaign once — so
// the AfterCampaign hook fires exactly once per retry-settle (a second firing
// overall is the registered, accepted consequence of re-settling a batch).
//
// Run finishing mirrors the normal executor's semantics: a run whose cells
// all executed finishes done regardless of how many retried cases failed
// again (a null score is a result, never a run failure), and a run whose
// cells were dropped by context cancellation finishes failed. Runs without
// retrievable failed results were left untouched by the reopen and stay
// untouched here.
//
// Only null-score units are touched: each case's old null row is deleted
// right before its fresh evaluation (DeleteNullScoreResult hardcodes
// score IS NULL), and a case whose retry fails again simply lands as a new
// null row. Scored results are never read for rewriting, let alone modified
// (W7). Cases not yet attempted keep their null rows, so an interrupted
// retry stays retryable.
func (e *Evaluator) retryNullScoreCells(ctx context.Context, campaignID int64, units map[store.RetryUnit]struct{}) {
	// Registered for operator cancel (GH #152), same as a fresh batch.
	ctx = e.registerCancel(ctx, campaignID)
	defer e.unregisterCancel(campaignID)

	runs, err := e.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		slog.Error("evaluator: load campaign runs for retry", "campaign_id", campaignID, "error", err)
		return
	}
	defaultSamples := e.resolveDefaultSampleCount()

	var mu sync.Mutex
	remaining := map[int64]int{}
	var cells []evalCell
	var preps []*preparedRun
	for i := range runs {
		run := runs[i]
		nulls, err := e.db.ListNullScoreCells(run.ID)
		if err != nil {
			// The run cannot even be inspected. Fail it only when the reopen
			// actually migrated it (migrated runs read back as running): a
			// migrated-but-uninspectable run must not strand the campaign
			// running, so failed stays visible and retryable. Stamping a
			// clean done run would mislabel it — the very class of error
			// this fix exists to remove (GH #39 check MEDIUM-1).
			slog.Error("evaluator: list null-score cells", "run_id", run.ID, "error", err)
			if run.Status == "running" {
				e.finishRetriedRun(run.ID, "failed")
			}
			continue
		}
		if units != nil {
			// Targeted retry: restrict to the requested units. A run holding
			// none of them was left untouched by the reopen (its EXISTS guard
			// uses the same unit set) and stays untouched here.
			requested := make([]store.NullScoreCell, 0, len(nulls))
			for _, n := range nulls {
				if _, ok := units[store.RetryUnit{ModelDBID: n.ModelDBID, CaseID: n.CaseID}]; ok {
					requested = append(requested, n)
				}
			}
			nulls = requested
		}
		if len(nulls) == 0 {
			// The reopen left this run untouched (GH #39): nothing to
			// re-evaluate, nothing to finish.
			continue
		}
		// Group the failed cases by model, preserving first-seen order.
		byModel := map[int64][]store.Case{}
		var order []int64
		for _, n := range nulls {
			c, err := e.db.GetCase(n.CaseID)
			if err != nil {
				// The case row is gone (purged): its null result stays as
				// history — retrying it is impossible by construction.
				slog.Error("evaluator: retry skips missing case", "case_id", n.CaseID, "error", err)
				continue
			}
			// Disabled since the original run (GH #154): out of the
			// rotation — its null result stays as history, like a purged
			// case.
			if !c.Enabled {
				slog.Info("evaluator: retry skips disabled case", "case_id", n.CaseID)
				continue
			}
			if _, seen := byModel[n.ModelDBID]; !seen {
				order = append(order, n.ModelDBID)
			}
			byModel[n.ModelDBID] = append(byModel[n.ModelDBID], *c)
		}
		if len(order) == 0 {
			// Every failed case is gone: nothing can be re-evaluated, so the
			// reopened run finishes done immediately — its null rows stay as
			// history (W7) and failed_results keeps reporting them.
			e.finishRetriedRun(run.ID, "done")
			continue
		}
		for _, modelDBID := range order {
			// Retry cells carry no task-center mirror: the run's original
			// task already settled, and the campaign's progress grid is the
			// observable surface of a retry.
			prep := &preparedRun{run: &run, cases: byModel[modelDBID]}
			preps = append(preps, prep)
			cells = append(cells, evalCell{prep: prep, modelDBID: modelDBID})
			remaining[run.ID]++
		}
	}

	p := newPipeline(e, preps)
	p.finishRun = func(prep *preparedRun) { e.finishRetriedRun(prep.run.ID, "done") }
	p.runJudgePool(ctx, nil)
	e.runCellPool(ctx, cells, nil, func(cctx context.Context, cell evalCell) {
		e.retryModel(cctx, p, cell.prep, cell.modelDBID, cell.prep.cases, defaultSamples)
		mu.Lock()
		remaining[cell.prep.run.ID]--
		done := remaining[cell.prep.run.ID] == 0
		mu.Unlock()
		if done {
			// All of the run's retried cells executed: the run finishes
			// once its judge queue is also empty (GH #176).
			p.markCellsDone(cell.prep.run.ID)
		}
	})
	p.closeJudgeQueue()

	// Cells dropped because the context was canceled never reported back;
	// their runs fail, matching the normal executor's cancellation outcome.
	// Runs whose judge queue could not drain fail the same way. Cases they
	// never reached keep their null rows and stay retryable.
	for runID := range remaining {
		if !p.isFinished(runID) {
			e.finishRetriedRun(runID, "failed")
		}
	}
	e.SettleCampaign(ctx, campaignID)
}

// RetryAnsweredUnits re-answers and re-judges the requested units that
// already hold a result row (2026-08-04 ruling): the operator picked them
// explicitly, so any score — not just nulls — is re-asked. settled=true
// drives the reopen → re-settle path of a finished batch; settled=false
// means the campaign is executing on this process and the retry's fresh
// results simply land into the in-flight batch (no reopen, no settle, no
// run finishing — the batch owns its lifecycle).
func (e *Evaluator) RetryAnsweredUnits(ctx context.Context, campaignID int64, units []store.RetryUnit, settled bool) {
	ctx = e.registerCancel(ctx, campaignID)
	defer e.unregisterCancel(campaignID)

	runs, err := e.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		slog.Error("evaluator: load campaign runs for retry-any", "campaign_id", campaignID, "error", err)
		return
	}
	runBySuite := map[int64]*store.EvalRun{}
	for i := range runs {
		runBySuite[runs[i].SuiteID] = &runs[i]
	}
	defaultSamples := e.resolveDefaultSampleCount()

	type cellKey struct {
		run   *store.EvalRun
		model int64
	}
	byCell := map[cellKey][]store.Case{}
	var order []cellKey
	for _, u := range units {
		c, err := e.db.GetCase(u.CaseID)
		if err != nil {
			slog.Error("evaluator: retry-any skips missing case", "case_id", u.CaseID, "error", err)
			continue
		}
		if !c.Enabled {
			slog.Info("evaluator: retry-any skips disabled case", "case_id", u.CaseID)
			continue
		}
		run := runBySuite[c.SuiteID]
		if run == nil {
			slog.Error("evaluator: retry-any case outside the campaign", "case_id", u.CaseID)
			continue
		}
		key := cellKey{run: run, model: u.ModelDBID}
		if _, seen := byCell[key]; !seen {
			order = append(order, key)
		}
		byCell[key] = append(byCell[key], *c)
	}

	var cells []evalCell
	var preps []*preparedRun
	for _, key := range order {
		prep := &preparedRun{run: key.run, cases: byCell[key]}
		preps = append(preps, prep)
		cells = append(cells, evalCell{prep: prep, modelDBID: key.model})
	}

	p := newPipeline(e, preps)
	if settled {
		p.finishRun = func(prep *preparedRun) { e.finishRetriedRun(prep.run.ID, "done") }
	} else {
		// The in-flight batch owns its runs' lifecycle; the retry only
		// lands fresh results.
		p.finishRun = func(*preparedRun) {}
	}
	p.runJudgePool(ctx, nil)
	e.runCellPool(ctx, cells, nil, func(cctx context.Context, cell evalCell) {
		e.retryModelAny(cctx, p, cell.prep, cell.modelDBID, cell.prep.cases, defaultSamples)
		p.markCellsDone(cell.prep.run.ID)
	})
	p.closeJudgeQueue()

	if settled {
		for _, prep := range preps {
			if !p.isFinished(prep.run.ID) {
				e.finishRetriedRun(prep.run.ID, "failed")
			}
		}
		e.SettleCampaign(ctx, campaignID)
	}
}

// retryModelAny re-evaluates one (run, model) cell's requested cases,
// deleting each unit's prior result regardless of score (2026-08-04
// ruling). Same setup guards as retryModel.
func (e *Evaluator) retryModelAny(ctx context.Context, p *pipeline, prep *preparedRun, modelDBID int64, cases []store.Case, defaultSamples int) {
	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		slog.Error("evaluator: retry-any skips model", "model_db_id", modelDBID, "error", err)
		return
	}
	if model.Status == "retired" {
		slog.Info("evaluator: retry-any skips retired model", "model", model.ModelID)
		return
	}
	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		slog.Error("evaluator: retry-any skips model, hub gone", "model", model.ModelID, "error", err)
		return
	}
	endpoints, err := e.db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		slog.Error("evaluator: retry-any skips model, endpoints unloadable", "model", model.ModelID, "error", err)
		return
	}
	protocol, ok := selectProtocol(endpoints)
	if !ok {
		slog.Error("evaluator: retry-any skips model, no enabled endpoint", "model", model.ModelID)
		return
	}

	for _, c := range cases {
		if ctx.Err() != nil {
			return
		}
		if err := e.db.DeleteUnitResult(prep.run.ID, modelDBID, c.ID); err != nil {
			slog.Error("evaluator: delete result before retry-any", "run_id", prep.run.ID,
				"model_db_id", modelDBID, "case_id", c.ID, "error", err)
			continue
		}
		e.evalCase(ctx, p, prep, hub, protocol, model, c, nil, defaultSamples)
	}
}

// ResumeCampaign restarts an interrupted batch where it stopped (2026-08-05
// ruling): answered units keep their results and scores; only missing
// units (the interruption never asked them) and null-score units re-run.
// The original jury snapshot rides along (judgesFor reads it off the run),
// and the confirmation gate is not re-engaged — the operator reviewed this
// plan already.
func (e *Evaluator) ResumeCampaign(ctx context.Context, campaignID int64) {
	ctx = e.registerCancel(ctx, campaignID)
	defer e.unregisterCancel(campaignID)

	runs, err := e.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		slog.Error("evaluator: load campaign runs for resume", "campaign_id", campaignID, "error", err)
		return
	}
	members, err := e.db.ListCampaignMembers(campaignID)
	if err != nil {
		slog.Error("evaluator: load campaign members for resume", "campaign_id", campaignID, "error", err)
		return
	}
	defaultSamples := e.resolveDefaultSampleCount()

	var cells []evalCell
	var preps []*preparedRun
	reopenSet := map[int64]bool{}
	var reopenRuns []int64
	for i := range runs {
		run := &runs[i]
		cases, err := e.db.ListEnabledCases(run.SuiteID)
		if err != nil {
			slog.Error("evaluator: list cases for resume", "run_id", run.ID, "error", err)
			continue
		}
		existing := map[int64]map[int64]*float64{} // model → case → score presence
		results, err := e.db.ListEvalResults(run.ID)
		if err != nil {
			slog.Error("evaluator: list results for resume", "run_id", run.ID, "error", err)
			continue
		}
		for _, r := range results {
			if existing[r.ModelDBID] == nil {
				existing[r.ModelDBID] = map[int64]*float64{}
			}
			existing[r.ModelDBID][r.CaseID] = r.Score
		}
		for _, m := range members {
			var todo []store.Case
			for _, c := range cases {
				score, done := existing[m.ModelDBID][c.ID]
				if !done || score == nil {
					todo = append(todo, c)
				}
			}
			if len(todo) == 0 {
				continue
			}
			prep := &preparedRun{run: run, cases: todo}
			preps = append(preps, prep)
			cells = append(cells, evalCell{prep: prep, modelDBID: m.ModelDBID})
			if !reopenSet[run.ID] {
				reopenSet[run.ID] = true
				reopenRuns = append(reopenRuns, run.ID)
			}
		}
	}
	if len(cells) == 0 {
		// Nothing incomplete: a settled batch stays settled; the caller's
		// reopen was never invoked, so no state moved at all.
		return
	}

	reopened, err := e.db.ReopenCampaignRuns(campaignID, reopenRuns)
	if err != nil {
		slog.Error("evaluator: reopen for resume", "campaign_id", campaignID, "error", err)
		return
	}
	if !reopened {
		slog.Info("evaluator: resume lost the settle-state race", "campaign_id", campaignID)
		return
	}

	p := newPipeline(e, preps)
	p.finishRun = func(prep *preparedRun) { e.finishRetriedRun(prep.run.ID, "done") }
	p.runJudgePool(ctx, nil)
	e.runCellPool(ctx, cells, nil, func(cctx context.Context, cell evalCell) {
		e.retryModelAny(cctx, p, cell.prep, cell.modelDBID, cell.prep.cases, defaultSamples)
		p.markCellsDone(cell.prep.run.ID)
	})
	p.closeJudgeQueue()

	for _, prep := range preps {
		if !p.isFinished(prep.run.ID) {
			e.finishRetriedRun(prep.run.ID, "failed")
		}
	}
	e.SettleCampaign(ctx, campaignID)
}

// finishRetriedRun stamps the retry's terminal status onto a reopened run
// (GH #39). Retry runs carry no task-center mirror — the original run's task
// settled with it — so this is a plain store transition.
func (e *Evaluator) finishRetriedRun(runID int64, status string) {
	if err := e.db.FinishEvalRun(runID, status, time.Now().UTC()); err != nil {
		slog.Error("evaluator: finish retried run", "run_id", runID, "status", status, "error", err)
	}
}

// retryModel re-evaluates the failed cases of one (run, model) cell. A
// model whose setup no longer resolves (deleted, hub gone, no enabled
// endpoint) keeps its existing null rows — the grid stays complete and the
// failure stays retryable — rather than gaining duplicate failure rows.
func (e *Evaluator) retryModel(ctx context.Context, p *pipeline, prep *preparedRun, modelDBID int64, cases []store.Case, defaultSamples int) {
	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		slog.Error("evaluator: retry skips model", "model_db_id", modelDBID, "error", err)
		return
	}
	// Retired since the original run (GH #154): keep the null rows as
	// history — re-asking a retired model is pure waste.
	if model.Status == "retired" {
		slog.Info("evaluator: retry skips retired model", "model", model.ModelID)
		return
	}
	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		slog.Error("evaluator: retry skips model, hub gone", "model", model.ModelID, "error", err)
		return
	}
	endpoints, err := e.db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		slog.Error("evaluator: retry skips model, endpoints unloadable", "model", model.ModelID, "error", err)
		return
	}
	protocol, ok := selectProtocol(endpoints)
	if !ok {
		slog.Error("evaluator: retry skips model, no enabled endpoint", "model", model.ModelID)
		return
	}

	for _, c := range cases {
		if ctx.Err() != nil {
			return
		}
		if err := e.db.DeleteNullScoreResult(prep.run.ID, modelDBID, c.ID); err != nil {
			// Keep the old row rather than risk a duplicate result for the
			// same (run, model, case) unit.
			slog.Error("evaluator: delete null result before retry", "run_id", prep.run.ID,
				"model_db_id", modelDBID, "case_id", c.ID, "error", err)
			continue
		}
		e.evalCase(ctx, p, prep, hub, protocol, model, c, nil, defaultSamples)
	}
}
