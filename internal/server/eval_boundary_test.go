package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// TestJudgeOutOfRangeScoreUnjudged pins the GH #154 judge contract: a judge
// verdict outside the documented 0~1 range is a breached contract, recorded
// as unjudged (null, never counted) — not silently clamped to full marks
// and averaged in.
func TestJudgeOutOfRangeScoreUnjudged(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createJudgeCaseForTest(t, ts.URL, suiteID, "OVERSHOOT:请回答")
	stub.setJudgeSeq("OVERSHOOT", `{"score": 7, "reason": "超纲给分"}`)

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0]["score"] != nil {
		t.Errorf("out-of-range judge score = %v, want null (unjudged)", results[0]["score"])
	}
	detail, _ := results[0]["verdict_detail"].(string)
	if !strings.Contains(detail, "out of range") {
		t.Errorf("verdict detail must name the out-of-range judge score: %q", detail)
	}
}

// TestRetiredModelSkippedFromEval pins the GH #154 boundary: a model
// retired mid-batch is skipped by every cell that has not started yet — no
// Hub calls, no stamped dead rows other views must filter out (retirement
// is an operator decision, not a failure).
func TestRetiredModelSkippedFromEval(t *testing.T) {
	ts, stub, db := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	ghostID := createEvalModel(t, ts.URL, stub.URL, "ghost-model")

	// One custom case per rotation suite keeps the batch small; five
	// suites still overflow the worker pool (4), so the ghost's cells
	// start only after the retirement lands.
	for _, key := range []string{"mmlu", "agieval_zh", "gsm8k", "cruxeval", "ifeval"} {
		suiteID := suiteIDByKey(t, ts.URL, key)
		retireSuiteCases(t, db, suiteID)
		createRuleCase(t, ts.URL, suiteID, "GHOST-"+key+":请作答", "好的", nil)
	}
	stub.resetCalls()

	// In-flight state observed: the probe stage's six rounds (3 per model,
	// GH #174) pass the count-based gate, then the first cell wave —
	// smart-model's four suite cells (GH #169 model-major order: every
	// suite of model 1 before model 2's first cell) — blocks when the
	// ghost retires; every ghost cell starts only after the delete.
	stub.blockCallsAfter(6)
	t.Cleanup(stub.releaseGlobal)
	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitFor(t, "first wave blocked in flight", func() bool {
		return stub.grandTotalCalls() >= 10
	})

	delResp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, ghostID))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("retire model: expected 200/204, got %d", delResp.StatusCode)
	}

	// Drain (ticket 100): terminal status covers every tail write.
	stub.releaseGlobal()
	final := waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone, store.CampaignStatusFailed)
	if final["status"] != store.CampaignStatusDone {
		t.Fatalf("campaign status = %v, want done (skipped cells still execute)", final["status"])
	}
	// Every ghost cell starts after the retirement and places no case call
	// — only the probe stage's three rounds reached it (GH #174);
	// smart-model answers all five suites.
	if got := stub.callTotal("ghost-model"); got != 3 {
		t.Errorf("ghost model calls = %d, want 3 (probe rounds only; all its cells start after the retirement)", got)
	}
	if got := stub.callTotal("smart-model"); got != 8 {
		t.Errorf("smart model calls = %d, want 8 (3 probe rounds + all five suites)", got)
	}
}

// TestManualTriggerRejectsDisabledSuite pins the GH #154 boundary: a suite
// out of the evaluation rotation cannot be triggered manually either (the
// full sweep already skips it via ListEnabledSuites).
func TestManualTriggerRejectsDisabledSuite(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	if err := db.SetSuiteEnabled(suiteID, false); err != nil {
		t.Fatalf("disable suite: %v", err)
	}

	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{modelID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("trigger on disabled suite: expected 400, got %d", resp.StatusCode)
	}
}

// TestRetrySkipsDisabledCase pins the GH #154 boundary: retry-failed only
// re-evaluates cases still in the rotation — a case disabled after the
// original run keeps its null row as history instead of being re-asked.
func TestRetrySkipsDisabledCase(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createJudgeCaseForTest(t, ts.URL, suiteID, "RETRY-A:请回答")
	createJudgeCaseForTest(t, ts.URL, suiteID, "RETRY-B:请回答")
	// Both judge replies are unparseable: both cases stay unjudged (null).
	stub.setJudgeSeq("RETRY-A", "completely unparseable")
	stub.setJudgeSeq("RETRY-B", "completely unparseable")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))
	if rows := resultsByModel(run, "smart-model"); len(rows) != 2 {
		t.Fatalf("initial results = %d, want 2 null rows", len(rows))
	}

	// Disable case A (out of the rotation), fix case B's judge, then retry.
	var caseAID int64
	for _, c := range suiteByKey(t, ts.URL, "gsm8k")["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if strings.HasPrefix(cm["prompt"].(string), "RETRY-A") {
			caseAID = int64(cm["id"].(float64))
		}
	}
	if caseAID == 0 {
		t.Fatal("RETRY-A case id not found")
	}
	if _, err := db.SetCaseEnabled(caseAID, false); err != nil {
		t.Fatalf("disable case A: %v", err)
	}
	stub.setJudgeSeq("RETRY-B", `{"score": 1.0, "reason": "好"}`)
	stub.resetCalls()

	resp := postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("retry-failed: expected 202, got %d", resp.StatusCode)
	}

	// Exactly one answer + one judge call: case B only. Case A's prompt
	// never reaches the Hub again.
	if got := stub.callTotal("smart-model"); got != 1 {
		t.Errorf("answer calls after retry = %d, want 1 (case B only)", got)
	}
	if got := stub.callTotal(store.DefaultJudgeModel); got != 1 {
		t.Errorf("judge calls after retry = %d, want 1 (case B only)", got)
	}

	run = waitEvalDone(t, ts.URL, runID)
	var aRow, bRow map[string]interface{}
	for _, row := range resultsByModel(run, "smart-model") {
		if d, _ := row["verdict_detail"].(string); strings.Contains(d, "unparseable") {
			aRow = row
		} else {
			bRow = row
		}
	}
	if aRow == nil || aRow["score"] != nil {
		t.Errorf("disabled case A row = %v, want its null row kept as history", aRow)
	}
	if bRow == nil || bRow["score"] == nil {
		t.Errorf("retried case B row = %v, want a scored row", bRow)
	}
}
