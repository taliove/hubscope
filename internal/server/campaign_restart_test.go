package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestFailedBatchOps pins the 2026-08-05 ops rulings: a failed batch shows
// every run's failure reason in the campaign detail, and restart RESUMES
// the same campaign — answered units keep their results (row IDs
// untouched), only missing or null-score units re-run.
func TestFailedBatchOps(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, ts.URL, suiteID, "RESUME-A:请作答", "好的", nil)
	createRuleCase(t, ts.URL, suiteID, "RESUME-B:请作答", "好的", nil)
	createRuleCase(t, ts.URL, suiteID, "RESUME-C:请作答", "好的", nil)

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	// Snapshot the scored rows, then drop two units to fake the
	// interruption's gaps.
	before := runDetail(t, ts.URL, runID)
	beforeRows := resultsByModel(before, "smart-model")
	if len(beforeRows) != 3 {
		t.Fatalf("results before resume = %d, want 3", len(beforeRows))
	}
	keptIDs := map[int64]bool{}
	var dropped []int64
	for i, r := range beforeRows {
		id := int64(r["id"].(float64))
		if i == 0 {
			dropped = append(dropped, id)
			if err := db.DeleteUnitResult(runID, modelID, int64(r["case_id"].(float64))); err != nil {
				t.Fatalf("drop unit: %v", err)
			}
			continue
		}
		keptIDs[id] = true
	}

	rest := doPost(t, ts.URL+"/api/campaigns/"+itoa(campaignID)+"/restart", map[string]interface{}{})
	if rest.StatusCode != http.StatusAccepted {
		t.Fatalf("restart: expected 202, got %d", rest.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(rest.Body).Decode(&env)
	rest.Body.Close()
	var resumed map[string]interface{}
	_ = json.Unmarshal(env.Data, &resumed)
	if int64(resumed["id"].(float64)) != campaignID {
		t.Errorf("resume must continue the same campaign, got new id %v", resumed["id"])
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")

	after := runDetail(t, ts.URL, runID)
	afterRows := resultsByModel(after, "smart-model")
	if len(afterRows) != 3 {
		t.Fatalf("results after resume = %d, want 3 (gaps refilled)", len(afterRows))
	}
	for _, r := range afterRows {
		id := int64(r["id"].(float64))
		if r["score"] != 1.0 {
			t.Errorf("resumed row %d score = %v, want 1", id, r["score"])
		}
		if keptIDs[id] {
			continue // untouched original row
		}
	}
	// Every originally-kept row must still be present with its original ID
	// (resume never touches answered units).
	afterIDs := map[int64]bool{}
	for _, r := range afterRows {
		afterIDs[int64(r["id"].(float64))] = true
	}
	for id := range keptIDs {
		if !afterIDs[id] {
			t.Errorf("previously scored row %d vanished across resume", id)
		}
	}
	_ = dropped
}

// TestFailedBatchFailureReason pins the ops-ruling read path: the campaign
// detail carries each failed run's reason from its task log.
func TestFailedBatchFailureReason(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	brokenID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	// The probe gate excludes the broken model; an all-unreachable run
	// settles failed synchronously, so trigger by hand.
	resp0 := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{brokenID},
	})
	if resp0.StatusCode != http.StatusAccepted {
		t.Fatalf("trigger: expected 202, got %d", resp0.StatusCode)
	}
	var env0 envelope
	_ = json.NewDecoder(resp0.Body).Decode(&env0)
	resp0.Body.Close()
	var campaign0 map[string]interface{}
	_ = json.Unmarshal(env0.Data, &campaign0)
	runID := int64(campaign0["runs"].([]interface{})[0].(map[string]interface{})["id"].(float64))
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed (all-unreachable gate)", run["status"])
	}
	campaignID := int64(run["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, "failed")

	resp := doGet(t, ts.URL+"/api/campaigns/"+itoa(campaignID))
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var detail map[string]interface{}
	_ = json.Unmarshal(env.Data, &detail)
	runs := detail["runs"].([]interface{})
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	reason, _ := runs[0].(map[string]interface{})["failure_reason"].(string)
	if reason == "" {
		t.Error("failure_reason must name why the run failed")
	}
}
