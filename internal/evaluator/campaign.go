package evaluator

import (
	"context"
	"log/slog"

	"github.com/taliove/hubscope/internal/store"
)

// RunCampaign executes one eval run per suite under the given campaign,
// sequentially, against the given models. Sequential execution keeps hub
// load predictable (the same cadence the weekly batch always used) and a
// failed suite never blocks the remaining ones. When the loop ends — all
// suites attempted or the context canceled — the campaign's aggregate status
// is settled from its member runs (firing the AfterCampaign hook on a done
// campaign). The campaign trigger is stamped onto every run so runs and
// their campaign always agree on provenance.
func (e *Evaluator) RunCampaign(ctx context.Context, campaignID int64, trigger string, suites []store.Suite, modelDBIDs []int64, judgeModel string) {
	for _, suite := range suites {
		if ctx.Err() != nil {
			break
		}
		run, err := e.db.CreateEvalRun(campaignID, suite.ID, trigger, judgeModel)
		if err != nil {
			slog.Error("evaluator: create campaign run", "campaign_id", campaignID, "suite_id", suite.ID, "error", err)
			continue
		}
		if err := e.RunEval(ctx, run.ID, modelDBIDs); err != nil {
			slog.Error("evaluator: run campaign suite", "campaign_id", campaignID, "suite_id", suite.ID, "error", err)
		}
	}
	e.SettleCampaign(ctx, campaignID)
}
