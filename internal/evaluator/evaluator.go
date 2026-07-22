// Package evaluator runs evaluation suites against models via the Hub and
// scores each answer either by rule (exact/regex/contains) or by an LLM
// judge following a rubric.
package evaluator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
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

	cases, err := e.db.ListEnabledCases(run.SuiteID)
	if err != nil {
		_ = e.db.FinishEvalRun(runID, "failed", time.Now().UTC())
		return fmt.Errorf("load cases for suite %d: %w", run.SuiteID, err)
	}

	for _, modelDBID := range modelDBIDs {
		if err := ctx.Err(); err != nil {
			_ = e.db.FinishEvalRun(runID, "failed", time.Now().UTC())
			return err
		}
		e.evalModel(ctx, run, modelDBID, cases)
	}

	if err := e.db.FinishEvalRun(runID, "done", time.Now().UTC()); err != nil {
		return err
	}
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
// case so the model x case grid stays complete.
func (e *Evaluator) evalModel(ctx context.Context, run *store.EvalRun, modelDBID int64, cases []store.Case) {
	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		e.failAllCases(run, modelDBID, "", cases, "model not found")
		return
	}

	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "hub not found")
		return
	}

	endpoints, err := e.db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "failed to load endpoints")
		return
	}

	protocol, ok := selectProtocol(endpoints)
	if !ok {
		e.failAllCases(run, modelDBID, model.ModelID, cases, "no enabled endpoint for this model")
		return
	}

	for _, c := range cases {
		e.evalCase(ctx, run, hub, protocol, model, c)
	}
}

// evalCase executes one case against one model and stores the result.
func (e *Evaluator) evalCase(ctx context.Context, run *store.EvalRun, hub *store.Hub, protocol string, model *store.Model, c store.Case) {
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
		return
	}

	result.AnswerText = &res.Text
	score, detail := e.verdict(ctx, hub, protocol, run.JudgeModel, c, res.Text)
	result.Score = score
	result.VerdictDetail = &detail
	e.storeResult(result)
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
