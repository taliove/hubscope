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

// postRetryFailed calls POST /api/campaigns/{id}/retry-failed (GH #28).
func postRetryFailed(t *testing.T, base string, campaignID int64) *http.Response {
	t.Helper()
	return doPost(t, fmt.Sprintf("%s/api/campaigns/%d/retry-failed", base, campaignID), nil)
}

// runDetail fetches GET /api/evals/{id}.
func runDetail(t *testing.T, base string, runID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, runID))
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	return run
}

// campaignReportField fetches the campaign report and returns it decoded.
func campaignReport(t *testing.T, base string, campaignID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/campaigns/%d/report", base, campaignID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET report: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var report map[string]interface{}
	_ = json.Unmarshal(env.Data, &report)
	return report
}

// campaignIDOfRun resolves the campaign of a run detail payload.
func campaignIDOfRun(t *testing.T, run map[string]interface{}) int64 {
	t.Helper()
	id, ok := run["campaign_id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("run campaign_id missing: %v", run)
	}
	return int64(id)
}

// TestRetryFailedRefillsNullScoresOnly covers the happy path (GH #28): a
// settled batch with a broken model retries its failed cells, scored results
// stay byte-identical throughout (W7), and once the hub recovers a second
// retry fills every null — the batch ends fully judged (complete=true).
func TestRetryFailedRefillsNullScoresOnly(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	brokenID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")

	runID := triggerEval(t, ts.URL, suiteID, smartID, brokenID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	// Every broken-model case failed (twice, via the GH #27 answer retry) and
	// landed as a null score.
	before := runDetail(t, ts.URL, runID)
	smartBefore := resultsByModel(before, "smart-model")
	brokenBefore := resultsByModel(before, "broken-model")
	if len(smartBefore) == 0 || len(brokenBefore) == 0 {
		t.Fatalf("expected results for both models, got smart=%d broken=%d", len(smartBefore), len(brokenBefore))
	}
	for _, r := range brokenBefore {
		if r["score"] != nil {
			t.Fatalf("broken case %v score = %v, want null before retry", r["case_id"], r["score"])
		}
	}

	// The report exposes the failure count driving the retry button.
	report := campaignReport(t, ts.URL, campaignID)
	if got := int(report["failed_results"].(float64)); got != len(brokenBefore) {
		t.Errorf("report failed_results = %d, want %d", got, len(brokenBefore))
	}

	// Retry while the model is still broken: accepted, re-settles to done,
	// failures remain — and nothing scored moved a byte.
	resp := postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("retry-failed: expected 202, got %d", resp.StatusCode)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")
	mid := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(smartBefore, resultsByModel(mid, "smart-model")) {
		t.Error("scored results must stay byte-identical across a retry (W7)")
	}

	// The model recovers: the second retry fills every null, again leaving
	// the scored rows untouched.
	stub.markBroken("broken-model", false)
	resp = postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("second retry-failed: expected 202, got %d", resp.StatusCode)
	}
	final := waitCampaignStatus(t, ts.URL, campaignID, "done")
	if final["status"] != "done" {
		t.Fatalf("campaign status after retry = %v, want done", final["status"])
	}

	after := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(smartBefore, resultsByModel(after, "smart-model")) {
		t.Error("scored results must stay byte-identical across the refill retry (W7)")
	}
	for _, r := range resultsByModel(after, "broken-model") {
		if r["score"] != 1.0 {
			t.Errorf("retried broken case %v score = %v, want 1", r["case_id"], r["score"])
		}
	}

	// The batch is now fully judged: no failures left, every row complete.
	report = campaignReport(t, ts.URL, campaignID)
	if got := report["failed_results"].(float64); got != 0 {
		t.Errorf("report failed_results after refill = %v, want 0", got)
	}
	for _, row := range report["rows"].([]interface{}) {
		rm := row.(map[string]interface{})
		if rm["complete"] != true {
			t.Errorf("row %v complete = %v, want true after refill", rm["model_id"], rm["complete"])
		}
	}

	// Nothing left to retry: a third attempt conflicts.
	resp = postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("retry without failures: expected 409, got %d", resp.StatusCode)
	}
}

// TestRetryFailedRequiresSettledCampaign rejects the retry while the batch
// is still running (GH #28: only done/failed batches retry).
func TestRetryFailedRequiresSettledCampaign(t *testing.T) {
	// Async eval: observes the mid-run 409 rejection (all calls blocked);
	// drained by stub.release + waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitFor(t, "first sweep call reaching the stub", func() bool {
		return stub.sawModel("smart-model")
	})

	resp := postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("retry on running campaign: expected 409, got %d", resp.StatusCode)
	}

	stub.release()
	waitCampaignStatus(t, ts.URL, campaignID, "done")
}

// TestRetryFailedGuards covers the remaining rejection paths (GH #28):
// unknown campaign → 404; anonymous → 401; settled batch without failures →
// 409.
func TestRetryFailedGuards(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	resp := postRetryFailed(t, ts.URL, 999999)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown campaign: expected 404, got %d", resp.StatusCode)
	}

	// A clean sweep has no failures to retry.
	resp = postRetryFailed(t, ts.URL, campaignID)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("no-failure campaign: expected 409, got %d (%s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "no failed results") {
		t.Errorf("409 body should explain the conflict, got %s", body)
	}

	// Anonymous callers never reach the handler.
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/api/campaigns/%d/retry-failed", ts.URL, campaignID),
		bytes.NewReader(nil))
	anon, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("anonymous POST: %v", err)
	}
	anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous retry: expected 401, got %d", anon.StatusCode)
	}
}

// TestRetryFailedKeepsScoredBytesOnFailedModel is the failAllCases variant:
// a model with no enabled endpoint records failed results for every case;
// the retry must re-evaluate them without touching the other model's scored
// rows (GH #28 store guarantee: the delete is hardcoded to score IS NULL).
func TestRetryFailedCoversSetupFailureCells(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	ghostID := createEvalModel(t, ts.URL, stub.URL, "ghost-model")
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")

	// Disable the ghost model's endpoints so its whole cell fails at setup.
	resp := doGet(t, ts.URL+"/api/models")
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)
	for _, m := range models {
		mm := m
		if int64(mm["id"].(float64)) != ghostID {
			continue
		}
		eps, _ := mm["endpoints"].([]interface{})
		for _, ep := range eps {
			epm := ep.(map[string]interface{})
			epID := int64(epm["id"].(float64))
			if en, _ := epm["enabled"].(bool); en {
				r := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epID), map[string]interface{}{"enabled": false})
				r.Body.Close()
			}
		}
	}

	runID := triggerEval(t, ts.URL, suiteID, smartID, ghostID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, run)
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	before := runDetail(t, ts.URL, runID)
	if got := resultsByModel(before, "ghost-model"); len(got) == 0 {
		t.Fatal("expected failed result rows for the ghost model")
	}
	for _, r := range resultsByModel(before, "ghost-model") {
		if r["score"] != nil {
			t.Fatalf("ghost case %v score = %v, want null", r["case_id"], r["score"])
		}
	}

	report := campaignReport(t, ts.URL, campaignID)
	if got := report["failed_results"].(float64); got == 0 {
		t.Error("expected failed_results > 0 for the ghost model's failed cell")
	}

	// Retry: the ghost cell fails again at setup (still no enabled endpoint),
	// its null rows are re-recorded, and the batch re-settles.
	resp = postRetryFailed(t, ts.URL, campaignID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("retry-failed: expected 202, got %d", resp.StatusCode)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	after := runDetail(t, ts.URL, runID)
	if !reflect.DeepEqual(resultsByModel(before, "smart-model"), resultsByModel(after, "smart-model")) {
		t.Error("smart-model scored results must stay byte-identical (W7)")
	}
	if got := resultsByModel(after, "ghost-model"); len(got) == 0 {
		t.Error("ghost model must keep failed result rows after the retry")
	}
	for _, r := range resultsByModel(after, "ghost-model") {
		if r["score"] != nil {
			t.Errorf("ghost case %v score = %v, want still null", r["case_id"], r["score"])
		}
	}
}
