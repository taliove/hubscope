// Package evaluator runs evaluation suites against models via the Hub and
// scores each answer either by rule (exact/regex/contains) or by an LLM
// judge following a rubric.
package evaluator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/store"
)

// evalMaxTokens gives evaluated models room for a complete answer (unlike
// the 16-token probe budget).
const evalMaxTokens = 1024

// RequestTimeout bounds a single completion call during evaluation.
const RequestTimeout = 120 * time.Second

// Evaluator executes eval runs and persists per-case results.
//
// AfterCampaign, when set, is invoked once after a campaign settles to
// "done" (never for failed campaigns). The score-drop alerter hooks in here;
// hook errors must be handled by the hook itself (the alerter logs instead
// of failing).
type Evaluator struct {
	db     *store.DB
	client *hubclient.Client

	AfterCampaign func(ctx context.Context, campaignID int64)
}

// New creates an Evaluator backed by the given store and hub client.
func New(db *store.DB, client *hubclient.Client) *Evaluator {
	return &Evaluator{db: db, client: client}
}

// RunEval executes every enabled case of the run's suite against each
// selected model and marks the run done. A failing model never blocks the
// others; per-case failures are recorded as results with nil scores.
//
// The judge model is read from settings at run start (default
// store.DefaultJudgeModel); the run record is updated when the configured
// value differs from the snapshot taken at creation, so eval_runs.judge_model
// always reflects the judge actually used. The default sample count is read
// the same way; a case's own sample_count overrides it.
func (e *Evaluator) RunEval(ctx context.Context, runID int64, modelDBIDs []int64) error {
	run, err := e.db.GetEvalRun(runID)
	if err != nil {
		return fmt.Errorf("load eval run %d: %w", runID, err)
	}
	run = e.resolveJudgeModel(run)
	defaultSamples := e.resolveDefaultSampleCount()

	// Register the run with the task center; tracking failures never abort
	// the run itself (beginRunTask returns a no-op logger then).
	task := e.beginRunTask(run, len(modelDBIDs))

	cases, err := e.db.ListEnabledCases(run.SuiteID)
	if err != nil {
		_ = e.db.FinishEvalRun(runID, "failed", time.Now().UTC())
		task.fail(fmt.Sprintf("load cases for suite %d: %v", run.SuiteID, err))
		return fmt.Errorf("load cases for suite %d: %w", run.SuiteID, err)
	}

	for _, modelDBID := range modelDBIDs {
		if err := ctx.Err(); err != nil {
			_ = e.db.FinishEvalRun(runID, "failed", time.Now().UTC())
			task.fail(err.Error())
			return err
		}
		e.evalModel(ctx, run, modelDBID, cases, task, defaultSamples)
	}

	if err := e.db.FinishEvalRun(runID, "done", time.Now().UTC()); err != nil {
		task.fail(err.Error())
		return err
	}
	task.succeed()
	return nil
}

// SettleCampaign settles a campaign from its member runs and, when the
// campaign reached "done", invokes the AfterCampaign hook. It is the single
// settle entry point shared by the sweep loop and the single-run trigger, so
// every done campaign produces exactly one hook invocation: terminal campaign
// states are sticky (the first settle wins) and each campaign is settled by
// exactly one goroutine.
//
// The hook runs detached from cancellation so a graceful shutdown cannot
// abort an alert send mid-flight, and a panicking hook is logged rather than
// allowed to take down the caller.
func (e *Evaluator) SettleCampaign(ctx context.Context, campaignID int64) {
	if err := e.db.SettleCampaign(campaignID, time.Now().UTC()); err != nil {
		slog.Error("evaluator: settle campaign", "campaign_id", campaignID, "error", err)
		return
	}
	if e.AfterCampaign == nil {
		return
	}
	campaign, err := e.db.GetCampaign(campaignID)
	if err != nil {
		slog.Error("evaluator: reload settled campaign", "campaign_id", campaignID, "error", err)
		return
	}
	if campaign.Status != store.CampaignStatusDone {
		return
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("evaluator: AfterCampaign hook panicked", "campaign_id", campaignID, "panic", r)
			}
		}()
		e.AfterCampaign(context.WithoutCancel(ctx), campaignID)
	}()
}

// resolveJudgeModel reads the configured judge model from settings and, when
// it differs from the run's snapshot, persists it onto the run. It returns a
// copy of the run carrying the effective judge model; the input is not
// mutated. Read/write failures fall back to the snapshot.
func (e *Evaluator) resolveJudgeModel(run *store.EvalRun) *store.EvalRun {
	judgeModel, err := e.db.GetSetting(store.SettingJudgeModel, store.DefaultJudgeModel)
	if err != nil {
		slog.Error("evaluator: read judge_model setting, keeping run snapshot", "error", err)
		return run
	}
	if judgeModel == "" || judgeModel == run.JudgeModel {
		return run
	}
	if err := e.db.SetEvalRunJudgeModel(run.ID, judgeModel); err != nil {
		slog.Error("evaluator: record judge_model on run", "run_id", run.ID, "error", err)
		return run
	}
	updated := *run
	updated.JudgeModel = judgeModel
	return &updated
}

// resolveDefaultSampleCount reads the global default sample count from
// settings, clamped to [1, store.MaxSampleCount]. Read failures fall back to
// the built-in default.
func (e *Evaluator) resolveDefaultSampleCount() int {
	n, err := e.db.GetSettingInt(store.SettingDefaultSampleCount, store.DefaultSampleCount)
	if err != nil {
		slog.Error("evaluator: read default_sample_count setting, using default", "error", err)
		return store.DefaultSampleCount
	}
	if n < 1 {
		return 1
	}
	if n > store.MaxSampleCount {
		return store.MaxSampleCount
	}
	return n
}

// evalModel runs all cases against one model. Any setup failure (model gone,
// hub gone, no enabled endpoint) is recorded as failed results for every
// case so the model x case grid stays complete, and logged to the task.
func (e *Evaluator) evalModel(ctx context.Context, run *store.EvalRun, modelDBID int64, cases []store.Case, task *runTask, defaultSamples int) {
	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		e.failAllCases(run, modelDBID, "", cases, "model not found")
		task.log(store.TaskLogWarn, fmt.Sprintf("model db_id=%d skipped: model not found", modelDBID))
		return
	}

	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "hub not found")
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: hub not found", model.ModelID))
		return
	}

	endpoints, err := e.db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "failed to load endpoints")
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: failed to load endpoints", model.ModelID))
		return
	}

	protocol, ok := selectProtocol(endpoints)
	if !ok {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "no enabled endpoint for this model")
		task.log(store.TaskLogWarn, fmt.Sprintf("model %s skipped: no enabled endpoint for this model", model.ModelID))
		return
	}

	for _, c := range cases {
		e.evalCase(ctx, run, hub, protocol, model, c, task, defaultSamples)
	}
}

// evalCase answers one case sampleCount times and stores a single result
// whose score is the average of the judged samples. Samples that cannot be
// judged (answer call failed, judge failed) contribute no score; when no
// sample is judged at all the case stays unscored — the same convention as a
// single unjudged answer. The outcome is logged to the task: scored
// completions at info, answer/judge failures at warn.
func (e *Evaluator) evalCase(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case, task *runTask, defaultSamples int) {
	samples := defaultSamples
	if c.SampleCount != nil && *c.SampleCount >= 1 {
		samples = *c.SampleCount
	}

	result := store.EvalResult{
		EvalRunID:      run.ID,
		ModelDBID:      model.ID,
		ModelID:        model.ModelID,
		CaseID:         c.ID,
		VerdictProfile: VerdictProfileCurrent,
	}

	var scoreSum float64
	var scored int
	var details []string
	for i := 1; i <= samples; i++ {
		sample := e.evalSample(ctx, run, hub, protocol, model, c)
		result.LatencyMs += sample.latencyMs
		result.InputTokens = addIntPtr(result.InputTokens, sample.inputTokens)
		result.OutputTokens = addIntPtr(result.OutputTokens, sample.outputTokens)
		if sample.answer != nil && result.AnswerText == nil {
			result.AnswerText = sample.answer
		}
		if sample.score != nil {
			scoreSum += *sample.score
			scored++
		}
		details = append(details, fmt.Sprintf("sample %d/%d: %s", i, samples, sample.detail))
	}

	if scored > 0 {
		avg := scoreSum / float64(scored)
		result.Score = &avg
	}
	detail := strings.Join(details, "; ")
	result.VerdictDetail = &detail
	e.storeResult(result)

	switch {
	case result.Score != nil:
		task.log(store.TaskLogInfo, fmt.Sprintf("case %d done: model=%s score=%.2f", c.ID, model.ModelID, *result.Score))
	case strings.Contains(detail, "judge"):
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d judge failed: model=%s detail=%q", c.ID, model.ModelID, detail))
	default:
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d failed: model=%s detail=%q", c.ID, model.ModelID, detail))
	}
}

// sampleOutcome is one answer-and-verdict attempt for a case.
type sampleOutcome struct {
	answer       *string
	score        *float64
	detail       string
	latencyMs    int
	inputTokens  *int
	outputTokens *int
}

// evalSample executes one answer call plus its verdict.
func (e *Evaluator) evalSample(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case) sampleOutcome {
	res := e.client.Complete(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, c.Prompt, evalMaxTokens)
	out := sampleOutcome{
		latencyMs:    res.LatencyMs,
		inputTokens:  res.InputTokens,
		outputTokens: res.OutputTokens,
	}

	if !res.OK {
		out.detail = "answer call failed"
		if res.ErrorSummary != nil {
			out.detail = "answer call failed: " + *res.ErrorSummary
		}
		return out
	}

	out.answer = &res.Text
	out.score, out.detail = e.verdict(ctx, hub, protocol, run.JudgeModel, c, res.Text)
	return out
}

// addIntPtr sums two nullable ints; null only when both sides are null.
func addIntPtr(a, b *int) *int {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	sum := *a + *b
	return &sum
}

// verdict scores an answer according to the case's verdict type. Rule
// verdicts run under the current verdict profile (ADR 0008).
func (e *Evaluator) verdict(ctx context.Context, hub *store.Hub, protocol, judgeModel string, c store.Case, answer string) (*float64, string) {
	if c.VerdictType == "rule" {
		return ruleVerdict(c, answer, VerdictProfileCurrent)
	}
	return e.judgeVerdict(ctx, hub, protocol, judgeModel, c, answer)
}

// failAllCases records a failed result (no answer, no score) for every case.
// Failed results carry the current profile too, so a run's caliber can always
// be derived from any of its rows.
func (e *Evaluator) failAllCases(run *store.EvalRun, modelDBID int64, modelID string, cases []store.Case, reason string) {
	for _, c := range cases {
		e.storeResult(store.EvalResult{
			EvalRunID:      run.ID,
			ModelDBID:      modelDBID,
			ModelID:        modelID,
			CaseID:         c.ID,
			VerdictDetail:  &reason,
			VerdictProfile: VerdictProfileCurrent,
		})
	}
}

// storeResult persists one result; persistence errors are logged but never
// abort the whole run.
func (e *Evaluator) storeResult(r store.EvalResult) {
	if _, err := e.db.CreateEvalResult(r); err != nil {
		slog.Error("evaluator: persist result for case", "case_id", r.CaseID, "error", err)
	}
}

// selectProtocol picks the chat protocol used to call a model: anthropic
// when its endpoint is enabled, otherwise openai. Image-protocol endpoints
// are never selected (spec 0014, R1 guard): an evaluation prompt sent to an
// image endpoint would burn a paid generation per case and pollute scoring
// with non-answers, so a model without an enabled chat endpoint reports "no
// enabled endpoint" instead.
func selectProtocol(endpoints []store.Endpoint) (string, bool) {
	for _, ep := range endpoints {
		if ep.Enabled && ep.Protocol == "anthropic" {
			return "anthropic", true
		}
	}
	for _, ep := range endpoints {
		if ep.Enabled && ep.Protocol == "openai" {
			return "openai", true
		}
	}
	return "", false
}
