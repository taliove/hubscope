package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestFullSweepSkipsModelsWithoutEnabledEndpoints pins the eval population
// rule: a model with no enabled endpoint cannot be called at all, so a full
// sweep must exclude it instead of recording "no enabled endpoint" failures
// for every case.
func TestFullSweepSkipsModelsWithoutEnabledEndpoints(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	orphanID := createEvalModel(t, ts.URL, stub.URL, "orphan-model")

	// Delete the orphan's endpoints through the API, leaving an active chat
	// model that cannot be called.
	resp := doGet(t, ts.URL+"/api/models")
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	resp.Body.Close()
	var models []struct {
		ID        int64 `json:"id"`
		Endpoints []struct {
			ID int64 `json:"id"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(env.Data, &models); err != nil {
		t.Fatalf("unmarshal models: %v", err)
	}
	deleted := 0
	for _, m := range models {
		if m.ID != orphanID {
			continue
		}
		for _, ep := range m.Endpoints {
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, ep.ID), nil)
			if err != nil {
				t.Fatalf("build delete: %v", err)
			}
			dresp, err := authedClient(t, ts.URL).Do(req)
			if err != nil {
				t.Fatalf("delete endpoint %d: %v", ep.ID, err)
			}
			dresp.Body.Close()
			if dresp.StatusCode != http.StatusOK && dresp.StatusCode != http.StatusNoContent {
				t.Fatalf("delete endpoint %d: got %d", ep.ID, dresp.StatusCode)
			}
			deleted++
		}
	}
	if deleted == 0 {
		t.Fatal("orphan model had no endpoints to delete; fixture broken")
	}

	campaign := triggerFullSweep(t, ts.URL)
	final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), "done")
	for _, run := range campaignRuns(t, final) {
		models := runModelIDs(t, ts.URL, int64(run["id"].(float64)))
		if models["orphan-model"] {
			t.Errorf("run %v covers orphan-model, which has no enabled endpoint", run["id"])
		}
		if !models["smart-model"] {
			t.Errorf("run %v lost smart-model, which has enabled endpoints", run["id"])
		}
	}
}
