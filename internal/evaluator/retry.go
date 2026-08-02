package evaluator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// RetryFailedResults re-evaluates every failed (null-score) result of a
// campaign (GH #28). The caller reopens the campaign to running before
// invoking it — reopening also migrates every run holding null-score results
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
// failed results were left untouched by the reopen and stay untouched here.
//
// Only null-score units are touched: each case's old null row is deleted
// right before its fresh evaluation (DeleteNullScoreResult hardcodes
// score IS NULL), and a case whose retry fails again simply lands as a new
// null row. Scored results are never read for rewriting, let alone modified
// (W7). Cases not yet attempted keep their null rows, so an interrupted
// retry stays retryable.
func (e *Evaluator) RetryFailedResults(ctx context.Context, campaignID int64) {
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
		if len(nulls) == 0 {
			// ReopenCampaignForRetry left this run untouched (GH #39):
			// nothing to re-evaluate, nothing to finish.
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
			cells = append(cells, evalCell{prep: prep, modelDBID: modelDBID})
			remaining[run.ID]++
		}
	}

	e.runCellPool(ctx, cells, nil, func(cctx context.Context, cell evalCell) {
		e.retryModel(cctx, cell.prep.run, cell.modelDBID, cell.prep.cases, defaultSamples)
		mu.Lock()
		remaining[cell.prep.run.ID]--
		done := remaining[cell.prep.run.ID] == 0
		mu.Unlock()
		if done {
			// All of the run's retried cells executed: the run is done
			// regardless of a later cancellation (the normal executor's
			// semantics, GH #26).
			e.finishRetriedRun(cell.prep.run.ID, "done")
		}
	})

	// Cells dropped because the context was canceled never reported back;
	// their runs fail, matching the normal executor's cancellation outcome.
	// Cases they never reached keep their null rows and stay retryable.
	mu.Lock()
	for runID, left := range remaining {
		if left > 0 {
			e.finishRetriedRun(runID, "failed")
		}
	}
	mu.Unlock()
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
func (e *Evaluator) retryModel(ctx context.Context, run *store.EvalRun, modelDBID int64, cases []store.Case, defaultSamples int) {
	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		slog.Error("evaluator: retry skips model", "model_db_id", modelDBID, "error", err)
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
		if err := e.db.DeleteNullScoreResult(run.ID, modelDBID, c.ID); err != nil {
			// Keep the old row rather than risk a duplicate result for the
			// same (run, model, case) unit.
			slog.Error("evaluator: delete null result before retry", "run_id", run.ID,
				"model_db_id", modelDBID, "case_id", c.ID, "error", err)
			continue
		}
		e.evalCase(ctx, run, hub, protocol, model, c, nil, defaultSamples)
	}
}
