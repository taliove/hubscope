package server_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// setupJuryCase builds a one-hub world (subject + three judge candidates
// with IQ-ranked registry overrides), retires the seeded gsm8k cases and
// installs three rule cases plus one judge case, returning the subject's
// model DB ID and the judge case ID. The iq policy makes the jury
// deterministic: [judge-1, judge-2, judge-3]. The custom case set keeps
// judge jobs at exactly one sample's worth — a seeded suite mixes in judge
// cases whose jobs would exhaust the pool and turn a decoupling scenario
// into a backpressure scenario.
func setupJuryCase(t *testing.T, base, stubURL string, db *store.DB) (int64, int64) {
	t.Helper()
	ids := createModelsOnOneHub(t, base, stubURL, "subject", "judge-1", "judge-2", "judge-3")
	subjectID := ids[0]
	putResp := doPut(t, base+"/api/settings", map[string]interface{}{
		"jury_policy": "iq",
		"model_registry_overrides": []map[string]interface{}{
			{"match": "judge-1", "iq_tier": 9},
			{"match": "judge-2", "iq_tier": 8},
			{"match": "judge-3", "iq_tier": 7},
			{"match": "subject", "iq_tier": 10},
		},
	})
	putResp.Body.Close()

	suiteID := suiteIDByKey(t, base, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, base, suiteID, "JURY-RULE-A:请作答", "好的", nil)
	createRuleCase(t, base, suiteID, "JURY-RULE-B:请作答", "好的", nil)
	createRuleCase(t, base, suiteID, "JURY-RULE-C:请作答", "好的", nil)
	resp := doPost(t, base+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "JURY-MEDIAN 谈谈对开源的看法",
		"verdict_type": "judge",
		"rubric":       "观点明确得 1 分,否则 0 分。",
	})
	resp.Body.Close()
	return subjectID, lastCaseID(t, base, suiteID)
}

// TestJuryMedianOfThree pins the spec-0020 headline rule (GH #177): three
// judges score every judge-case sample and the sample score is the median —
// not the mean — so one outlier judge cannot drag the result.
func TestJuryMedianOfThree(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	subjectID, caseID := setupJuryCase(t, ts.URL, stub.URL, db)
	stub.setJudgeSeqFor("judge-1", `{"score": 0.4, "reason": "low"}`)
	stub.setJudgeSeqFor("judge-2", `{"score": 0.6, "reason": "mid"}`)
	stub.setJudgeSeqFor("judge-3", `{"score": 0.9, "reason": "high"}`)

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), subjectID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "subject", caseID)
	if result["score"] != 0.6 {
		t.Errorf("score = %v, want 0.6 (median of 0.4/0.6/0.9, not mean 0.63)", result["score"])
	}
	if result["verdict_profile"] != "v3" {
		t.Errorf("verdict_profile = %v, want v3 (jury caliber, ADR 0016)", result["verdict_profile"])
	}
	detail, _ := result["verdict_detail"].(string)
	for _, want := range []string{"judge-1=0.40", "judge-2=0.60", "judge-3=0.90", "median 0.60 (3 votes)"} {
		if !strings.Contains(detail, want) {
			t.Errorf("verdict_detail missing %q: %s", want, detail)
		}
	}
}

// TestJuryMedianDegradation pins the partial-jury rules (GH #177): one
// failed judge leaves two votes whose mean is the sample score; every
// judge failing leaves the sample unscored (W7 — never zero).
func TestJuryMedianDegradation(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	subjectID, caseID := setupJuryCase(t, ts.URL, stub.URL, db)
	stub.setJudgeSeqFor("judge-1", `{"score": 0.4, "reason": "low"}`)
	stub.setJudgeSeqFor("judge-2", `{"score": 0.6, "reason": "mid"}`)
	stub.setJudgeSeqFor("judge-3", `{"score": 0.9, "reason": "high"}`)
	// judge-3's call fails: two votes remain.
	stub.failNextCalls("judge-3", "你是评估裁判", 1)

	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	runID := triggerEval(t, ts.URL, suiteID, subjectID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "subject", caseID)
	if result["score"] != 0.5 {
		t.Errorf("score = %v, want 0.5 (mean of the two surviving votes 0.4/0.6)", result["score"])
	}
	detail, _ := result["verdict_detail"].(string)
	if !strings.Contains(detail, "judge-3=FAIL") || !strings.Contains(detail, "median 0.50 (2 votes, mean)") {
		t.Errorf("verdict_detail should name the failed judge and the 2-vote rule: %s", detail)
	}
}

// TestJuryAllJudgesFail pins the zero-vote edge: every judge call failed
// leaves the case unscored (W7), never a zero.
func TestJuryAllJudgesFail(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	subjectID, caseID := setupJuryCase(t, ts.URL, stub.URL, db)
	for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
		stub.failNextCalls(j, "你是评估裁判", 1)
	}

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), subjectID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "subject", caseID)
	if result["score"] != nil {
		t.Errorf("score = %v, want null (every judge failed, W7)", result["score"])
	}
	if detail, _ := result["verdict_detail"].(string); !strings.Contains(detail, "unjudged") {
		t.Errorf("verdict_detail should report the unjudged sample: %s", detail)
	}
}

// TestJudgeStageDecoupled pins the pipeline's headline property (GH #176):
// judging does not block answering — with every judge model frozen, the
// subject still answers its whole suite, and the medians land when the
// judges are released.
func TestJudgeStageDecoupled(t *testing.T) {
	// Async eval: observes answers completing while every judge is frozen;
	// drained by releaseModel + waitEvalDone.
	ts, stub, db := setupAsyncEvalEnv(t)
	subjectID, caseID := setupJuryCase(t, ts.URL, stub.URL, db)
	// Creation trial-probes pollute the stub's call counters; reset before
	// arming the gates so blockModelAfter thresholds count from zero.
	stub.resetCalls()
	// Freeze every judge after its probe rounds: the gate completes
	// normally, then no judge call can start.
	for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
		stub.blockModelAfter(j, 3)
	}
	defer func() {
		for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
			stub.releaseModel(j)
		}
	}()

	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	totalCases := enabledCaseCount(t, ts.URL, suiteID)
	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{subjectID},
	})
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var campaign map[string]interface{}
	_ = json.Unmarshal(env.Data, &campaign)
	runID := int64(campaign["runs"].([]interface{})[0].(map[string]interface{})["id"].(float64))

	// The subject answers every case (3 probe rounds + one answer call per
	// case) while the judges' verdict calls sit blocked on the model gates:
	// answering does not wait for judging.
	waitFor(t, "subject answering everything with judges frozen", func() bool {
		return stub.callTotal("subject") >= 3+totalCases
	})
	for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
		if got := stub.callTotal(j); got != 4 {
			t.Errorf("%s calls while frozen = %d, want 4 (3 probe rounds + 1 blocked judge call — judging is decoupled)", j, got)
		}
	}

	for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
		stub.releaseModel(j)
	}
	run := waitEvalDone(t, ts.URL, runID)
	result := resultByCaseID(t, run, "subject", caseID)
	if result["score"] == nil {
		t.Error("judge case must score once the judges are released")
	}
	if result["verdict_profile"] != "v3" {
		t.Errorf("verdict_profile = %v, want v3", result["verdict_profile"])
	}
}

// TestRecoveryDrainsInterruptedJudging pins the crash-recovery contract
// (GH #176): answers persisted before a crash are judged by the recovery
// sweep at startup — only the missing slots are re-issued, completed votes
// are never re-judged, and the run keeps its failed stamp.
func TestRecoveryDrainsInterruptedJudging(t *testing.T) {
	db := openTempDB(t)
	seedTestUser(t, db)
	stub := newEvalStubHub()
	t.Cleanup(stub.Close)

	// World setup through a first server: hub, three models on it, one
	// judge case.
	ts1 := newTestAPIServer(t, db)
	ids := createModelsOnOneHub(t, ts1.URL, stub.URL, "subject", "judge-1", "judge-2")
	subjectID := ids[0]
	suiteID := suiteIDByKey(t, ts1.URL, "gsm8k")
	resp := doPost(t, ts1.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "RECOVER-MARKER 谈谈对开源的看法",
		"verdict_type": "judge",
		"rubric":       "观点明确得 1 分,否则 0 分。",
	})
	resp.Body.Close()
	caseID := lastCaseID(t, ts1.URL, suiteID)
	ts1.Close()

	// Simulate the crash residue by hand: a failed run (store.Open stamps
	// stale running runs failed — the recovery sweep runs after that), its
	// jury snapshot, one persisted answer, and two of three judge votes.
	model, err := db.GetModel(subjectID)
	if err != nil {
		t.Fatalf("load subject: %v", err)
	}
	campaign, err := db.CreateCampaign("manual", []int64{subjectID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	run, err := db.CreateEvalRun(campaign.ID, suiteID, "manual", "subject")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	snapshot := fmt.Sprintf(`{"policy":"iq","juries":{"%d":["judge-1","judge-2","subject"]}}`, subjectID)
	if err := db.SetEvalRunJuryModels(run.ID, snapshot); err != nil {
		t.Fatalf("set jury snapshot: %v", err)
	}
	// Mimic the post-crash state store.Open produces: the interrupted run
	// is stamped failed, and the recovery sweep drains what it left behind.
	if err := db.FinishEvalRun(run.ID, "failed", time.Now().UTC()); err != nil {
		t.Fatalf("stamp run failed: %v", err)
	}
	text := "开源很好"
	answerID, err := db.CreateEvalAnswer(store.EvalAnswer{
		EvalRunID: run.ID, ModelDBID: model.ID, ModelID: model.ModelID,
		CaseID: caseID, SampleNo: 1, Status: store.EvalAnswerAnswered, AnswerText: &text,
	})
	if err != nil {
		t.Fatalf("seed answer: %v", err)
	}
	low, high := 0.4, 0.8
	if _, err := db.CreateEvalJudgeScore(store.EvalJudgeScore{AnswerID: answerID, Slot: 0, JudgeModel: "judge-1", Score: &low}); err != nil {
		t.Fatalf("seed vote 0: %v", err)
	}
	if _, err := db.CreateEvalJudgeScore(store.EvalJudgeScore{AnswerID: answerID, Slot: 2, JudgeModel: "subject", Score: &high}); err != nil {
		t.Fatalf("seed vote 2: %v", err)
	}
	stub.setJudgeSeqFor("judge-2", `{"score": 0.6, "reason": "mid"}`)
	stub.resetCalls()

	// The second server boots the recovery sweep: only judge-2's missing
	// vote is re-issued, and the case result lands with the median.
	_ = newTestAPIServer(t, db)

	if got := stub.callTotal("judge-2"); got != 1 {
		t.Errorf("judge-2 calls = %d, want exactly 1 (only the missing slot is re-judged)", got)
	}
	for _, m := range []string{"judge-1", "subject"} {
		if got := stub.callTotal(m); got != 0 {
			t.Errorf("%s calls during recovery = %d, want 0 (completed votes are never re-judged)", m, got)
		}
	}
	run2, err := db.GetEvalRun(run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if run2.Status != "failed" {
		t.Errorf("run status = %v, want failed (recovery rescues results, not the run)", run2.Status)
	}
	results, err := db.ListEvalResults(run.ID)
	if err != nil || len(results) != 1 {
		t.Fatalf("recovered results = %v (n=%d), want exactly 1", err, len(results))
	}
	if results[0].Score == nil || *results[0].Score != 0.6 {
		t.Errorf("recovered score = %v, want 0.6 (median of 0.4/0.6/0.8)", results[0].Score)
	}
	if results[0].VerdictProfile != "v3" {
		t.Errorf("recovered profile = %v, want v3", results[0].VerdictProfile)
	}
}
