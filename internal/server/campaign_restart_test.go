package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestFailedBatchOps pins the 2026-08-05 ops ruling: a failed batch shows
// every run's failure reason in the campaign detail, and restart re-runs
// the exact same plan as a new campaign.
func TestFailedBatchOps(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	brokenID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	// The probe gate excludes the broken model; an all-unreachable run
	// settles failed synchronously, so trigger by hand (triggerEval asserts
	// running/done).
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

	// The campaign detail names the failure reason per run.
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
	if !strings.Contains(reason, "probe gate") {
		t.Errorf("failure_reason = %q, want the probe-gate reason", reason)
	}

	// Restart with the model healthy: the new campaign carries the same
	// plan and completes.
	stub.markBroken("broken-model", false)
	rest := doPost(t, ts.URL+"/api/campaigns/"+itoa(campaignID)+"/restart", map[string]interface{}{})
	if rest.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(rest.Body)
		rest.Body.Close()
		t.Fatalf("restart: expected 202, got %d: %s", rest.StatusCode, body)
	}
	var env2 envelope
	_ = json.NewDecoder(rest.Body).Decode(&env2)
	rest.Body.Close()
	var fresh map[string]interface{}
	_ = json.Unmarshal(env2.Data, &fresh)
	freshID := int64(fresh["id"].(float64))
	if freshID == campaignID {
		t.Fatal("restart must create a new campaign")
	}
	waitCampaignStatus(t, ts.URL, freshID, "done")

	freshRuns := fresh["runs"].([]interface{})
	if len(freshRuns) != 1 || int64(freshRuns[0].(map[string]interface{})["suite_id"].(float64)) != suiteID {
		t.Errorf("restarted plan = %v, want the same suite", freshRuns)
	}
}
