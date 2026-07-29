package evaluator

import (
	"context"
	"log/slog"

	"github.com/taliove/hubscope/internal/store"
)

// RetryFailedResults re-evaluates every failed (null-score) result of a
// campaign (GH #28). The caller reopens the campaign to running before
// invoking it; this method fans the failed (run × model) cells out through
// the same bounded pool as a normal batch and settles the campaign once when
// they finish — so the AfterCampaign hook fires exactly once per
// retry-settle (a second firing overall is the registered, accepted
// consequence of re-settling a batch).
//
// Only null-score units are touched: each case's old null row is deleted
// right before its fresh evaluation (DeleteNullScoreResult hardcodes
// score IS NULL), and a case whose retry fails again simply lands as a new
// null row. Scored results are never read for rewriting, let alone modified
// (W7). Cases not yet attempted keep their null rows, so an interrupted
// retry stays retryable.
func (e *Evaluator) RetryFailedResults(ctx context.Context, campaignID int64) {
	runs, err := e.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		slog.Error("evaluator: load campaign runs for retry", "campaign_id", campaignID, "error", err)
		return
	}
	defaultSamples := e.resolveDefaultSampleCount()

	var cells []evalCell
	for i := range runs {
		run := runs[i]
		nulls, err := e.db.ListNullScoreCells(run.ID)
		if err != nil {
			slog.Error("evaluator: list null-score cells", "run_id", run.ID, "error", err)
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
		for _, modelDBID := range order {
			// Retry cells carry no task-center mirror: the run's original
			// task already settled, and the campaign's progress grid is the
			// observable surface of a retry.
			prep := &preparedRun{run: &run, cases: byModel[modelDBID]}
			cells = append(cells, evalCell{prep: prep, modelDBID: modelDBID})
		}
	}

	e.runCellPool(ctx, cells, func(cctx context.Context, cell evalCell) {
		e.retryModel(cctx, cell.prep.run, cell.modelDBID, cell.prep.cases, defaultSamples)
	})
	e.SettleCampaign(ctx, campaignID)
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
