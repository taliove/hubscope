package store

import (
	"testing"
	"time"
)

// setupEvalRunForAnswers creates the hub/model/campaign/run chain the answer
// tables hang off.
func setupEvalRunForAnswers(t *testing.T, db *DB) (*EvalRun, *Model) {
	t.Helper()
	hub, err := db.CreateHub("answer-hub", "http://answer.test", "tok-answer-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	model, err := db.CreateModel(hub.ID, "answer-model", []string{"openai"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	campaign, err := db.CreateCampaign("manual", []int64{model.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	run, err := db.CreateEvalRun(campaign.ID, suites[0].ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run, model
}

// TestEvalAnswerJudgeScoreRoundTrip pins the ADR 0016 persistence contract:
// answers and per-slot jury verdicts land and read back, a failed judge
// keeps a NULL score (W7), and the cell/slot uniqueness constraints hold.
func TestEvalAnswerJudgeScoreRoundTrip(t *testing.T) {
	db := openTestDB(t)
	run, model := setupEvalRunForAnswers(t, db)

	text := "the answer"
	answerID, err := db.CreateEvalAnswer(EvalAnswer{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: 7, SampleNo: 1, Status: EvalAnswerAnswered, AnswerText: &text,
		LatencyMs: 1200, InputTokens: new(200), OutputTokens: new(400),
	})
	if err != nil {
		t.Fatalf("create answer: %v", err)
	}
	// Re-answering the same cell (retry-failed) lands as attempt 2, not a
	// duplicate-key failure (GH #176).
	secondID, err := db.CreateEvalAnswer(EvalAnswer{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: 7, SampleNo: 1, Status: EvalAnswerFailed,
	})
	if err != nil {
		t.Fatalf("second attempt on the same cell must be allowed: %v", err)
	}
	if secondID == answerID {
		t.Fatal("the second attempt must be its own row")
	}

	score := 0.62
	if _, err := db.CreateEvalJudgeScore(EvalJudgeScore{AnswerID: answerID, Slot: 0, JudgeModel: "judge-a", Score: &score, LatencyMs: 800}); err != nil {
		t.Fatalf("create judge score: %v", err)
	}
	// A failed judge call lands with a NULL score, never a zero.
	if _, err := db.CreateEvalJudgeScore(EvalJudgeScore{AnswerID: answerID, Slot: 1, JudgeModel: "judge-b", LatencyMs: 900}); err != nil {
		t.Fatalf("create failed-judge row: %v", err)
	}
	if _, err := db.CreateEvalJudgeScore(EvalJudgeScore{AnswerID: answerID, Slot: 1, JudgeModel: "judge-b"}); err == nil {
		t.Fatal("duplicate (answer, slot) must be rejected by the unique index")
	}

	answers, err := db.ListEvalAnswersByRun(run.ID)
	if err != nil || len(answers) != 2 {
		t.Fatalf("list answers: %v (n=%d, want 2 attempts)", err, len(answers))
	}
	if answers[0].AnswerText == nil || *answers[0].AnswerText != text || answers[0].LatencyMs != 1200 {
		t.Fatalf("answer round-trip mismatch: %+v", answers[0])
	}
	if answers[0].Attempt != 1 || answers[1].Attempt != 2 {
		t.Fatalf("attempt numbers = %d, %d, want 1, 2", answers[0].Attempt, answers[1].Attempt)
	}
	scores, err := db.ListJudgeScoresByAnswer(answerID)
	if err != nil || len(scores) != 2 {
		t.Fatalf("list judge scores: %v (n=%d)", err, len(scores))
	}
	if scores[0].Score == nil || *scores[0].Score != 0.62 {
		t.Fatalf("slot 0 score mismatch: %+v", scores[0])
	}
	if scores[1].Score != nil {
		t.Fatalf("failed judge must read back NULL score, got %+v", scores[1].Score)
	}
}

// TestListUnderJudgedAnswers pins the crash-recovery candidate query: only
// answered rows with fewer than jurySize judge rows come back, and a fully
// judged answer leaves the set (never re-judged, ADR 0016).
func TestListUnderJudgedAnswers(t *testing.T) {
	db := openTestDB(t)
	run, model := setupEvalRunForAnswers(t, db)
	text := "x"

	pending, err := db.CreateEvalAnswer(EvalAnswer{EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: 1, SampleNo: 1, Status: EvalAnswerAnswered, AnswerText: &text})
	if err != nil {
		t.Fatalf("create pending answer: %v", err)
	}
	complete, err := db.CreateEvalAnswer(EvalAnswer{EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: 2, SampleNo: 1, Status: EvalAnswerAnswered, AnswerText: &text})
	if err != nil {
		t.Fatalf("create complete answer: %v", err)
	}
	failed, err := db.CreateEvalAnswer(EvalAnswer{EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: 3, SampleNo: 1, Status: EvalAnswerFailed})
	if err != nil {
		t.Fatalf("create failed answer: %v", err)
	}
	for slot := 0; slot < 3; slot++ {
		if _, err := db.CreateEvalJudgeScore(EvalJudgeScore{AnswerID: complete, Slot: slot, JudgeModel: "j"}); err != nil {
			t.Fatalf("fill complete answer: %v", err)
		}
	}
	if _, err := db.CreateEvalJudgeScore(EvalJudgeScore{AnswerID: pending, Slot: 0, JudgeModel: "j"}); err != nil {
		t.Fatalf("partial judge: %v", err)
	}

	got, err := db.ListUnderJudgedAnswers(run.ID, 3)
	if err != nil {
		t.Fatalf("list under-judged: %v", err)
	}
	if len(got) != 1 || got[0].ID != pending {
		t.Fatalf("only the partially judged answer should be pending, got %+v", got)
	}
	// A failed answer never enters the judge queue, so it never appears.
	for _, a := range got {
		if a.ID == failed || a.ID == complete {
			t.Fatalf("failed/complete answers must not be pending: %+v", a)
		}
	}
}

// TestEvalRunJuryAndCostColumns pins the new eval_runs columns: unset reads
// come back empty, writes round-trip, and a nil component stays NULL (the
// "price not registered" caliber).
func TestEvalRunJuryAndCostColumns(t *testing.T) {
	db := openTestDB(t)
	run, _ := setupEvalRunForAnswers(t, db)

	if jury, err := db.GetEvalRunJuryModels(run.ID); err != nil || jury != "" {
		t.Fatalf("unset jury should read empty, got %q, %v", jury, err)
	}
	reloaded, err := db.GetEvalRun(run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if reloaded.EstimatedCost != "" {
		t.Fatalf("unset cost should read empty, got %q", reloaded.EstimatedCost)
	}

	juryJSON := `{"policy":"balanced","judges":["qwen3-235b","deepseek-v3","qwen3-30b-a3b"]}`
	if err := db.SetEvalRunJuryModels(run.ID, juryJSON); err != nil {
		t.Fatalf("set jury: %v", err)
	}
	if jury, err := db.GetEvalRunJuryModels(run.ID); err != nil || jury != juryJSON {
		t.Fatalf("jury round-trip: got %q, %v", jury, err)
	}

	exam, judge := 1.23, 0.45
	if err := db.SetEvalRunCost(run.ID, &exam, &judge); err != nil {
		t.Fatalf("set cost: %v", err)
	}
	reloaded, err = db.GetEvalRun(run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if reloaded.EstimatedCost != `{"exam":1.23,"judge":0.45}` {
		t.Fatalf("cost round-trip: got %q", reloaded.EstimatedCost)
	}
	if err := db.SetEvalRunCost(run.ID, nil, nil); err != nil {
		t.Fatalf("null cost: %v", err)
	}
	reloaded, _ = db.GetEvalRun(run.ID)
	if reloaded.EstimatedCost != "" {
		t.Fatalf("null cost must read empty, got %q", reloaded.EstimatedCost)
	}
}

// TestCampaignCostRowsAvgTPS pins the GH #178 speed aggregate: the cost
// row's avg_tps is output tokens per second over the model's answered
// samples of the run, null when every answer was sub-millisecond.
func TestCampaignCostRowsAvgTPS(t *testing.T) {
	db := openTestDB(t)
	run, model := setupEvalRunForAnswers(t, db)
	text := "x"
	if _, err := db.CreateEvalAnswer(EvalAnswer{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: 1, SampleNo: 1, Status: EvalAnswerAnswered, AnswerText: &text,
		LatencyMs: 2000, OutputTokens: new(400),
	}); err != nil {
		t.Fatalf("seed slow answer: %v", err)
	}
	if _, err := db.CreateEvalAnswer(EvalAnswer{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: 2, SampleNo: 1, Status: EvalAnswerAnswered, AnswerText: &text,
		LatencyMs: 1000, OutputTokens: new(200),
	}); err != nil {
		t.Fatalf("seed fast answer: %v", err)
	}
	// A result row puts the model on the cost board.
	score := 1.0
	if _, err := db.CreateEvalResult(EvalResult{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: 1, Score: &score, VerdictProfile: VerdictProfileV2,
	}); err != nil {
		t.Fatalf("seed result: %v", err)
	}

	rows, err := db.ListCampaignCostRows(run.CampaignID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("cost rows: %v (n=%d)", err, len(rows))
	}
	if rows[0].AvgTPS == nil {
		t.Fatal("avg_tps must be computed from measured answers")
	}
	// (400 + 200) tokens / (2000 + 1000) ms * 1000 = 200 tps.
	if got := *rows[0].AvgTPS; got < 199.9 || got > 200.1 {
		t.Errorf("avg_tps = %v, want 200", got)
	}
}

// CREATE TABLE IF NOT EXISTS statements and the ensureColumn path must both
// be no-ops on an already-migrated database.
func TestEvalAnswerMigrationIdempotent(t *testing.T) {
	path := t.TempDir() + "/test.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second open (migration must be idempotent): %v", err)
	}
	defer db2.Close()
	for _, table := range []string{"eval_answers", "eval_judge_scores"} {
		var n int
		if err := db2.conn.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s missing after reopen (n=%d, %v)", table, n, err)
		}
	}
}
