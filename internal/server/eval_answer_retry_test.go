package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// resultByCaseID finds one model's result row for a case in a run detail.
func resultByCaseID(t *testing.T, run map[string]interface{}, modelID string, caseID int64) map[string]interface{} {
	t.Helper()
	for _, r := range run["results"].([]interface{}) {
		rm := r.(map[string]interface{})
		if rm["model_id"] == modelID && int64(rm["case_id"].(float64)) == caseID {
			return rm
		}
	}
	t.Fatalf("no result for model %s case %d in run %v", modelID, caseID, run["id"])
	return nil
}

// lastCaseID returns the largest case id of the suite — the custom case just
// appended via the case API.
func lastCaseID(t *testing.T, base string, suiteID int64) int64 {
	t.Helper()
	resp := doGet(t, base+"/api/suites")
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	for _, s := range suites {
		if int64(s["id"].(float64)) != suiteID {
			continue
		}
		var max int64
		for _, c := range s["cases"].([]interface{}) {
			id := int64(c.(map[string]interface{})["id"].(float64))
			if id > max {
				max = id
			}
		}
		return max
	}
	t.Fatalf("suite %d not found", suiteID)
	return 0
}

// TestAnswerRetrySucceedsOnSecondAttempt scripts the first answer call to
// fail and asserts the automatic retry (GH #27) scores the sample normally,
// with the detail honestly reporting the second attempt.
func TestAnswerRetrySucceedsOnSecondAttempt(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	prompt := "RETRY-OK-MARKER 请只回答四个字:紫电青霜"
	createRuleCase(t, ts.URL, suiteID, prompt, "紫电青霜", nil)
	caseID := lastCaseID(t, ts.URL, suiteID)

	stub.setAnswerSeq("RETRY-OK-MARKER", "紫电青霜")
	stub.failNextCalls("smart-model", "RETRY-OK-MARKER", 1)

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	result := resultByCaseID(t, run, "smart-model", caseID)
	if result["score"] != 1.0 {
		t.Errorf("retried case score = %v, want 1 (detail: %v)", result["score"], result["verdict_detail"])
	}
	if got := stub.callCount("smart-model", prompt); got != 2 {
		t.Errorf("answer calls = %d, want 2 (initial failure + retry)", got)
	}
	detail, _ := result["verdict_detail"].(string)
	if !strings.Contains(detail, "attempt 2") {
		t.Errorf("verdict_detail must not claim a first-try success, got %q", detail)
	}
}

// TestAnswerRetryBothAttemptsFail scripts both attempts to fail and asserts
// the case lands as a null score whose detail names the two attempts and the
// last failure reason (GH #27).
func TestAnswerRetryBothAttemptsFail(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	prompt := "RETRY-FAIL-MARKER 请只回答四个字:紫电青霜"
	createRuleCase(t, ts.URL, suiteID, prompt, "紫电青霜", nil)
	caseID := lastCaseID(t, ts.URL, suiteID)

	stub.setAnswerSeq("RETRY-FAIL-MARKER", "紫电青霜")
	stub.failNextCalls("smart-model", "RETRY-FAIL-MARKER", 2)

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "smart-model", caseID)
	if result["score"] != nil {
		t.Errorf("twice-failed case score = %v, want null", result["score"])
	}
	if got := stub.callCount("smart-model", prompt); got != 2 {
		t.Errorf("answer calls = %d, want exactly 2 (no third attempt)", got)
	}
	detail, _ := result["verdict_detail"].(string)
	if !strings.Contains(detail, "after 2 attempts") {
		t.Errorf("verdict_detail must name the two attempts, got %q", detail)
	}
	if !strings.Contains(detail, "503") {
		t.Errorf("verdict_detail must carry the last failure reason, got %q", detail)
	}
}

// TestJudgeCallIsNotRetried scripts the judge call to fail and asserts the
// judge path keeps its single-attempt semantics (GH #27: the retry covers
// answer calls only; a failed judge stays a null score, W7).
func TestJudgeCallIsNotRetried(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")
	// Retire the seeded cases: with the jury riding the subject's own model
	// (GH #175), judge calls are indistinguishable from answer calls by
	// model — the single judge case keeps the call accounting exact.
	retireSuiteCases(t, db, suiteID)

	prompt := "JUDGE-FAIL-MARKER 随便聊聊天气"
	resp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       prompt,
		"verdict_type": "judge",
		"rubric":       "回答与天气相关得 1 分,否则 0 分。",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create judge case: expected 201, got %d", resp.StatusCode)
	}
	caseID := lastCaseID(t, ts.URL, suiteID)

	stub.resetCalls()
	// The jury is the subject itself (GH #175: the only candidate on its
	// hub), so the judge failure is scripted on smart-model's judge prompt.
	stub.failNextCalls("smart-model", "你是评估裁判", 1)

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "smart-model", caseID)
	if result["score"] != nil {
		t.Errorf("judge-failed case score = %v, want null", result["score"])
	}
	// 3 probe rounds + 1 answer call + exactly 1 judge call (the judge is
	// never retried).
	if got := stub.callTotal("smart-model"); got != 5 {
		t.Errorf("smart-model calls = %d, want 5 (3 probes + 1 answer + 1 judge, judge never retried)", got)
	}
	detail, _ := result["verdict_detail"].(string)
	if !strings.Contains(detail, "judge") {
		t.Errorf("verdict_detail must report the judge failure, got %q", detail)
	}
	// The answer call succeeded on the first try — no answer retry either.
	if got := stub.callCount("smart-model", prompt); got != 1 {
		t.Errorf("answer calls = %d, want 1 (answer succeeded first try)", got)
	}
}

// TestAnswerRetryDetailKeepsSampleFraming guards the sample-N/M framing of
// verdict_detail around the retry wording (GH #27 detail evolution rides the
// existing sample format).
func TestAnswerRetryDetailKeepsSampleFraming(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	prompt := "RETRY-FRAME-MARKER 请只回答四个字:紫电青霜"
	createRuleCase(t, ts.URL, suiteID, prompt, "紫电青霜", nil)
	caseID := lastCaseID(t, ts.URL, suiteID)

	stub.setAnswerSeq("RETRY-FRAME-MARKER", "紫电青霜")
	stub.failNextCalls("smart-model", "RETRY-FRAME-MARKER", 1)

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "smart-model", caseID)
	detail, _ := result["verdict_detail"].(string)
	if !strings.HasPrefix(detail, "sample 1/1: ") {
		t.Errorf("verdict_detail must keep the sample framing, got %q", detail)
	}
}
