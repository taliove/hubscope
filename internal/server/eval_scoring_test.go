package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestEvalExactMode runs the reasoning suite (exact rule mode) against a
// smart model; every answer must score 1 and the aggregate must be 1.
func TestEvalExactMode(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "reasoning")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, r := range results {
		if r["score"] != 1.0 {
			t.Errorf("case %v score = %v, want 1 (detail: %v)", r["case_id"], r["score"], r["verdict_detail"])
		}
		if !strings.Contains(r["verdict_detail"].(string), "exact") {
			t.Errorf("verdict_detail should mention exact mode: %v", r["verdict_detail"])
		}
	}
	if score, ok := run["score"].(float64); !ok || score != 1.0 {
		t.Errorf("run score = %v, want 1", run["score"])
	}
}

// TestEvalJudge covers the judge verdict path: two cases get a parsed score
// and reason, one case gets judge garbage back and must stay unscored
// (score null). The aggregate must average only the scored cases.
func TestEvalJudge(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "chinese")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	var scored, unscored int
	for _, r := range results {
		detail := r["verdict_detail"].(string)
		if r["score"] == nil {
			unscored++
			if !strings.Contains(detail, "judge parse failed") {
				t.Errorf("unscored case detail = %q, want judge parse failure", detail)
			}
			continue
		}
		scored++
		if r["score"] != 0.75 {
			t.Errorf("judge score = %v, want 0.75", r["score"])
		}
		if !strings.Contains(detail, "meets the rubric") {
			t.Errorf("verdict_detail should carry the judge reason: %q", detail)
		}
		if r["answer_text"] == nil || r["answer_text"] == "" {
			t.Error("answer_text should still be stored when the judge ran")
		}
	}
	if scored != 2 || unscored != 1 {
		t.Errorf("scored/unscored = %d/%d, want 2/1", scored, unscored)
	}

	// Aggregate averages only non-null scores: (0.75 + 0.75) / 2 = 0.75.
	if score, ok := run["score"].(float64); !ok || score != 0.75 {
		t.Errorf("run score = %v, want 0.75 (nulls excluded)", run["score"])
	}
}

// TestEvalModelFailure points a model at a hub that 503s: every case yields
// no answer and no score, the run still completes, and the aggregate is null.
func TestEvalModelFailure(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done even when the model fails", run["status"])
	}

	results := resultsByModel(run, "broken-model")
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, r := range results {
		if r["answer_text"] != nil {
			t.Errorf("failed case answer_text = %v, want null", r["answer_text"])
		}
		if r["score"] != nil {
			t.Errorf("failed case score = %v, want null", r["score"])
		}
		detail, _ := r["verdict_detail"].(string)
		if !strings.Contains(detail, "answer call failed") {
			t.Errorf("verdict_detail should record the call failure: %q", detail)
		}
	}
	if run["score"] != nil {
		t.Errorf("run score = %v, want null (nothing scored)", run["score"])
	}
}

// TestEvalValidation covers the POST /api/evals rejection branches.
func TestEvalValidation(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	post := func(body map[string]interface{}) *http.Response {
		return doPost(t, ts.URL+"/api/evals", body)
	}

	t.Run("empty_model_ids", func(t *testing.T) {
		resp := post(map[string]interface{}{"suite_id": suiteID, "model_ids": []int64{}})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("missing_model_ids", func(t *testing.T) {
		resp := post(map[string]interface{}{"suite_id": suiteID})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("non_chat_model_rejected", func(t *testing.T) {
		// There is no API to tag capability yet (discovery is ticket 05), so
		// stage the non_chat tag directly in the store the test owns.
		if err := db.SetModelCapability(modelID, "non_chat"); err != nil {
			t.Fatalf("stage non_chat capability: %v", err)
		}
		resp := post(map[string]interface{}{"suite_id": suiteID, "model_ids": []int64{modelID}})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("unknown_suite", func(t *testing.T) {
		resp := post(map[string]interface{}{"suite_id": 99999, "model_ids": []int64{modelID}})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("unknown_model", func(t *testing.T) {
		resp := post(map[string]interface{}{"suite_id": suiteID, "model_ids": []int64{99999}})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestEvalProtocolFallback disables the anthropic endpoint of a model; the
// evaluator must fall back to the openai endpoint and still score the run.
func TestEvalProtocolFallback(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")

	// Find the model's anthropic endpoint and disable it via the API.
	modelsResp := doGet(t, ts.URL+"/api/models")
	var env envelope
	_ = json.NewDecoder(modelsResp.Body).Decode(&env)
	modelsResp.Body.Close()
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)

	var anthropicEndpointID int64
	for _, m := range models {
		if int64(m["id"].(float64)) != modelID {
			continue
		}
		for _, e := range m["endpoints"].([]interface{}) {
			ep := e.(map[string]interface{})
			if ep["protocol"] == "anthropic" {
				anthropicEndpointID = int64(ep["id"].(float64))
			}
		}
	}
	if anthropicEndpointID == 0 {
		t.Fatal("anthropic endpoint not found for model")
	}

	patchResp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, anthropicEndpointID),
		map[string]interface{}{"enabled": false})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("disable anthropic endpoint: got %d", patchResp.StatusCode)
	}
	patchResp.Body.Close()

	suiteID := suiteIDByKey(t, ts.URL, "basic")
	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, r := range results {
		if r["score"] != 1.0 {
			t.Errorf("fallback case %v score = %v, want 1 (detail: %v)", r["case_id"], r["score"], r["verdict_detail"])
		}
	}
	if !stub.sawProtocol("smart-model", "openai") {
		t.Error("expected fallback to the openai endpoint")
	}
	if stub.sawProtocol("smart-model", "anthropic") {
		t.Error("anthropic endpoint is disabled and must not be called")
	}
}

// TestCaseCreatePatch covers POST /api/cases and PATCH /api/cases/{id}.
func TestCaseCreatePatch(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	// Create a new rule case.
	createResp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "只回复 OK",
		"verdict_type": "rule",
		"rule_config":  map[string]string{"mode": "exact", "expected": "OK"},
		"enabled":      true,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create case: expected 201, got %d", createResp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(createResp.Body).Decode(&env)
	createResp.Body.Close()
	var created map[string]interface{}
	_ = json.Unmarshal(env.Data, &created)
	caseID := int64(created["id"].(float64))
	if created["verdict_type"] != "rule" || created["enabled"] != true {
		t.Errorf("created case mismatch: %v", created)
	}
	rc := created["rule_config"].(map[string]interface{})
	if rc["mode"] != "exact" || rc["expected"] != "OK" {
		t.Errorf("created rule_config mismatch: %v", rc)
	}

	// Patch prompt and disable it.
	patchResp := doPatch(t, fmt.Sprintf("%s/api/cases/%d", ts.URL, caseID), map[string]interface{}{
		"prompt":  "只回复好的",
		"enabled": false,
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch case: expected 200, got %d", patchResp.StatusCode)
	}
	_ = json.NewDecoder(patchResp.Body).Decode(&env)
	patchResp.Body.Close()
	var patched map[string]interface{}
	_ = json.Unmarshal(env.Data, &patched)
	if patched["prompt"] != "只回复好的" || patched["enabled"] != false {
		t.Errorf("patched case mismatch: %v", patched)
	}
	// Untouched fields survive the patch.
	if patched["rule_config"].(map[string]interface{})["expected"] != "OK" {
		t.Errorf("rule_config should survive a partial patch: %v", patched["rule_config"])
	}

	// The new case is visible in the suite listing, disabled as patched.
	basicID := suiteIDByKey(t, ts.URL, "basic")
	if basicID != suiteID {
		t.Fatalf("suite id changed: %d -> %d", suiteID, basicID)
	}
	suitesResp := doGet(t, ts.URL+"/api/suites")
	_ = json.NewDecoder(suitesResp.Body).Decode(&env)
	suitesResp.Body.Close()
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	var found bool
	for _, s := range suites {
		if s["key"] != "basic" {
			continue
		}
		for _, c := range s["cases"].([]interface{}) {
			cm := c.(map[string]interface{})
			if int64(cm["id"].(float64)) == caseID {
				found = true
				if cm["enabled"] != false || cm["prompt"] != "只回复好的" {
					t.Errorf("listed case does not reflect the patch: %v", cm)
				}
			}
		}
	}
	if !found {
		t.Error("created case not visible in GET /api/suites")
	}

	t.Run("validation_errors", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]interface{}
			want int
		}{
			{"rule_without_config", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "rule"}, http.StatusBadRequest},
			{"bad_verdict_type", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "bogus"}, http.StatusBadRequest},
			{"bad_rule_mode", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "rule",
				"rule_config": map[string]string{"mode": "fuzzy", "expected": "x"}}, http.StatusBadRequest},
			{"invalid_regex", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "rule",
				"rule_config": map[string]string{"mode": "regex", "expected": "("}}, http.StatusBadRequest},
			{"judge_without_rubric", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "judge"}, http.StatusBadRequest},
			{"unknown_suite", map[string]interface{}{
				"suite_id": 99999, "prompt": "x", "verdict_type": "judge", "rubric": "r"}, http.StatusNotFound},
		}
		for _, tc := range cases {
			resp := doPost(t, ts.URL+"/api/cases", tc.body)
			if resp.StatusCode != tc.want {
				t.Errorf("%s: expected %d, got %d", tc.name, tc.want, resp.StatusCode)
			}
			resp.Body.Close()
		}

		resp := doPatch(t, ts.URL+"/api/cases/99999", map[string]interface{}{"prompt": "y"})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("patch unknown case: expected 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}
