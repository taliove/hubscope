package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestJudgeModelFromSettings changes settings.judge_model and asserts the
// next run records the new judge and routes its judge calls through it.
func TestJudgeModelFromSettings(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	// One custom judge case (seeded cases retired): the post-cutover bank
	// seeds zero judge cases, so judge-path tests install their own.
	retireSuiteCases(t, db, suiteID)
	createJudgeCaseForTest(t, ts.URL, suiteID, "JUDGE-SETTINGS:请回答")

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"judge_model": "alt-judge-model",
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	runID := triggerEvalExpectJudge(t, ts.URL, suiteID, "alt-judge-model", modelID)
	run := waitEvalDone(t, ts.URL, runID)

	if run["judge_model"] != "alt-judge-model" {
		t.Errorf("run judge_model = %v, want alt-judge-model", run["judge_model"])
	}
	if !stub.sawModel("alt-judge-model") {
		t.Error("judge calls should have been made with the configured judge model")
	}

	// The stub role-plays any judge via the裁判 prompt marker, so judge
	// verdicts still parse and score through the new model.
	results := resultsByModel(run, "smart-model")
	var scored int
	for _, r := range results {
		if r["score"] != nil {
			scored++
		}
	}
	if scored == 0 {
		t.Error("expected judge-scored results via the configured judge model")
	}

	// The recorded judge also survives on the list endpoint.
	resp := doGet(t, ts.URL+"/api/evals")
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var runs []map[string]interface{}
	_ = json.Unmarshal(env.Data, &runs)
	if len(runs) != 1 || runs[0]["judge_model"] != "alt-judge-model" {
		t.Errorf("list endpoint judge_model mismatch: %v", runs)
	}
}

// triggerEvalExpectJudge is triggerEval plus a judge_model assertion on the
// campaign's single run.
func triggerEvalExpectJudge(t *testing.T, base string, suiteID int64, judge string, modelIDs ...int64) int64 {
	t.Helper()
	resp := doPost(t, base+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": modelIDs,
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/evals: expected 202, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var campaign map[string]interface{}
	_ = json.Unmarshal(env.Data, &campaign)
	runs, ok := campaign["runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("manual single-suite trigger: campaign runs = %v, want exactly 1", campaign["runs"])
	}
	run := runs[0].(map[string]interface{})
	if run["judge_model"] != judge {
		t.Fatalf("new run judge_model = %v, want %s", run["judge_model"], judge)
	}
	return int64(run["id"].(float64))
}
