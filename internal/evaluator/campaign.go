package evaluator

import (
	"context"
	"log/slog"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// RunCampaign executes one eval run per suite under the given campaign
// against the given models. Every run is created up front — the progress
// grid shows the full batch shape from the first cell on — and execution
// fans out as (run × model) cells through the bounded worker pool (GH #26):
// cells run concurrently up to the configured eval_concurrency, cases stay
// serial inside a cell, and a failed suite or model never blocks the
// remaining ones. When every cell has executed (or the context was
// canceled) the campaign's aggregate status is settled once from its member
// runs, firing the AfterCampaign hook on a done campaign. The campaign
// trigger is stamped onto every run so runs and their campaign always agree
// on provenance.
func (e *Evaluator) RunCampaign(ctx context.Context, campaignID int64, trigger string, suites []store.Suite, modelDBIDs []int64, judgeModel string) {
	// Registered for operator cancel (GH #152); unregistered when the
	// campaign's execution (including settle) returns.
	ctx = e.registerCancel(ctx, campaignID)
	defer e.unregisterCancel(campaignID)

	// Zero suites in the rotation (the cutover's empty-rotation window,
	// ticket 99 risk 3): degrade to an empty done batch instead of letting
	// the aggregate rule settle the campaign as failed.
	if len(suites) == 0 {
		slog.Warn("evaluator: no suites in the evaluation rotation, settling an empty batch", "campaign_id", campaignID)
		if err := e.db.SettleEmptyCampaign(campaignID, time.Now().UTC()); err != nil {
			slog.Error("evaluator: settle empty campaign", "campaign_id", campaignID, "error", err)
		}
		return
	}
	var prepared []*preparedRun
	for _, suite := range suites {
		if ctx.Err() != nil {
			break
		}
		run, err := e.db.CreateEvalRun(campaignID, suite.ID, trigger, judgeModel)
		if err != nil {
			slog.Error("evaluator: create campaign run", "campaign_id", campaignID, "suite_id", suite.ID, "error", err)
			continue
		}
		prep, err := e.prepareRun(run.ID, len(modelDBIDs))
		if err != nil {
			// prepareRun already marked the run failed; the campaign settles
			// from its member runs either way.
			slog.Error("evaluator: prepare campaign run", "campaign_id", campaignID, "suite_id", suite.ID, "error", err)
			continue
		}
		prepared = append(prepared, prep)
	}
	e.executePrepared(ctx, prepared, modelDBIDs)
	e.SettleCampaign(ctx, campaignID)
}
