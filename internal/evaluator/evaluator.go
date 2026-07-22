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

	"github.com/taliove2009/hubscope/internal/hubclient"
	"github.com/taliove2009/hubscope/internal/store"
)

// evalMaxTokens gives evaluated models room for a complete answer (unlike
// the 16-token probe budget).
const evalMaxTokens = 1024

// RequestTimeout bounds a single completion call during evaluation.
const RequestTimeout = 120 * time.Second

// Evaluator executes eval runs and persists per-case results.
//
// AfterRun, when set, is invoked once after a run reaches "done" (never for
// failed runs). The score-drop alerter hooks in here; hook errors must be
// handled by the hook itself (the alerter logs instead of failing).
type Evaluator struct {
	db     *store.DB
	client *hubclient.Client

	AfterRun func(ctx context.Context, runID int64)
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
// always reflects the judge actually used.
func (e *Evaluator) RunEval(ctx context.Context, runID int64, modelDBIDs []int64) error {
	run, err := e.db.GetEvalRun(runID)
	if err != nil {
		return fmt.Errorf("load eval run %d: %w", runID, err)
	}
	run = e.resolveJudgeModel(run)

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
		e.evalModel(ctx, run, modelDBID, cases, task)
	}

	if err := e.db.FinishEvalRun(runID, "done", time.Now().UTC()); err != nil {
		task.fail(err.Error())
		return err
	}
	task.succeed()
	if e.AfterRun != nil {
		// Detach cancellation so a graceful shutdown cannot abort the alert
		// send mid-flight (mirrors the prober's WithoutCancel rounds). A
		// misbehaving hook must never take down the evaluator.
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("evaluator: AfterRun hook panicked", "run_id", runID, "panic", r)
				}
			}()
			e.AfterRun(context.WithoutCancel(ctx), runID)
		}()
	}
	return nil
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

// evalModel runs all cases against one model. Any setup failure (model gone,
// hub gone, no enabled endpoint) is recorded as failed results for every
// case so the model x case grid stays complete, and logged to the task.
func (e *Evaluator) evalModel(ctx context.Context, run *store.EvalRun, modelDBID int64, cases []store.Case, task *runTask) {
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
		e.evalCase(ctx, run, hub, protocol, model, c, task)
	}
}

// evalCase executes one case against one model, stores the result and logs
// the outcome to the task: scored completions at info, answer/judge failures
// at warn.
func (e *Evaluator) evalCase(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case, task *runTask) {
	result := store.EvalResult{
		EvalRunID: run.ID,
		ModelDBID: model.ID,
		ModelID:   model.ModelID,
		CaseID:    c.ID,
	}

	res := e.client.Complete(ctx, hub.BaseURL, hub.Token, protocol, model.ModelID, c.Prompt, evalMaxTokens)
	result.LatencyMs = res.LatencyMs
	result.InputTokens = res.InputTokens
	result.OutputTokens = res.OutputTokens

	if !res.OK {
		detail := "answer call failed"
		if res.ErrorSummary != nil {
			detail = "answer call failed: " + *res.ErrorSummary
		}
		result.VerdictDetail = &detail
		e.storeResult(result)
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d failed: model=%s detail=%q", c.ID, model.ModelID, detail))
		return
	}

	result.AnswerText = &res.Text
	score, detail := e.verdict(ctx, hub, protocol, run.JudgeModel, c, res.Text)
	result.Score = score
	result.VerdictDetail = &detail
	e.storeResult(result)

	switch {
	case score != nil:
		task.log(store.TaskLogInfo, fmt.Sprintf("case %d done: model=%s score=%.2f", c.ID, model.ModelID, *score))
	case strings.HasPrefix(detail, "judge"):
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d judge failed: model=%s detail=%q", c.ID, model.ModelID, detail))
	default:
		task.log(store.TaskLogWarn, fmt.Sprintf("case %d failed: model=%s detail=%q", c.ID, model.ModelID, detail))
	}
}

// verdict scores an answer according to the case's verdict type.
func (e *Evaluator) verdict(ctx context.Context, hub *store.Hub, protocol, judgeModel string, c store.Case, answer string) (*float64, string) {
	if c.VerdictType == "rule" {
		return ruleVerdict(c, answer)
	}
	return e.judgeVerdict(ctx, hub, protocol, judgeModel, c, answer)
}

// failAllCases records a failed result (no answer, no score) for every case.
func (e *Evaluator) failAllCases(run *store.EvalRun, modelDBID int64, modelID string, cases []store.Case, reason string) {
	for _, c := range cases {
		e.storeResult(store.EvalResult{
			EvalRunID:     run.ID,
			ModelDBID:     modelDBID,
			ModelID:       modelID,
			CaseID:        c.ID,
			VerdictDetail: &reason,
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

// selectProtocol picks the protocol used to call a model: anthropic when its
// endpoint is enabled, otherwise any enabled endpoint's protocol.
func selectProtocol(endpoints []store.Endpoint) (string, bool) {
	for _, ep := range endpoints {
		if ep.Enabled && ep.Protocol == "anthropic" {
			return "anthropic", true
		}
	}
	for _, ep := range endpoints {
		if ep.Enabled {
			return ep.Protocol, true
		}
	}
	return "", false
}
