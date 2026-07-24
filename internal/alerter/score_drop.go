package alerter

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// ScoreDropThreshold is how far a (suite, model) aggregate score must fall
// versus the previous done campaign before a score_drop alert fires. The
// same bar marks a single case as a big drop inside the alert detail.
const ScoreDropThreshold = 0.2

// maxCaseChangesPerSuite caps the per-case change lines inside one suite
// section of an alert message; the remainder is summarized as a count.
const maxCaseChangesPerSuite = 5

// promptSnippetRunes bounds the case prompt excerpt quoted in case lines.
const promptSnippetRunes = 16

// caseChange is one case whose outcome materially changed between the two
// compared runs: it went from scored to unjudged, or its score fell beyond
// ScoreDropThreshold. Previous is never nil (a change requires a baseline).
type caseChange struct {
	caseID   int64
	prompt   string
	previous float64
	current  *float64
}

// suiteDrop is one suite's aggregate drop for one model, with the
// case-level changes that explain it.
type suiteDrop struct {
	suiteName string
	previous  float64
	current   float64
	changes   []caseChange
}

// modelAlert accumulates every suite drop of one model inside a campaign.
type modelAlert struct {
	modelID string
	drops   []suiteDrop
}

// HandleCampaign compares a settled campaign against the previous done
// campaign, suite by suite: for every (suite, model) pair whose aggregate
// fell beyond ScoreDropThreshold, one consolidated message per model lists
// each suite's drop plus the per-case changes behind it. It is hooked into
// evaluator.AfterCampaign and runs exactly once per done campaign.
//
// Comparison rules:
//   - only done campaigns serve as baselines (a failed campaign's partial
//     results never anchor a comparison);
//   - when the two runs ran different suite versions, the pair is skipped —
//     the questions changed, so the scores are not comparable — and the skip
//     is annotated as a score_drop_skipped alert event plus a warn line on
//     the run's task log;
//   - pairs without a baseline (first campaign covering the suite) or with
//     nothing scored in the current run are skipped silently.
//
// Debounce semantics mirror the probe alerter: the score_drop_alert_enabled
// switch and the webhook are re-read on every invocation, nothing is
// recorded while either is off, and a failed send still records the event
// (sent_ok=false) so it is not retried.
func (e *Evaluator) HandleCampaign(ctx context.Context, campaignID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	campaign, err := e.db.GetCampaign(campaignID)
	if err != nil {
		slog.Error("alerter: load campaign for score-drop check", "campaign_id", campaignID, "error", err)
		return
	}
	if campaign.Status != store.CampaignStatusDone {
		// A partially failed or aborted campaign never alerts: score-drop
		// comparison happens per whole campaign (ADR 0003). This widens the
		// interruption window versus the old per-run check — accepted trade —
		// but it must not be silent.
		slog.Warn("alerter: score-drop check skipped, campaign not done",
			"campaign_id", campaignID, "status", campaign.Status)
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
		slog.Debug("alerter: score-drop check skipped (webhook not configured)", "campaign_id", campaignID)
		return
	}

	runs, err := e.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		slog.Error("alerter: list campaign runs for score-drop check", "campaign_id", campaignID, "error", err)
		return
	}

	alerts := map[int64]*modelAlert{}
	var order []int64
	for i := range runs {
		run := &runs[i]
		if run.Status != "done" {
			continue
		}
		for _, drop := range e.compareSuiteRun(campaign, run) {
			entry, ok := alerts[drop.modelDBID]
			if !ok {
				entry = &modelAlert{modelID: drop.modelID}
				alerts[drop.modelDBID] = entry
				order = append(order, drop.modelDBID)
			}
			entry.drops = append(entry.drops, drop.suiteDrop)
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	for _, modelDBID := range order {
		e.sendModelAlert(ctx, webhook, campaign, alerts[modelDBID])
	}
}

// modelSuiteDrop pairs one model's identity with its drop inside one suite.
type modelSuiteDrop struct {
	modelDBID int64
	modelID   string
	suiteDrop
}

// compareSuiteRun compares one done run of the campaign against the previous
// done campaign's run for the same suite and returns every (model, suite)
// drop beyond the threshold. A cross-version pair is skipped and annotated;
// a missing baseline skips silently.
func (e *Evaluator) compareSuiteRun(campaign *store.Campaign, run *store.EvalRun) []modelSuiteDrop {
	previousRun, err := e.db.PreviousDoneCampaignRun(campaign.ID, run.SuiteID)
	if err != nil {
		slog.Error("alerter: load baseline run", "campaign_id", campaign.ID, "suite_id", run.SuiteID, "error", err)
		return nil
	}
	if previousRun == nil {
		return nil
	}
	if previousRun.SuiteVersion != run.SuiteVersion {
		e.annotateVersionSkip(campaign, run, previousRun)
		return nil
	}

	suite, err := e.db.GetSuite(run.SuiteID)
	if err != nil {
		slog.Error("alerter: load suite for score-drop check", "suite_id", run.SuiteID, "error", err)
		return nil
	}
	currentResults, err := e.db.ListEvalResults(run.ID)
	if err != nil {
		slog.Error("alerter: load current results for score-drop check", "run_id", run.ID, "error", err)
		return nil
	}
	previousResults, err := e.db.ListEvalResults(previousRun.ID)
	if err != nil {
		slog.Error("alerter: load baseline results for score-drop check", "run_id", previousRun.ID, "error", err)
		return nil
	}

	// A verdict-profile break makes the pair incomparable exactly like a
	// suite-version break (ADR 0008): skip and annotate instead of alerting.
	previousProfile, currentProfile := runVerdictProfile(previousResults), runVerdictProfile(currentResults)
	if previousProfile != "" && currentProfile != "" && previousProfile != currentProfile {
		e.annotateProfileSkip(campaign, run, previousRun, previousProfile, currentProfile)
		return nil
	}

	prompts := casePrompts(e.db, run.SuiteID)

	var drops []modelSuiteDrop
	for _, modelDBID := range sortedPairIDs(currentResults) {
		modelID, current := aggregate(currentResults, modelDBID)
		if current == nil {
			continue
		}
		_, previous := aggregate(previousResults, modelDBID)
		if previous == nil || *previous-*current <= ScoreDropThreshold {
			continue
		}
		drops = append(drops, modelSuiteDrop{
			modelDBID: modelDBID,
			modelID:   modelID,
			suiteDrop: suiteDrop{
				suiteName: suite.Name,
				previous:  *previous,
				current:   *current,
				changes:   caseChanges(previousResults, currentResults, modelDBID, prompts),
			},
		})
	}
	return drops
}

// annotateVersionSkip records that a (suite) comparison was skipped because
// the two runs ran different suite versions: one score_drop_skipped alert
// event plus a warn line on the current run's (already finished) task log.
// Nothing is sent to the webhook — the annotation replaces the alert.
func (e *Evaluator) annotateVersionSkip(campaign *store.Campaign, run, previousRun *store.EvalRun) {
	suiteName := e.suiteName(run.SuiteID)
	e.annotateSkip(campaign, run,
		fmt.Sprintf(
			"【HubScope】分数对比跳过:评估集「%s」题目已变更(Suite 版本 v%d → v%d),分数不可比;本轮评估(Campaign #%d)该评估集的分数大跌对比已跳过。",
			suiteName, previousRun.SuiteVersion, run.SuiteVersion, campaign.ID),
		fmt.Sprintf(
			"题目已变更(Suite 版本 v%d → v%d),分数不可比,评估集「%s」与上一轮(Campaign #%d)的分数大跌对比已跳过",
			previousRun.SuiteVersion, run.SuiteVersion, suiteName, previousRun.CampaignID))
}

// annotateProfileSkip records that a comparison was skipped because the two
// runs were scored under different verdict profiles (ADR 0008). Same shape
// as annotateVersionSkip: an event plus a task-log warn line, nothing sent.
func (e *Evaluator) annotateProfileSkip(campaign *store.Campaign, run, previousRun *store.EvalRun, previousProfile, currentProfile string) {
	suiteName := e.suiteName(run.SuiteID)
	e.annotateSkip(campaign, run,
		fmt.Sprintf(
			"【HubScope】分数对比跳过:评估集「%s」判分口径已变更(%s → %s),分数不可比;本轮评估(Campaign #%d)该评估集的分数大跌对比已跳过。",
			suiteName, previousProfile, currentProfile, campaign.ID),
		fmt.Sprintf(
			"判分口径已变更(%s → %s),分数不可比,评估集「%s」与上一轮(Campaign #%d)的分数大跌对比已跳过",
			previousProfile, currentProfile, suiteName, previousRun.CampaignID))
}

// suiteName resolves a suite's display name, falling back to its id.
func (e *Evaluator) suiteName(suiteID int64) string {
	suite, err := e.db.GetSuite(suiteID)
	if err != nil {
		slog.Error("alerter: load suite for skip annotation", "suite_id", suiteID, "error", err)
		return fmt.Sprintf("suite %d", suiteID)
	}
	return suite.Name
}

// annotateSkip persists one score_drop_skipped alert event and appends a warn
// line to the current run's (already finished) task log. Nothing is sent to
// the webhook — the annotation replaces the alert.
func (e *Evaluator) annotateSkip(campaign *store.Campaign, run *store.EvalRun, message, logLine string) {
	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindScoreDropSkipped,
		Message: message,
		SentOK:  false,
	}); err != nil {
		slog.Error("alerter: record score_drop_skipped event", "campaign_id", campaign.ID, "run_id", run.ID, "error", err)
	}

	task, err := e.db.GetTaskByEntity(store.TaskEntityEvalRun, run.ID)
	if err != nil {
		slog.Error("alerter: find task for skip annotation", "run_id", run.ID, "error", err)
		return
	}
	if task == nil {
		return
	}
	if err := e.db.AppendTaskLog(task.ID, store.TaskLogWarn, logLine, time.Now().UTC()); err != nil {
		slog.Error("alerter: annotate skip on task log", "task_id", task.ID, "error", err)
	}
}

// runVerdictProfile derives a run's scoring caliber from its results: the
// newest profile found among its rows (profiles order lexically: v1 < v2).
// "" means the run produced no results and its caliber is unknown — such a
// run never anchors a profile comparison.
func runVerdictProfile(results []store.EvalResult) string {
	profile := ""
	for _, r := range results {
		if r.VerdictProfile > profile {
			profile = r.VerdictProfile
		}
	}
	return profile
}

// sendModelAlert delivers the consolidated per-model alert and records it.
func (e *Evaluator) sendModelAlert(ctx context.Context, webhook string, campaign *store.Campaign, alert *modelAlert) {
	message := buildModelAlertMessage(campaign, alert)

	sentOK := true
	if err := e.sender.Send(ctx, webhook, message); err != nil {
		slog.Error("alerter: send score_drop alert", "campaign_id", campaign.ID, "model", alert.modelID, "error", err)
		sentOK = false
	}
	if _, err := e.db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindScoreDrop,
		Message: message,
		SentOK:  sentOK,
	}); err != nil {
		slog.Error("alerter: record score_drop event", "campaign_id", campaign.ID, "model", alert.modelID, "error", err)
	}
}

// buildModelAlertMessage composes the consolidated alert text: a header
// naming the model and campaign, one section per dropped suite with its
// aggregate fall, and the case-level changes behind each drop.
func buildModelAlertMessage(campaign *store.Campaign, alert *modelAlert) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【HubScope】评估分数大跌:模型 %s 本轮评估(Campaign #%d)对比上一轮,%d 个评估集得分大跌(阈值 %.1f):",
		alert.modelID, campaign.ID, len(alert.drops), ScoreDropThreshold)
	for _, drop := range alert.drops {
		fmt.Fprintf(&b, "\n·「%s」%.2f → %.2f(跌 %.2f)",
			drop.suiteName, drop.previous, drop.current, drop.previous-drop.current)
		shown := drop.changes
		if len(shown) > maxCaseChangesPerSuite {
			shown = shown[:maxCaseChangesPerSuite]
		}
		for _, ch := range shown {
			fmt.Fprintf(&b, "\n  - Case#%d %s:%s", ch.caseID, ch.prompt, caseChangeText(ch))
		}
		if extra := len(drop.changes) - len(shown); extra > 0 {
			fmt.Fprintf(&b, "\n  - …另有 %d 项变动", extra)
		}
	}
	return b.String()
}

// caseChangeText renders one case-level change: "1.00 → 未判分" for a case
// that lost its score, or "1.00 → 0.30(跌 0.70)" for a big drop.
func caseChangeText(ch caseChange) string {
	if ch.current == nil {
		return fmt.Sprintf("%.2f → 未判分", ch.previous)
	}
	return fmt.Sprintf("%.2f → %.2f(跌 %.2f)", ch.previous, *ch.current, ch.previous-*ch.current)
}

// caseChanges joins the two runs' results for one model by case and lists
// the material changes: scored-before cases that are now unjudged, and
// per-case drops beyond ScoreDropThreshold, ordered by case ID.
func caseChanges(previousResults, currentResults []store.EvalResult, modelDBID int64, prompts map[int64]string) []caseChange {
	previousByCase := map[int64]*float64{}
	for _, r := range previousResults {
		if r.ModelDBID == modelDBID {
			previousByCase[r.CaseID] = r.Score
		}
	}

	var changes []caseChange
	for _, r := range currentResults {
		if r.ModelDBID != modelDBID {
			continue
		}
		previous, ok := previousByCase[r.CaseID]
		if !ok || previous == nil {
			continue
		}
		switch {
		case r.Score == nil:
			changes = append(changes, caseChange{
				caseID: r.CaseID, prompt: prompts[r.CaseID], previous: *previous,
			})
		case *previous-*r.Score > ScoreDropThreshold:
			current := *r.Score
			changes = append(changes, caseChange{
				caseID: r.CaseID, prompt: prompts[r.CaseID], previous: *previous, current: &current,
			})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].caseID < changes[j].caseID })
	return changes
}

// casePrompts maps every case of a suite (enabled or retired) to a bounded
// prompt excerpt for alert text. Unresolvable cases fall back to an empty
// excerpt.
func casePrompts(db *store.DB, suiteID int64) map[int64]string {
	prompts := map[int64]string{}
	cases, err := db.ListCases(suiteID)
	if err != nil {
		slog.Error("alerter: load cases for alert text", "suite_id", suiteID, "error", err)
		return prompts
	}
	for _, c := range cases {
		prompts[c.ID] = snippet(c.Prompt, promptSnippetRunes)
	}
	return prompts
}

// snippet truncates s to n runes, appending an ellipsis when cut.
func snippet(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
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
	modelID := ""
	var sum float64
	var count int
	for _, r := range results {
		if r.ModelDBID != modelDBID {
			continue
		}
		modelID = r.ModelID
		if r.Score != nil {
			sum += *r.Score
			count++
		}
	}
	if count == 0 {
		return modelID, nil
	}
	avg := sum / float64(count)
	return modelID, &avg
}
