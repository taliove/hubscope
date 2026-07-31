package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// lastGenerationRaw returns the raw JSON body of the model's most recent
// /v1/images/generations call seen by the stub.
func lastGenerationRaw(t *testing.T, stub *discoveryStubHub, modelID string) map[string]interface{} {
	t.Helper()
	reqs := stub.imageRequests(modelID)
	if len(reqs) == 0 {
		t.Fatalf("stub saw no images_generation call for %s", modelID)
	}
	return reqs[len(reqs)-1].Raw
}

// lastEditFields returns the flattened multipart fields of the model's most
// recent /v1/images/edits call seen by the stub.
func lastEditFields(t *testing.T, stub *discoveryStubHub, modelID string) map[string]string {
	t.Helper()
	reqs := stub.editRequests(modelID)
	if len(reqs) == 0 {
		t.Fatalf("stub saw no images_edit call for %s", modelID)
	}
	return reqs[len(reqs)-1].Fields
}

// probeImageEndpoint fires one manual probe round for the endpoint and
// expects HTTP 200.
func probeImageEndpoint(t *testing.T, baseURL string, endpointID int) {
	t.Helper()
	resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", baseURL, endpointID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe endpoint %d: expected 200, got %d", endpointID, resp.StatusCode)
	}
}

// TestImageParamSeededRuleAppliesOnAllProbePaths is the core acceptance test
// of GH #33 (spec 0014 US 16-17): the seeded gpt-image rule appends
// quality:"low" to gpt-image probe requests while unmatched models keep the
// minimal body {model, prompt, n:1} — on all three call sites (discovery
// trial, manual model creation trial, and the scheduled/manual probe round)
// and on both image protocols.
func TestImageParamSeededRuleAppliesOnAllProbePaths(t *testing.T) {
	// spec 0018 T2 (GH #100): image/video models are trial-free, no discovery
	// trial requests. This test verified trial request parameters, which no
	// longer exist. Skip it (image params rules are now unused, kept for
	// potential future use but have no consumer).
	t.Skip("spec 0018 T2 (GH #100): image models are trial-free, no discovery trial requests")
}

// ---------------------------------------------------------------------------
// Management API (GH #33): /api/image-param-rules mirrors the classification
// rule surface — super_admin writes with audit, any authenticated user reads.
// ---------------------------------------------------------------------------

// listImageParamRules fetches GET /api/image-param-rules.
func listImageParamRules(t *testing.T, baseURL string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, baseURL+"/api/image-param-rules")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list image param rules: expected 200, got %d", resp.StatusCode)
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

// findImageParamRule returns the first rule with the keyword, or nil.
func findImageParamRule(rules []map[string]interface{}, keyword string) map[string]interface{} {
	for _, r := range rules {
		if r["keyword"].(string) == keyword {
			return r
		}
	}
	return nil
}

// TestImageParamRulesSeedOnBoot covers US 16's built-in rule: a fresh
// database exposes exactly the seeded gpt-image -> quality:low rule.
func TestImageParamRulesSeedOnBoot(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	rules := listImageParamRules(t, ts.URL)
	if len(rules) != 1 {
		t.Fatalf("expected exactly 1 seeded rule, got %d: %v", len(rules), rules)
	}
	rule := rules[0]
	if rule["keyword"] != "gpt-image" {
		t.Errorf("seeded keyword: expected gpt-image, got %v", rule["keyword"])
	}
	params, ok := rule["params"].(map[string]interface{})
	if !ok || params["quality"] != "low" || len(params) != 1 {
		t.Errorf("seeded params: expected {quality:low}, got %v", rule["params"])
	}
	if rule["priority"].(float64) != 100 {
		t.Errorf("seeded priority: expected 100, got %v", rule["priority"])
	}
}

// TestImageParamRulesCRUDAndImmediateEffect covers US 18: rules are managed
// through the API (create/list/patch/delete, duplicate 409, missing 404) and
// a mutation takes effect on the very next probe — no restart, no cache.
func TestImageParamRulesCRUDAndImmediateEffect(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// Create: a flux rule adding a size param.
	resp := doPost(t, ts.URL+"/api/image-param-rules", map[string]interface{}{
		"keyword":  "FLUX",
		"params":   map[string]string{"size": "1024x1024"},
		"priority": 50,
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
	// Keywords are normalized to lowercase (matching is case-insensitive, and
	// case variants of one word must not become two rules).
	if created["keyword"] != "flux" {
		t.Errorf("created keyword: expected normalized flux, got %v", created["keyword"])
	}
	ruleID := int64(created["id"].(float64))

	// Duplicate keyword (any case) conflicts and is audited as failed.
	resp = doPost(t, ts.URL+"/api/image-param-rules", map[string]interface{}{
		"keyword": "Flux",
		"params":  map[string]string{"size": "512x512"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate keyword: expected 409, got %d", resp.StatusCode)
	}

	// Immediate effect: the very next trial carries the new param.
	stub.setImageMode("flux-2-dev", "success")
	resp = doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "flux-2-dev",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create flux-2-dev: expected 201, got %d", resp.StatusCode)
	}
	raw := lastGenerationRaw(t, stub, "flux-2-dev")
	if raw["size"] != "1024x1024" {
		t.Errorf("trial after rule create: expected size=1024x1024, body %v", raw)
	}

	// Patch: change the param value; next probe reflects it.
	resp = doPatch(t, fmt.Sprintf("%s/api/image-param-rules/%d", ts.URL, ruleID), map[string]interface{}{
		"params": map[string]string{"size": "512x512"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("patch rule: expected 200, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	genID := int(endpointByProtocol(t, models["flux-2-dev"], "images_generation")["id"].(float64))
	probeImageEndpoint(t, ts.URL, genID)
	raw = lastGenerationRaw(t, stub, "flux-2-dev")
	if raw["size"] != "512x512" {
		t.Errorf("probe after rule patch: expected size=512x512, body %v", raw)
	}

	// Missing rows are a clean 404 on both write verbs.
	resp = doPatch(t, ts.URL+"/api/image-param-rules/99999", map[string]interface{}{
		"priority": 10,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("patch missing rule: expected 404, got %d", resp.StatusCode)
	}
	resp = doDelete(t, ts.URL+"/api/image-param-rules/99999")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing rule: expected 404, got %d", resp.StatusCode)
	}

	// Delete: the match disappears and the body degrades to the minimal shape
	// without an error (spec 0014 US 17).
	resp = doDelete(t, fmt.Sprintf("%s/api/image-param-rules/%d", ts.URL, ruleID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete rule: expected 204, got %d", resp.StatusCode)
	}
	if findImageParamRule(listImageParamRules(t, ts.URL), "flux") != nil {
		t.Error("deleted rule should be absent from the list")
	}
	probeImageEndpoint(t, ts.URL, genID)
	raw = lastGenerationRaw(t, stub, "flux-2-dev")
	if _, hasSize := raw["size"]; hasSize || len(raw) != 3 {
		t.Errorf("probe after rule delete: expected minimal body, got %v", raw)
	}

	// The failed duplicate create was audited alongside the successes.
	items := auditItems(t, fetchAuditLogs(t, ts.URL, "?action=image_param_rule.create"))
	var sawSuccess, sawFailed bool
	for _, item := range items {
		switch item["result"] {
		case "success":
			sawSuccess = true
		default:
			sawFailed = true
		}
	}
	if !sawSuccess || !sawFailed {
		t.Errorf("audit image_param_rule.create: expected a success and a failed entry, got %v", items)
	}
}

// TestImageParamRulesValidation pins the write-time guards (GH #33 decisions):
// params values are strings only and the reserved keys model/prompt/n are
// rejected with 400 on create and patch.
func TestImageParamRulesValidation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing keyword", map[string]interface{}{"params": map[string]string{"a": "b"}}},
		{"empty params", map[string]interface{}{"keyword": "x", "params": map[string]string{}}},
		{"missing params", map[string]interface{}{"keyword": "x"}},
		{"non-string value", map[string]interface{}{"keyword": "x", "params": map[string]interface{}{"quality": 1}}},
		{"reserved model", map[string]interface{}{"keyword": "x", "params": map[string]string{"model": "evil"}}},
		{"reserved prompt", map[string]interface{}{"keyword": "x", "params": map[string]string{"prompt": "evil"}}},
		{"reserved n", map[string]interface{}{"keyword": "x", "params": map[string]string{"n": "5"}}},
		{"reserved case variant", map[string]interface{}{"keyword": "x", "params": map[string]string{"Model": "evil"}}},
		{"priority zero", map[string]interface{}{"keyword": "x", "params": map[string]string{"a": "b"}, "priority": 0}},
		{"priority too large", map[string]interface{}{"keyword": "x", "params": map[string]string{"a": "b"}, "priority": 10001}},
	}
	for _, tc := range cases {
		resp := doPost(t, ts.URL+"/api/image-param-rules", tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("create %s: expected 400, got %d", tc.name, resp.StatusCode)
		}
	}
	if got := len(listImageParamRules(t, ts.URL)); got != 1 {
		t.Errorf("rejected creates must leave only the seeded rule, got %d rules", got)
	}

	// Patch rejects reserved keys too (the seeded rule is a valid target).
	rules := listImageParamRules(t, ts.URL)
	seededID := int64(rules[0]["id"].(float64))
	resp := doPatch(t, fmt.Sprintf("%s/api/image-param-rules/%d", ts.URL, seededID), map[string]interface{}{
		"params": map[string]string{"n": "2"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("patch reserved key: expected 400, got %d", resp.StatusCode)
	}
}

// TestImageParamRulesPermissions covers the role split (spec 0005): reads are
// open to every authenticated role while writes are super_admin-only, with
// anonymous callers answered 401.
func TestImageParamRulesPermissions(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newStubHubServer()
	defer stub.Close()
	hub, err := db.CreateHub("perm-hub", stub.URL, "test-token-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	seedUserWithRole(t, db, "ip-view", store.RoleViewer, &hub.ID)
	seedUserWithRole(t, db, "ip-oper", store.RoleOperator, &hub.ID)
	seedUserWithRole(t, db, "ip-admin", store.RoleAdmin, &hub.ID)

	ruleBody := map[string]interface{}{
		"keyword": "perm-model",
		"params":  map[string]string{"quality": "low"},
	}

	// Anonymous: 401 on read and write.
	resp, err := http.Post(ts.URL+"/api/image-param-rules", "application/json", nil)
	if err != nil {
		t.Fatalf("anonymous post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous create: expected 401, got %d", resp.StatusCode)
	}
	resp, err = http.Get(ts.URL + "/api/image-param-rules")
	if err != nil {
		t.Fatalf("anonymous get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous list: expected 401, got %d", resp.StatusCode)
	}

	// viewer/operator/admin cannot write but can read.
	for _, username := range []string{"ip-view", "ip-oper", "ip-admin"} {
		client := loginAsClient(t, ts.URL, username)
		resp := postAs(t, client, ts.URL+"/api/image-param-rules", ruleBody)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s create: expected 403, got %d", username, resp.StatusCode)
		}
		resp, err := client.Get(ts.URL + "/api/image-param-rules")
		if err != nil {
			t.Fatalf("%s list: %v", username, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s list: expected 200, got %d", username, resp.StatusCode)
		}
	}

	// super_admin (the seeded test admin) writes successfully and is audited.
	resp = doPost(t, ts.URL+"/api/image-param-rules", ruleBody)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("super_admin create: expected 201, got %d", resp.StatusCode)
	}
	items := auditItems(t, fetchAuditLogs(t, ts.URL, "?action=image_param_rule.create"))
	if len(items) != 1 || items[0]["result"] != "success" {
		t.Errorf("expected exactly 1 successful image_param_rule.create audit entry, got %v", items)
	}
}

// TestImageParamMergeCollisionAndReservedDefense pins the merge semantics
// (GH #33 decision 2+4): every matching rule contributes, the smaller
// priority number wins a key collision, and reserved keys are skipped even
// when a rule bypassing API validation carries them (double insurance).
func TestImageParamMergeCollisionAndReservedDefense(t *testing.T) {
	t.Skip("spec 0018 T1 (GH #97): image endpoints no longer probed, test obsolete")
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	stub.setImageMode("gpt-image-2", "success")
	img := createImageEndpointViaDiscovery(t, ts.URL, stub, "gpt-image-2")
	genID := int(endpointByProtocol(t, img, "images_generation")["id"].(float64))

	// A narrower rule with a smaller priority number overrides the seeded
	// rule's quality and adds its own key.
	resp := doPost(t, ts.URL+"/api/image-param-rules", map[string]interface{}{
		"keyword":  "gpt-image-2",
		"params":   map[string]string{"quality": "medium", "size": "512x512"},
		"priority": 50,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create narrow rule: expected 201, got %d", resp.StatusCode)
	}

	// A rule inserted straight into the store (bypassing API validation)
	// carries reserved keys: Merge must skip them defensively while still
	// applying its legitimate key.
	if _, err := db.CreateImageParamRule("gpt", map[string]string{
		"model": "evil-model", "prompt": "evil", "n": "9", "extra": "yes",
	}, 1); err != nil {
		t.Fatalf("insert reserved-key rule: %v", err)
	}

	probeImageEndpoint(t, ts.URL, genID)
	raw := lastGenerationRaw(t, stub, "gpt-image-2")
	if raw["quality"] != "medium" {
		t.Errorf("collision: expected quality=medium (priority 50 beats seeded 100), body %v", raw)
	}
	if raw["size"] != "512x512" {
		t.Errorf("narrow rule key: expected size=512x512, body %v", raw)
	}
	if raw["extra"] != "yes" {
		t.Errorf("store-inserted rule: expected extra=yes, body %v", raw)
	}
	if raw["model"] != "gpt-image-2" {
		t.Errorf("reserved model must stay the probed model, body %v", raw)
	}
	if raw["n"] != float64(1) {
		t.Errorf("reserved n must stay 1, body %v", raw)
	}
	if raw["prompt"] == "evil" || raw["prompt"] == "" {
		t.Errorf("reserved prompt must stay the fixed probe prompt, body %v", raw)
	}
}

// TestImageParamRulesSeedIdempotent pins the one-shot seed flag (GH #33
// decision 1): an emptied rule table is a deliberate admin choice and a
// restart must not reseed it.
func TestImageParamRulesSeedIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	seedTestUser(t, db)
	ts := newTestAPIServer(t, db)

	rules := listImageParamRules(t, ts.URL)
	if len(rules) != 1 {
		t.Fatalf("expected the seeded rule, got %d", len(rules))
	}
	seededID := int64(rules[0]["id"].(float64))
	resp := doDelete(t, fmt.Sprintf("%s/api/image-param-rules/%d", ts.URL, seededID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete seeded rule: expected 204, got %d", resp.StatusCode)
	}

	// "Restart": close and reopen the same database file.
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	db2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	t.Cleanup(func() { db2.Close() })
	seedTestUser(t, db2)
	ts2 := newTestAPIServer(t, db2)

	if got := listImageParamRules(t, ts2.URL); len(got) != 0 {
		t.Errorf("after restart: expected the emptied rule table to stay empty, got %v", got)
	}
}
