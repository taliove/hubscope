package evaluator

// Crash recovery for the decoupled pipeline (GH #176, ADR 0016). store.Open
// has already stamped a dead process's running runs failed; this sweep
// rescues what the crash suppressed: every persisted answer's missing jury
// votes are re-issued (completed slots are never re-judged) and the case
// results are written. The exam stage is not recovered — the run keeps its
// failed stamp: recovery rescues results, it does not resurrect the run.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/taliove/hubscope/internal/store"
)

// RecoverInterruptedRuns drains judge work left behind by a crashed
// process. It runs once at server start; live batches (runs still running
// on this process) are never touched.
func (e *Evaluator) RecoverInterruptedRuns(ctx context.Context) {
	runIDs, err := e.db.ListRunsWithAnswers()
	if err != nil {
		slog.Error("evaluator: list runs with answers for recovery", "error", err)
		return
	}
	for _, runID := range runIDs {
		run, err := e.db.GetEvalRun(runID)
		if err != nil {
			slog.Error("evaluator: load run for recovery", "run_id", runID, "error", err)
			continue
		}
		if run.Status == "running" {
			continue // a live batch on this process, not crash residue
		}
		_, juries := parseJurySnapshot(run.JuryModels)
		if juries == nil {
			continue // pre-pipeline run: nothing to recover
		}
		e.recoverRunJudging(ctx, run, juries)
	}
}

// recoverRunJudging re-issues the missing jury votes of one run's persisted
// answers and writes the case results the crash suppressed.
func (e *Evaluator) recoverRunJudging(ctx context.Context, run *store.EvalRun, juries map[int64][]string) {
	results, err := e.db.ListEvalResults(run.ID)
	if err != nil {
		slog.Error("evaluator: list results for recovery", "run_id", run.ID, "error", err)
		return
	}
	haveResult := map[[2]int64]bool{}
	for _, r := range results {
		haveResult[[2]int64{r.ModelDBID, r.CaseID}] = true
	}
	answers, err := e.db.ListEvalAnswersByRun(run.ID)
	if err != nil {
		slog.Error("evaluator: list answers for recovery", "run_id", run.ID, "error", err)
		return
	}

	byCell := map[[2]int64][]store.EvalAnswer{}
	var order [][2]int64
	for _, a := range answers {
		key := [2]int64{a.ModelDBID, a.CaseID}
		if _, seen := byCell[key]; !seen {
			order = append(order, key)
		}
		byCell[key] = append(byCell[key], a)
	}

	for _, key := range order {
		modelDBID, caseID := key[0], key[1]
		if haveResult[key] {
			continue
		}
		c, err := e.db.GetCase(caseID)
		if err != nil {
			slog.Error("evaluator: load case for recovery", "case_id", caseID, "error", err)
			continue
		}
		if c.VerdictType != "judge" {
			continue // rule results were written inline at exam time
		}
		e.recoverCase(ctx, run, *c, byCell[key], juries[modelDBID])
	}
}

// recoverCase judges one (model, case) cell's persisted answers and writes
// its result row.
func (e *Evaluator) recoverCase(ctx context.Context, run *store.EvalRun, c store.Case, answers []store.EvalAnswer, judges []string) {
	if len(answers) == 0 {
		return
	}
	modelDBID := answers[0].ModelDBID
	modelID := answers[0].ModelID

	model, err := e.db.GetModel(modelDBID)
	if err != nil {
		slog.Error("evaluator: recovery skips cell, model gone", "model_db_id", modelDBID, "error", err)
		return
	}
	hub, err := e.db.GetHub(model.HubID)
	if err != nil {
		slog.Error("evaluator: recovery skips cell, hub gone", "model_db_id", modelDBID, "error", err)
		return
	}
	endpoints, err := e.db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		slog.Error("evaluator: recovery skips cell, endpoints unloadable", "model_db_id", modelDBID, "error", err)
		return
	}
	protocol, ok := selectProtocol(endpoints)
	if !ok && len(judges) > 0 {
		slog.Error("evaluator: recovery skips cell, no enabled endpoint", "model_db_id", modelDBID)
		return
	}

	agg := caseAggregate{modelID: modelID, profile: store.VerdictProfileV3}
	// Only the latest attempt of each sample counts: an earlier attempt
	// was superseded by its retry, result and judging alike.
	latest := map[int]store.EvalAnswer{}
	var sampleOrder []int
	for _, a := range answers {
		if prev, ok := latest[a.SampleNo]; !ok || a.Attempt > prev.Attempt {
			if !ok {
				sampleOrder = append(sampleOrder, a.SampleNo)
			}
			latest[a.SampleNo] = a
		}
	}
	agg.expectedSamples = len(latest)
	for _, sampleNo := range sampleOrder {
		a := latest[sampleNo]
		agg.latencyMs += a.LatencyMs
		agg.inputTokens = addIntPtr(agg.inputTokens, a.InputTokens)
		agg.outputTokens = addIntPtr(agg.outputTokens, a.OutputTokens)
		if agg.answerText == nil {
			agg.answerText = a.AnswerText
		}
		if a.Status != store.EvalAnswerAnswered || a.AnswerText == nil {
			agg.settledSamples++
			agg.details = append(agg.details, fmt.Sprintf("sample %d: answer call failed (recovered)", a.SampleNo))
			continue
		}
		if len(judges) == 0 {
			agg.settledSamples++
			agg.details = append(agg.details, fmt.Sprintf("sample %d: no jury available (recovered)", a.SampleNo))
			continue
		}
		votes := make([]*float64, len(judges))
		stored, err := e.db.ListJudgeScoresByAnswer(a.ID)
		if err != nil {
			slog.Error("evaluator: list judge scores for recovery", "answer_id", a.ID, "error", err)
			continue
		}
		for _, s := range stored {
			if s.Slot >= 0 && s.Slot < len(votes) {
				votes[s.Slot] = s.Score
			}
		}
		// Re-issue only the missing slots — a completed vote is never
		// re-judged (ADR 0016).
		var details []string
		for slot, judgeModel := range judges {
			if votes[slot] != nil || slotDone(stored, slot) {
				details = append(details, voteDetail(judgeModel, votes[slot], ""))
				continue
			}
			score, _ := e.judgeVerdict(ctx, hub, protocol, judgeModel, c, *a.AnswerText)
			if _, err := e.db.CreateEvalJudgeScore(store.EvalJudgeScore{
				AnswerID: a.ID, Slot: slot, JudgeModel: judgeModel, Score: score,
			}); err != nil {
				slog.Error("evaluator: persist recovered judge score", "answer_id", a.ID, "error", err)
			}
			votes[slot] = score
			details = append(details, voteDetail(judgeModel, score, ""))
		}
		median, rule := medianOfVotes(votes)
		agg.settledSamples++
		if median != nil {
			agg.scoreSum += *median
			agg.scored++
		}
		agg.details = append(agg.details, fmt.Sprintf("sample %d: %s → %s (recovered)", a.SampleNo, strings.Join(details, ", "), rule))
	}

	if agg.settledSamples != agg.expectedSamples {
		slog.Error("evaluator: recovery could not settle every sample", "run_id", run.ID, "case_id", c.ID)
		return
	}
	p := newPipeline(e, nil)
	p.writeCaseResult(run.ID, modelDBID, c.ID, &agg)
	slog.Info("evaluator: recovered case result", "run_id", run.ID, "case_id", c.ID, "model", modelID)
}

// slotDone reports whether a judge-score row exists for the slot at all —
// including a NULL-score row (a failed judge is never re-judged, W7).
func slotDone(scores []store.EvalJudgeScore, slot int) bool {
	for _, s := range scores {
		if s.Slot == slot {
			return true
		}
	}
	return false
}
