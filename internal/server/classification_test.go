package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// listRulesViaAPI fetches GET /api/classification-rules.
func listRulesViaAPI(t *testing.T, baseURL string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, baseURL+"/api/classification-rules")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list rules: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode rules: %v", err)
	}
	var rules []map[string]interface{}
	if err := json.Unmarshal(env.Data, &rules); err != nil {
		t.Fatalf("unmarshal rules: %v", err)
	}
	return rules
}

// findRule returns the first rule matching dimension and keyword, or nil.
func findRule(rules []map[string]interface{}, dimension, keyword string) map[string]interface{} {
	for _, r := range rules {
		if r["dimension"].(string) == dimension && r["keyword"].(string) == keyword {
			return r
		}
	}
	return nil
}

// assertClassification checks a model's family and capability via the models API.
func assertClassification(t *testing.T, baseURL, modelID, wantFamily, wantCapability string) {
	t.Helper()
	models := listModelsViaAPI(t, baseURL)
	m, ok := models[modelID]
	if !ok {
		t.Fatalf("model %s not found", modelID)
	}
	if got := m["family"]; got != wantFamily {
		t.Errorf("%s family: expected %q, got %v", modelID, wantFamily, got)
	}
	if got := m["capability"]; got != wantCapability {
		t.Errorf("%s capability: expected %q, got %v", modelID, wantCapability, got)
	}
}

// TestClassificationDefaultsOnDiscovery verifies that the seeded default rules
// classify discovered models along both dimensions.
func TestClassificationDefaultsOnDiscovery(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{
		"gpt-5", "claude-sonnet-4", "qwen3-32b", "deepseek-v3",
		"text-embedding-3-large", "gpt-image-2", "whisper-1", "weird-thing",
		"bge-m3", "llama-3-8b-gptq", "o3-mini",
	})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	assertClassification(t, ts.URL, "gpt-5", "gpt", "chat")
	assertClassification(t, ts.URL, "claude-sonnet-4", "claude", "chat")
	assertClassification(t, ts.URL, "qwen3-32b", "qwen", "chat")
	assertClassification(t, ts.URL, "deepseek-v3", "deepseek", "chat")
	assertClassification(t, ts.URL, "text-embedding-3-large", "other", "embedding")
	assertClassification(t, ts.URL, "gpt-image-2", "gpt", "image")
	assertClassification(t, ts.URL, "whisper-1", "other", "audio")
	assertClassification(t, ts.URL, "weird-thing", "other", "chat")
	// bge models are embeddings even without the literal "embedding" substring.
	assertClassification(t, ts.URL, "bge-m3", "other", "embedding")
	// The GPTQ quantization suffix must not misfire the gpt family rule.
	assertClassification(t, ts.URL, "llama-3-8b-gptq", "llama", "chat")
	// OpenAI reasoning series without "gpt" in the ID.
	assertClassification(t, ts.URL, "o3-mini", "gpt", "chat")
}

// TestClassificationOnManualModel verifies manual model registration goes
// through the same rule-based classification.
func TestClassificationOnManualModel(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	hubID := createHubViaAPI(t, ts.URL, stubHub.URL)
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "GLM-4.5-Air",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", resp.StatusCode)
	}

	assertClassification(t, ts.URL, "GLM-4.5-Air", "glm", "chat")
}

// TestClassificationRulesCRUDAndReclassify verifies the rules API and that
// every mutation immediately reclassifies all stored models.
func TestClassificationRulesCRUDAndReclassify(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"weird-thing"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")
	assertClassification(t, ts.URL, "weird-thing", "other", "chat")

	// Default rules are seeded on first run.
	rules := listRulesViaAPI(t, ts.URL)
	if len(rules) == 0 {
		t.Fatal("expected seeded default classification rules")
	}
	if findRule(rules, "family", "qwen") == nil {
		t.Error("expected a seeded qwen family rule")
	}

	// Create: a new family rule reclassifies the stored model on save.
	resp := doPost(t, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "family",
		"keyword":   "weird",
		"category":  "acme",
		"priority":  10,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rule: expected 201, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode created rule: %v", err)
	}
	resp.Body.Close()
	var created map[string]interface{}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("unmarshal created rule: %v", err)
	}
	ruleID := int(created["id"].(float64))
	assertClassification(t, ts.URL, "weird-thing", "acme", "chat")

	// A capability rule on the same keyword applies independently.
	resp = doPost(t, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "capability",
		"keyword":   "weird",
		"category":  "code",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create capability rule: expected 201, got %d", resp.StatusCode)
	}
	assertClassification(t, ts.URL, "weird-thing", "acme", "code")

	// Update: moving the family keyword off the model reclassifies it back.
	resp = doPatch(t, ts.URL+fmt.Sprintf("/api/classification-rules/%d", ruleID), map[string]interface{}{
		"keyword": "strange",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch rule: expected 200, got %d", resp.StatusCode)
	}
	assertClassification(t, ts.URL, "weird-thing", "other", "code")

	// Delete: removing the capability rule restores the default.
	rules = listRulesViaAPI(t, ts.URL)
	capRule := findRule(rules, "capability", "weird")
	if capRule == nil {
		t.Fatal("capability rule weird not found")
	}
	delResp := doDelete(t, fmt.Sprintf("%s/api/classification-rules/%d", ts.URL, int(capRule["id"].(float64))))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete rule: expected 204, got %d", delResp.StatusCode)
	}
	assertClassification(t, ts.URL, "weird-thing", "other", "chat")
}

// TestClassificationRuleValidation verifies input validation on the rules API.
func TestClassificationRuleValidation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	cases := []map[string]interface{}{
		{"dimension": "bogus", "keyword": "x", "category": "y"},
		{"dimension": "family", "keyword": "", "category": "y"},
		{"dimension": "family", "keyword": "x", "category": ""},
	}
	for i, body := range cases {
		resp := doPost(t, ts.URL+"/api/classification-rules", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("case %d: expected 400, got %d", i, resp.StatusCode)
		}
	}

	// Patching a missing rule is a 404.
	resp := doPatch(t, ts.URL+"/api/classification-rules/99999", map[string]interface{}{"keyword": "x"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("patch missing rule: expected 404, got %d", resp.StatusCode)
	}
}
