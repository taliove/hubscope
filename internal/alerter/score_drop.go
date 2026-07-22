package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/taliove2009/hubscope/internal/store"
)

// ScoreDropThreshold is how far a (suite, model) aggregate score must fall
// versus the previous done run before a score_drop alert fires.
const ScoreDropThreshold = 0.2

// pairScore accumulates one model's scores inside a run.
type pairScore struct {
	modelID string
	sum     float64
	count   int
}

// HandleEvalRun compares every (suite, model) aggregate of a finished eval
// run against the previous done run for the same pair and alerts on drops
// beyond ScoreDropThreshold. It is hooked into evaluator.AfterRun.
//
// Alerting rules differ from down/recovered: every large drop fires (there
// is no "still dropped" suppression across runs), but a given run alerts at
// most once per pair because this method runs exactly once per run. The
// score_drop_alert_enabled setting is re-read on every invocation, so toggles
// take effect immediately. Nothing is recorded while the webhook is
// unconfigured or the switch is off — mirroring probe-alert semantics.
func (e *Evaluator) HandleEvalRun(ctx context.Context, runID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, err := e.db.GetEvalRun(runID)
	if err != nil {
		slog.Error("alerter: load eval run for score-drop check", "run_id", runID, "error", err)
		return
	}
	if run.Status != "done" {
		return
	}

	enabled, err := e.db.GetSettingBool(store.SettingScoreDropAlertEnabled, store.DefaultScoreDropAlertEnabled)
	if err != nil {
		slog.Error("alerter: read score_drop_alert_enabled setting", "error", err)
		return
	}
	if !enabled {
		return
	}
	webhook, err := e.db.GetSetting(store.SettingLarkWebhookURL, "")
	if err != nil {
		slog.Error("alerter: read webhook setting", "error", err)
		return
	}
	if webhook == "" {
		slog.Debug("alerter: score-drop check skipped (webhook not configured)", "run_id", runID)
		return
	}

	suite, err := e.db.GetSuite(run.SuiteID)
	if err != nil {
		slog.Error("alerter: load suite for score-drop check", "suite_id", run.SuiteID, "error", err)
		return
	}

	results, err := e.db.ListEvalResults(runID)
	if err != nil {
		slog.Error("alerter: load results for score-drop check", "run_id", runID, "error", err)
		return
	}

	for _, modelDBID := range sortedPairIDs(results) {
		modelID, current := aggregate(results, modelDBID)
		e.checkPair(ctx, webhook, run, suite, modelDBID, modelID, current)
	}
}

// sortedPairIDs returns the distinct model database IDs covered by a run's
// results, sorted for deterministic alert order.
func sortedPairIDs(results []store.EvalResult) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	for _, r := range results {
		if !seen[r.ModelDBID] {
			seen[r.ModelDBID] = true
			ids = append(ids, r.ModelDBID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// aggregate averages the non-null scores of one model inside the given
// results. The score is nil when the model scored nothing (all results
// unjudged), matching the read-time run aggregation.
func aggregate(results []store.EvalResult, modelDBID int64) (string, *float64) {
	p := pairScore{}
	for _, r := range results {
		if r.ModelDBID != modelDBID {
			continue
		}
		p.modelID = r.ModelID
		if r.Score != nil {
			p.sum += *r.Score
			p.count++
		}
	}
	if p.count == 0 {
		return p.modelID, nil
	}
	avg := p.sum / float64(p.count)
	return p.modelID, &avg
}

// checkPair alerts when one model's aggregate dropped beyond the threshold
// versus the previous done run for the same (suite, model) pair.
func (e *Evaluator) checkPair(ctx context.Context, webhook string, run *store.EvalRun, suite *store.Suite, modelDBID int64, modelID string, current *float64) {
	if current == nil {
		return
	}
	previous, _, err := e.db.PreviousDoneScore(run.SuiteID, modelDBID, run.ID)
	if err != nil {
		slog.Error("alerter: load previous score", "suite_id", run.SuiteID, "model_db_id", modelDBID, "error", err)
		return
	}
	if previous == nil || *previous-*current <= ScoreDropThreshold {
		return
	}

	message := fmt.Sprintf(
		"【HubScope】评估分数大跌:模型 %s 在评估集「%s」的得分由 %.2f 跌至 %.2f(下跌 %.2f,超过阈值 %.1f)。",
		modelID, suite.Name, *previous, *current, *previous-*current, ScoreDropThreshold)

	sentOK := true
	if err := e.sender.Send(ctx, webhook, message); err != nil {
		slog.Error("alerter: send score_drop alert", "run_id", run.ID, "model_db_id", modelDBID, "error", err)
		sentOK = false
	}
	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindScoreDrop,
		Message: message,
		SentOK:  sentOK,
	}); err != nil {
		slog.Error("alerter: record score_drop event", "run_id", run.ID, "model_db_id", modelDBID, "error", err)
	}
}
