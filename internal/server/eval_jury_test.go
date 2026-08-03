package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// createModelsOnOneHub registers one hub pointing at the stub plus several
// models on it, returning the model database IDs in order. Jury selection
// rides the hub record, so jury scenarios need models sharing one hub.
func createModelsOnOneHub(t *testing.T, base, stubURL string, modelIDs ...string) []int64 {
	t.Helper()
	resp := doPost(t, base+"/api/hubs", map[string]interface{}{
		"name": "Jury Hub", "base_url": stubURL, "token": "eval-token",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create jury hub: got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var hub map[string]interface{}
	_ = json.Unmarshal(env.Data, &hub)

	out := make([]int64, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		resp = doPost(t, base+"/api/models", map[string]interface{}{
			"hub_id": hub["id"], "model_id": modelID,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create model %s: got %d", modelID, resp.StatusCode)
		}
		_ = json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		var model map[string]interface{}
		_ = json.Unmarshal(env.Data, &model)
		out = append(out, int64(model["id"].(float64)))
	}
	return out
}

// jurySnapshot reads the run detail API and returns its jury snapshot.
func jurySnapshot(t *testing.T, base string, runID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/evals/"+itoa(runID))
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	jury, _ := run["jury_models"].(map[string]interface{})
	return jury
}

// TestJurySelectionSnapshot pins the spec-0020 jury contract end to end
// (GH #175): the jury comes from the subject's own hub only, the policy
// ranks it (iq policy + registry overrides make the ranking
// deterministic), the subject is excluded despite having the highest IQ,
// and the run snapshots the full selection for the API.
func TestJurySelectionSnapshot(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	// One shared hub: the subject plus four judge candidates. The second
	// hub hosts a high-IQ outsider that must never leak into the jury.
	ids := createModelsOnOneHub(t, ts.URL, stub.URL, "subject", "cand-1", "cand-2", "cand-3", "cand-4")
	subjectID := ids[0]
	createModelsOnOneHub(t, ts.URL, stub.URL, "outsider")

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"jury_policy": "iq",
		"model_registry_overrides": []map[string]interface{}{
			{"match": "subject", "iq_tier": 10},
			{"match": "cand-1", "iq_tier": 9},
			{"match": "cand-2", "iq_tier": 8},
			{"match": "cand-3", "iq_tier": 7},
			{"match": "cand-4", "iq_tier": 1},
			{"match": "outsider", "iq_tier": 10},
		},
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put jury settings: expected 200, got %d", putResp.StatusCode)
	}

	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	runID := triggerEval(t, ts.URL, suiteID, subjectID)
	waitEvalDone(t, ts.URL, runID)

	jury := jurySnapshot(t, ts.URL, runID)
	if jury == nil {
		t.Fatal("run detail must carry the jury snapshot")
	}
	if jury["policy"] != "iq" {
		t.Errorf("snapshot policy = %v, want iq", jury["policy"])
	}
	juries, _ := jury["juries"].(map[string]interface{})
	judges, _ := juries[itoa(subjectID)].([]interface{})
	var got []string
	for _, j := range judges {
		got = append(got, j.(string))
	}
	want := []string{"cand-1", "cand-2", "cand-3"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("judges = %v, want %v (subject excluded despite top IQ, outsider on another hub ignored)", got, want)
	}

	// The task log names the picked jury before the cases burned.
	if !probeGateRunLogContains(t, ts.URL, runID, "jury (iq): cand-1, cand-2, cand-3") {
		t.Error("run task log must name the selected jury")
	}
}

// TestJuryShortAndSelfIncluded pins the degraded shapes (GH #175): with
// only one alternative candidate the subject serves on its own jury
// (flagged in the task log) and the short jury is logged, never fatal.
func TestJuryShortAndSelfIncluded(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	ids := createModelsOnOneHub(t, ts.URL, stub.URL, "subject", "cand-1")
	subjectID := ids[0]
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	runID := triggerEval(t, ts.URL, suiteID, subjectID)
	waitEvalDone(t, ts.URL, runID)

	jury := jurySnapshot(t, ts.URL, runID)
	juries, _ := jury["juries"].(map[string]interface{})
	judges, _ := juries[itoa(subjectID)].([]interface{})
	if len(judges) != 2 {
		t.Fatalf("short jury = %v, want both candidates", judges)
	}
	if !probeGateRunLogContains(t, ts.URL, runID, "serves on its own jury") {
		t.Error("task log must flag the self-preference risk")
	}
	if !probeGateRunLogContains(t, ts.URL, runID, "short jury") {
		t.Error("task log must flag the short jury")
	}
}

// TestJuryPolicyValidation pins the settings boundary: unknown policies
// are rejected, the valid four round-trip.
func TestJuryPolicyValidation(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{"jury_policy": "wisest"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown jury_policy: expected 400, got %d", resp.StatusCode)
	}

	for _, policy := range []string{"balanced", "speed", "iq", "cost"} {
		resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{"jury_policy": policy})
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("jury_policy %q: expected 200, got %d", policy, resp.StatusCode)
		}
		getResp := doGet(t, ts.URL+"/api/settings")
		var env envelope
		_ = json.NewDecoder(getResp.Body).Decode(&env)
		getResp.Body.Close()
		var settings map[string]interface{}
		_ = json.Unmarshal(env.Data, &settings)
		if settings["jury_policy"] != policy {
			t.Errorf("jury_policy read-back = %v, want %q", settings["jury_policy"], policy)
		}
	}
}
