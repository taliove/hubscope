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
	suiteID := suiteIDByKey(t, ts.URL, "cap_reasoning")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
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

// TestEvalJudge covers the judge verdict path on the language suite (6 rule
// cases + 4 judge cases): judge cases get a parsed score and reason, except
// one whose judge replies are scripted to garbage and which must stay
// unscored (score null). The aggregate must average only the scored cases.
func TestEvalJudge(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "cap_language")

	// The product-intro judge case gets unparseable verdicts on every sample.
	stub.setJudgeSeq("保温杯", "I cannot produce a score for this.")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
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
		if r["answer_text"] == nil || r["answer_text"] == "" {
			t.Error("answer_text should still be stored when the judge ran")
		}
		switch {
		case strings.Contains(detail, "meets the rubric"):
			if r["score"] != 0.75 {
				t.Errorf("judge score = %v, want 0.75", r["score"])
			}
		default:
			// Rule cases score 1 for the smart model.
			if r["score"] != 1.0 {
				t.Errorf("rule score = %v, want 1 (detail: %q)", r["score"], detail)
			}
		}
	}
	if scored != 9 || unscored != 1 {
		t.Errorf("scored/unscored = %d/%d, want 9/1", scored, unscored)
	}

	// Aggregate averages only non-null scores: (6 x 1 + 3 x 0.75) / 9.
	want := (6.0 + 3.0*0.75) / 9.0
	if score, ok := run["score"].(float64); !ok || score < want-1e-9 || score > want+1e-9 {
		t.Errorf("run score = %v, want %v (nulls excluded)", run["score"], want)
	}
}

// TestEvalModelFailure points a model at a hub that 503s: every case yields
// no answer and no score, the run still completes, and the aggregate is null.
func TestEvalModelFailure(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	// Register while the hub is healthy (creation trial-probes), then break
	// the model: every eval call 503s from here on.
	modelID := createEvalModel(t, ts.URL, stub.URL, "flaky-model")
	stub.markBroken("flaky-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done even when the model fails", run["status"])
	}

	results := resultsByModel(run, "flaky-model")
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
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
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")

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
	stub.resetCalls()

	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")
	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "smart-model")
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
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

// TestCaseCreatePatch covers POST /api/cases and the immutable PATCH
// semantics of /api/cases/{id}: a content edit retires the old case and
// returns a new one, while an enabled-only patch toggles the row in place.
func patchSampleCount(t *testing.T, base string, id int64, value interface{}) map[string]interface{} {
	t.Helper()
	resp := doPatch(t, fmt.Sprintf("%s/api/cases/%d", base, id), map[string]interface{}{
		"sample_count": value,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch sample_count: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var out map[string]interface{}
	_ = json.Unmarshal(env.Data, &out)
	return out
}

func TestCaseCreatePatch(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")

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
	if created["difficulty"] != "basic" {
		t.Errorf("created difficulty = %v, want basic default", created["difficulty"])
	}
	if created["sample_count"] != nil {
		t.Errorf("created sample_count = %v, want null (inherits default)", created["sample_count"])
	}

	// A content patch creates a NEW case and disables the old one.
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
	newCaseID := int64(patched["id"].(float64))
	if newCaseID == caseID {
		t.Errorf("content patch should mint a new case id, still got %d", newCaseID)
	}
	if patched["prompt"] != "只回复好的" || patched["enabled"] != false {
		t.Errorf("new case mismatch: %v", patched)
	}
	// Untouched fields survive the merge onto the new case.
	if patched["rule_config"].(map[string]interface{})["expected"] != "OK" {
		t.Errorf("rule_config should survive a partial patch: %v", patched["rule_config"])
	}

	// The suite listing shows both rows: the old one disabled with its
	// original prompt, the new one carrying the patch.
	suitesResp := doGet(t, ts.URL+"/api/suites")
	_ = json.NewDecoder(suitesResp.Body).Decode(&env)
	suitesResp.Body.Close()
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	var oldRow, newRow map[string]interface{}
	for _, s := range suites {
		if s["key"] != "cap_instruction" {
			continue
		}
		for _, c := range s["cases"].([]interface{}) {
			cm := c.(map[string]interface{})
			switch int64(cm["id"].(float64)) {
			case caseID:
				oldRow = cm
			case newCaseID:
				newRow = cm
			}
		}
	}
	if oldRow == nil || newRow == nil {
		t.Fatalf("expected both old case %d and new case %d in the listing", caseID, newCaseID)
	}
	if oldRow["enabled"] != false || oldRow["prompt"] != "只回复 OK" {
		t.Errorf("old case should stay disabled with its original prompt: %v", oldRow)
	}
	if newRow["prompt"] != "只回复好的" {
		t.Errorf("new case should carry the patched prompt: %v", newRow)
	}

	// An enabled-only patch toggles the new row in place (same id).
	toggleResp := doPatch(t, fmt.Sprintf("%s/api/cases/%d", ts.URL, newCaseID), map[string]interface{}{
		"enabled": true,
	})
	if toggleResp.StatusCode != http.StatusOK {
		t.Fatalf("toggle case: expected 200, got %d", toggleResp.StatusCode)
	}
	_ = json.NewDecoder(toggleResp.Body).Decode(&env)
	toggleResp.Body.Close()
	var toggled map[string]interface{}
	_ = json.Unmarshal(env.Data, &toggled)
	if int64(toggled["id"].(float64)) != newCaseID || toggled["enabled"] != true {
		t.Errorf("enabled-only patch should toggle in place: %v", toggled)
	}

	// sample_count can be overridden and cleared back to the inherited
	// default via an explicit JSON null.
	sc := patchSampleCount(t, ts.URL, newCaseID, 3)
	if sc["sample_count"] != 3.0 {
		t.Errorf("sample_count override = %v, want 3", sc["sample_count"])
	}
	cleared := patchSampleCount(t, ts.URL, int64(sc["id"].(float64)), nil)
	if cleared["sample_count"] != nil {
		t.Errorf("sample_count after explicit null = %v, want null (inherit default)", cleared["sample_count"])
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
			{"bad_difficulty", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "rule",
				"rule_config": map[string]string{"mode": "exact", "expected": "x"},
				"difficulty":  "nightmare"}, http.StatusBadRequest},
			{"bad_sample_count", map[string]interface{}{
				"suite_id": suiteID, "prompt": "x", "verdict_type": "rule",
				"rule_config":  map[string]string{"mode": "exact", "expected": "x"},
				"sample_count": 0}, http.StatusBadRequest},
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
