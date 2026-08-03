package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// postRetryUnits calls POST /api/campaigns/{id}/retry-units (targeted
// retry): the explicit (model, case) units to re-evaluate.
func postRetryUnits(t *testing.T, base string, campaignID int64, items []map[string]int64) *http.Response {
	t.Helper()
	return doPost(t, fmt.Sprintf("%s/api/campaigns/%d/retry-units", base, campaignID),
		map[string]interface{}{"items": items})
}

// retryUnit builds one retry-units request item.
func retryUnit(modelDBID, caseID int64) map[string]int64 {
	return map[string]int64{"model_db_id": modelDBID, "case_id": caseID}
}

// retryUnitsAck decodes the {accepted, skipped} response body.
func retryUnitsAck(t *testing.T, resp *http.Response) (accepted, skipped int) {
	t.Helper()
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var ack struct {
		Accepted int `json:"accepted"`
		Skipped  int `json:"skipped"`
	}
	if err := json.Unmarshal(env.Data, &ack); err != nil {
		t.Fatalf("decode retry-units ack: %v", err)
	}
	return ack.Accepted, ack.Skipped
}

// TestRetryUnitsRetriesOnlyRequestedNulls covers the targeted retry's happy
// path: a settled batch whose broken model holds two null-score units is
// asked to re-run exactly one of them plus one already-scored unit. The
// scored unit is skipped and counted (never re-asked — W7), the requested
// null unit refills once the model recovers, and the unrequested null unit
// keeps its null row and stays retryable on its own.
func TestRetryUnitsRetriesOnlyRequestedNulls(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	brokenID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markCaseBroken("broken-model", true)
	// Custom exact-rule bank (two cases in mmlu): the default stub answer
	// scores 1 once the model recovers, so a refill is observable as
	// null -> 1.
	installCustomBank(t, ts.URL, db, map[string]int{"mmlu": 2})
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	runID := triggerEval(t, ts.URL, suiteID, smartID, brokenID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	before := runDetail(t, ts.URL, runID)
	smartBefore := resultsByModel(before, "smart-model")
	brokenBefore := resultsByModel(before, "broken-model")
	if len(smartBefore) != 2 || len(brokenBefore) != 2 {
		t.Fatalf("expected 2 results per model, got smart=%d broken=%d", len(smartBefore), len(brokenBefore))
	}
	for _, r := range brokenBefore {
		if r["score"] != nil {
			t.Fatalf("broken case %v score = %v, want null before retry", r["case_id"], r["score"])
		}
	}
	case1 := int64(brokenBefore[0]["case_id"].(float64))
	case2 := int64(brokenBefore[1]["case_id"].(float64))
	scoredCase := int64(smartBefore[0]["case_id"].(float64))

	// The model recovers before the retry, so the retried unit refills.
	stub.markCaseBroken("broken-model", false)

	// One null unit plus one already-scored unit: accepted=1, skipped=1.
	resp := postRetryUnits(t, ts.URL, campaignID, []map[string]int64{
		retryUnit(brokenID, case1),
		retryUnit(smartID, scoredCase),
	})
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("retry-units: expected 202, got %d (%s)", resp.StatusCode, body)
	}
	accepted, skipped := retryUnitsAck(t, resp)
	if accepted != 1 || skipped != 1 {
		t.Errorf("retry-units ack = {accepted:%d skipped:%d}, want {1 1}", accepted, skipped)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	mid := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(smartBefore, resultsByModel(mid, "smart-model")) {
		t.Error("scored results must stay byte-identical across a targeted retry (W7)")
	}
	brokenMid := map[int64]interface{}{}
	for _, r := range resultsByModel(mid, "broken-model") {
		brokenMid[int64(r["case_id"].(float64))] = r["score"]
	}
	if brokenMid[case1] != 1.0 {
		t.Errorf("requested unit score = %v, want 1 after refill", brokenMid[case1])
	}
	if brokenMid[case2] != nil {
		t.Errorf("unrequested unit score = %v, want null (only requested units re-run)", brokenMid[case2])
	}
	report := campaignReport(t, ts.URL, campaignID)
	if got := int(report["failed_results"].(float64)); got != 1 {
		t.Errorf("failed_results after targeted retry = %d, want 1 (the unrequested null stays)", got)
	}

	// The remaining null unit stays retryable on its own.
	resp = postRetryUnits(t, ts.URL, campaignID, []map[string]int64{retryUnit(brokenID, case2)})
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("second retry-units: expected 202, got %d (%s)", resp.StatusCode, body)
	}
	accepted, skipped = retryUnitsAck(t, resp)
	if accepted != 1 || skipped != 0 {
		t.Errorf("second retry-units ack = {accepted:%d skipped:%d}, want {1 0}", accepted, skipped)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	after := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(smartBefore, resultsByModel(after, "smart-model")) {
		t.Error("scored results must stay byte-identical across the second targeted retry (W7)")
	}
	for _, r := range resultsByModel(after, "broken-model") {
		if r["score"] != 1.0 {
			t.Errorf("retried broken case %v score = %v, want 1", r["case_id"], r["score"])
		}
	}
	report = campaignReport(t, ts.URL, campaignID)
	if got := report["failed_results"].(float64); got != 0 {
		t.Errorf("failed_results after refill = %v, want 0", got)
	}
}

// TestRetryUnitsGuards covers the rejection paths: unknown campaign → 404;
// empty, oversized or non-positive item lists → 400; anonymous → 401; a
// unit the campaign already judged → 200 with {accepted:0, skipped:1} and
// no state change (the campaign stays done).
func TestRetryUnitsGuards(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	scored := resultsByModel(runDetail(t, ts.URL, runID), "smart-model")
	if len(scored) == 0 {
		t.Fatal("expected scored results for the clean sweep")
	}
	scoredCase := int64(scored[0]["case_id"].(float64))

	resp := postRetryUnits(t, ts.URL, 999999, []map[string]int64{retryUnit(smartID, scoredCase)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown campaign: expected 404, got %d", resp.StatusCode)
	}

	resp = postRetryUnits(t, ts.URL, campaignID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty items: expected 400, got %d", resp.StatusCode)
	}

	oversized := make([]map[string]int64, 0, 201)
	for i := 0; i < 201; i++ {
		oversized = append(oversized, retryUnit(smartID, int64(i+1)))
	}
	resp = postRetryUnits(t, ts.URL, campaignID, oversized)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized items: expected 400, got %d", resp.StatusCode)
	}

	resp = postRetryUnits(t, ts.URL, campaignID, []map[string]int64{retryUnit(0, scoredCase)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("non-positive model_db_id: expected 400, got %d", resp.StatusCode)
	}

	// An already-judged unit: skipped and counted, nothing re-runs — 200
	// with the counts, and the campaign never leaves done.
	resp = postRetryUnits(t, ts.URL, campaignID, []map[string]int64{retryUnit(smartID, scoredCase)})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("all-skipped retry-units: expected 200, got %d (%s)", resp.StatusCode, body)
	}
	accepted, skipped := retryUnitsAck(t, resp)
	if accepted != 0 || skipped != 1 {
		t.Errorf("all-skipped ack = {accepted:%d skipped:%d}, want {0 1}", accepted, skipped)
	}
	if got := getCampaign(t, ts.URL, campaignID)["status"]; got != "done" {
		t.Errorf("campaign status after all-skipped retry-units = %v, want done (no state change)", got)
	}

	// Anonymous callers never reach the handler.
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/api/campaigns/%d/retry-units", ts.URL, campaignID),
		bytes.NewReader(nil))
	anon, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("anonymous POST: %v", err)
	}
	anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous retry-units: expected 401, got %d", anon.StatusCode)
	}
}

// TestRetryUnitsRequiresSettledCampaignAndMutex covers the two 409
// preconditions: the target campaign must be settled (a running one
// conflicts), and the GH #153 cross-campaign mutex bars the retry while any
// other campaign is active.
func TestRetryUnitsRequiresSettledCampaignAndMutex(t *testing.T) {
	// Async eval: campaign A settles carrying a failed unit while campaign B
	// blocks mid-run; drained by stub.release + waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	brokenID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markCaseBroken("broken-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "mmlu")

	// Campaign A settles done with the broken model's null-score units.
	runAID := triggerEval(t, ts.URL, suiteID, smartID, brokenID)
	runA := waitEvalDone(t, ts.URL, runAID)
	campaignA := campaignIDOfRun(t, runA)
	waitCampaignStatus(t, ts.URL, campaignA, "done")
	brokenResults := resultsByModel(runDetail(t, ts.URL, runAID), "broken-model")
	if len(brokenResults) == 0 {
		t.Fatal("expected null-score results for the broken model")
	}
	nullCase := int64(brokenResults[0]["case_id"].(float64))

	// Campaign B starts and blocks mid-run.
	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)
	runBID := triggerEval(t, ts.URL, suiteID, smartID)
	waitFor(t, "campaign B's first call reaching the stub", func() bool {
		return stub.sawModel("smart-model")
	})
	campaignB := campaignIDOfRun(t, runDetail(t, ts.URL, runBID))

	// The running campaign itself is not settled → 409.
	resp := postRetryUnits(t, ts.URL, campaignB, []map[string]int64{retryUnit(smartID, nullCase)})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("retry-units on running campaign: expected 409, got %d", resp.StatusCode)
	}

	// Another campaign active: the cross-campaign mutex conflicts even
	// though campaign A itself is settled → 409.
	resp = postRetryUnits(t, ts.URL, campaignA, []map[string]int64{retryUnit(brokenID, nullCase)})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("retry-units during another active campaign: expected 409, got %d (%s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "already running") {
		t.Errorf("409 body should name the active-campaign conflict, got %s", body)
	}

	stub.release()
	waitCampaignStatus(t, ts.URL, campaignB, "done")
}
