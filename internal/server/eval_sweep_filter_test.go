package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// triggerFilteredSweep posts POST /api/evals with the given body and returns
// the created campaign decoded from the 202 response.
func triggerFilteredSweep(t *testing.T, base string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := doPost(t, base+"/api/evals", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/evals %v: expected 202, got %d: %s", body, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode sweep campaign: %v", err)
	}
	var campaign map[string]interface{}
	if err := json.Unmarshal(env.Data, &campaign); err != nil {
		t.Fatalf("unmarshal sweep campaign: %v", err)
	}
	return campaign
}

// runSuiteIDs returns the suite ID of each run in the campaign.
func runSuiteIDs(runs []map[string]interface{}) map[int64]bool {
	ids := map[int64]bool{}
	for _, run := range runs {
		ids[int64(run["suite_id"].(float64))] = true
	}
	return ids
}

// TestSweepSuiteIDsSubset pins the suite_ids half of the extended trigger
// contract: a sweep naming a subset of the rotation creates exactly one run
// per named suite and touches no other suite.
func TestSweepSuiteIDsSubset(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	mmluID := suiteIDByKey(t, ts.URL, "mmlu")
	gsm8kID := suiteIDByKey(t, ts.URL, "gsm8k")

	campaign := triggerFilteredSweep(t, ts.URL, map[string]interface{}{
		"suite_ids": []int64{mmluID, gsm8kID},
	})
	final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), store.CampaignStatusDone)

	runs := campaignRuns(t, final)
	if len(runs) != 2 {
		t.Fatalf("filtered sweep has %d runs, want 2 (mmlu, gsm8k)", len(runs))
	}
	seen := runSuiteIDs(runs)
	if !seen[mmluID] || !seen[gsm8kID] {
		t.Errorf("run suites = %v, want {%d, %d}", seen, mmluID, gsm8kID)
	}
	for _, run := range runs {
		if run["status"] != "done" {
			t.Errorf("run %v status = %v, want done", run["id"], run["status"])
		}
		models := runModelIDs(t, ts.URL, int64(run["id"].(float64)))
		if len(models) != 1 || !models["smart-model"] {
			t.Errorf("run %v covered models %v, want {smart-model}", run["id"], models)
		}
	}
}

// TestSweepSuiteIDsRejection pins the 400 branches: an all-invalid
// suite_ids list is rejected, and a mixed list keeps only the valid suites.
func TestSweepSuiteIDsRejection(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	mmluID := suiteIDByKey(t, ts.URL, "mmlu")

	t.Run("all_invalid", func(t *testing.T) {
		resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
			"suite_ids": []int64{99998, 99999},
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		if got := len(listCampaigns(t, ts.URL)); got != 0 {
			t.Errorf("rejected sweep must not create a campaign, found %d", got)
		}
	})

	t.Run("mixed_valid_invalid", func(t *testing.T) {
		campaign := triggerFilteredSweep(t, ts.URL, map[string]interface{}{
			"suite_ids": []int64{mmluID, 99999},
		})
		final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), store.CampaignStatusDone)
		runs := campaignRuns(t, final)
		if len(runs) != 1 {
			t.Fatalf("mixed sweep has %d runs, want 1 (mmlu only)", len(runs))
		}
		if got := int64(runs[0]["suite_id"].(float64)); got != mmluID {
			t.Errorf("run suite_id = %d, want %d (mmlu)", got, mmluID)
		}
	})
}

// TestSweepModelIDsOverride pins the model_ids half: an explicit model set
// replaces the eval_enabled candidate list — an opted-out model named
// explicitly is evaluated (the manual override path), and unnamed models
// are excluded.
func TestSweepModelIDsOverride(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "on-model")
	offID := createEvalModel(t, ts.URL, stub.URL, "off-model")
	patchEvalEnabled(t, ts.URL, offID, false)
	stub.resetCalls()

	campaign := triggerFilteredSweep(t, ts.URL, map[string]interface{}{
		"model_ids": []int64{offID},
	})
	final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), store.CampaignStatusDone)

	runs := campaignRuns(t, final)
	if len(runs) != suiteCount(t, ts.URL) {
		t.Fatalf("override sweep has %d runs, want one per suite", len(runs))
	}
	for _, run := range runs {
		models := runModelIDs(t, ts.URL, int64(run["id"].(float64)))
		if len(models) != 1 || !models["off-model"] {
			t.Errorf("run %v covered models %v, want {off-model} only", run["id"], models)
		}
	}
	if stub.sawModel("on-model") {
		t.Error("sweep called the unnamed eval-enabled model — explicit model_ids must replace the candidate list")
	}
	if !stub.sawModel("off-model") {
		t.Error("sweep never called the explicitly named opted-out model")
	}
}

// TestSweepModelIDsValidation pins the explicit-set validation caliber:
// unknown models 404 and non-chat models 400, same as the single-suite
// path, and neither creates a campaign.
func TestSweepModelIDsValidation(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")

	t.Run("unknown_model", func(t *testing.T) {
		resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
			"model_ids": []int64{99999},
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("non_chat_model", func(t *testing.T) {
		if err := db.SetModelCapability(modelID, "non_chat"); err != nil {
			t.Fatalf("stage non_chat capability: %v", err)
		}
		resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
			"model_ids": []int64{modelID},
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	if got := len(listCampaigns(t, ts.URL)); got != 0 {
		t.Errorf("rejected sweeps must not create a campaign, found %d", got)
	}
}

// TestSweepSuiteIDsAndModelIDs pins the combined form: one suite against an
// explicit one-model set produces exactly one run.
func TestSweepSuiteIDsAndModelIDs(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")
	mmluID := suiteIDByKey(t, ts.URL, "mmlu")
	models := listModelIDs(t, ts.URL)

	campaign := triggerFilteredSweep(t, ts.URL, map[string]interface{}{
		"suite_ids": []int64{mmluID},
		"model_ids": []int64{models["chat-two"]},
	})
	final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), store.CampaignStatusDone)

	runs := campaignRuns(t, final)
	if len(runs) != 1 {
		t.Fatalf("combined sweep has %d runs, want 1", len(runs))
	}
	if got := int64(runs[0]["suite_id"].(float64)); got != mmluID {
		t.Errorf("run suite_id = %d, want %d (mmlu)", got, mmluID)
	}
	covered := runModelIDs(t, ts.URL, int64(runs[0]["id"].(float64)))
	if len(covered) != 1 || !covered["chat-two"] {
		t.Errorf("run covered models %v, want {chat-two} only", covered)
	}
}

// listModelIDs maps model_id strings to database IDs via GET /api/models.
func listModelIDs(t *testing.T, base string) map[string]int64 {
	t.Helper()
	resp := doGet(t, base+"/api/models")
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)
	ids := map[string]int64{}
	for _, m := range models {
		ids[m["model_id"].(string)] = int64(m["id"].(float64))
	}
	return ids
}
