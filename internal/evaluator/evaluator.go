// Package evaluator runs evaluation suites against models via the Hub and
// scores each answer either by rule (exact/regex/contains) or by an LLM
// judge following a rubric.
package evaluator

import (
	"context"
	"fmt"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// DefaultJudgeModel scores judge-type cases when nothing else is configured.
// TODO(ticket 06): read from the settings table (settings.judge_model) once
// it exists; until then this package-level constant is the single source.
const DefaultJudgeModel = "claude-opus-4-8"

// evalMaxTokens gives evaluated models room for a complete answer (unlike
// the 16-token probe budget).
const evalMaxTokens = 1024

// RequestTimeout bounds a single completion call during evaluation.
const RequestTimeout = 120 * time.Second

// Evaluator executes eval runs and persists per-case results.
type Evaluator struct {
	db     *store.DB
	client *hubclient.Client
}

// New creates an Evaluator backed by the given store and hub client.
func New(db *store.DB, client *hubclient.Client) *Evaluator {
	return &Evaluator{db: db, client: client}
}

// RunEval executes every enabled case of the run's suite against each
// selected model and marks the run done. A failing model never blocks the
// others; per-case failures are recorded as results with nil scores.
func (e *Evaluator) RunEval(ctx context.Context, runID int64, modelDBIDs []int64) error {
	run, err := e.db.GetEvalRun(runID)
	if err != nil {
		return fmt.Errorf("load eval run %d: %w", runID, err)
	}

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

	return e.db.FinishEvalRun(runID, "done", time.Now().UTC())
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

// storeResult persists one result; persistence errors are intentionally
// swallowed so one bad write cannot abort the whole run.
func (e *Evaluator) storeResult(r store.EvalResult) {
	_, _ = e.db.CreateEvalResult(r)
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
