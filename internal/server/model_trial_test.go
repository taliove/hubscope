package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// trialModelViaAPI calls POST /api/models/{id}/trial and returns the status
// code plus the decoded result object (nil when the status is not 200).
func trialModelViaAPI(t *testing.T, baseURL string, modelID int64) (int, map[string]interface{}) {
	t.Helper()
	resp := doPost(t, fmt.Sprintf("%s/api/models/%d/trial", baseURL, modelID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode trial result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("unmarshal trial result: %v", err)
	}
	return resp.StatusCode, result
}

// createdProtocols reads the created_protocols string list of a trial result.
func createdProtocols(t *testing.T, result map[string]interface{}) []string {
	t.Helper()
	raw, ok := result["created_protocols"].([]interface{})
	if !ok {
		t.Fatalf("created_protocols must be an array, got %#v", result["created_protocols"])
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

// createEndpointlessManualModel registers a manual model on the given hub and
// deletes every endpoint, leaving the model endpointless. Returns its DB id.
func createEndpointlessManualModel(t *testing.T, baseURL string, hubID int, modelID string) int64 {
	t.Helper()
	resp := doPost(t, baseURL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": modelID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", resp.StatusCode)
	}

	models := listModelsViaAPI(t, baseURL)
	m, ok := models[modelID]
	if !ok {
		t.Fatalf("model %q should be registered", modelID)
	}
	for _, raw := range m["endpoints"].([]interface{}) {
		epID := int64(raw.(map[string]interface{})["id"].(float64))
		del := doDelete(t, fmt.Sprintf("%s/api/endpoints/%d", baseURL, epID))
		del.Body.Close()
		if del.StatusCode != http.StatusNoContent {
			t.Fatalf("delete endpoint: expected 204, got %d", del.StatusCode)
		}
	}
	return int64(m["id"].(float64))
}

// TestEndpointlessModelStaysListed pins the list-API semantics: a model whose
// endpoints were all deleted remains visible with an empty (non-null)
// endpoints array, and a manual one stays deletable.
func TestEndpointlessModelStaysListed(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	modelDBID := createEndpointlessManualModel(t, ts.URL, hubID, "endpointless-manual")

	// The model is still listed, with an empty (non-null) endpoints array.
	models := listModelsViaAPI(t, ts.URL)
	m, ok := models["endpointless-manual"]
	if !ok {
		t.Fatal("endpointless model must stay listed")
	}
	endpoints, ok := m["endpoints"].([]interface{})
	if !ok || endpoints == nil {
		t.Fatalf("endpoints must be an empty array, got %#v", m["endpoints"])
	}
	if len(endpoints) != 0 {
		t.Fatalf("expected 0 endpoints, got %d", len(endpoints))
	}

	// A manual endpointless model is deletable.
	del := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelDBID))
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete endpointless manual model: expected 204, got %d", del.StatusCode)
	}
	if _, ok := listModelsViaAPI(t, ts.URL)["endpointless-manual"]; ok {
		t.Error("deleted model must disappear from the list")
	}
}

// TestTrialModelBackfillsMissingProtocols verifies the manual re-trial: only
// missing protocols are probed, answering protocols get an enabled endpoint,
// and a fully-covered model is a no-op.
func TestTrialModelBackfillsMissingProtocols(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	modelDBID := createEndpointlessManualModel(t, ts.URL, hubID, "retry-model")

	// Anthropic keeps failing; only openai answers the re-trial.
	stub.setFailing("retry-model", "anthropic")
	status, result := trialModelViaAPI(t, ts.URL, modelDBID)
	if status != http.StatusOK {
		t.Fatalf("trial: expected 200, got %d", status)
	}
	created := createdProtocols(t, result)
	if len(created) != 1 || created[0] != "openai" {
		t.Fatalf("created_protocols: expected [openai], got %v", created)
	}
	if result["failures"].(string) == "" {
		t.Error("failures should explain the anthropic trial failure")
	}

	models := listModelsViaAPI(t, ts.URL)
	m := models["retry-model"]
	if !hasEndpoint(m, "openai") || hasEndpoint(m, "anthropic") {
		t.Fatalf("expected only the openai endpoint after trial, got %v", m["endpoints"])
	}
	if endpointEnabled(t, m, "openai") != true {
		t.Error("backfilled openai endpoint should be enabled")
	}

	// The protocol heals: the next trial backfills anthropic only.
	stub.clearFailing("retry-model", "anthropic")
	status, result = trialModelViaAPI(t, ts.URL, modelDBID)
	if status != http.StatusOK {
		t.Fatalf("second trial: expected 200, got %d", status)
	}
	created = createdProtocols(t, result)
	if len(created) != 1 || created[0] != "anthropic" {
		t.Fatalf("created_protocols: expected [anthropic], got %v", created)
	}
	if result["failures"].(string) != "" {
		t.Errorf("failures should be empty when nothing failed, got %q", result["failures"])
	}

	// A third trial is a no-op: no protocol is missing an endpoint.
	status, result = trialModelViaAPI(t, ts.URL, modelDBID)
	if status != http.StatusOK {
		t.Fatalf("third trial: expected 200, got %d", status)
	}
	if created := createdProtocols(t, result); len(created) != 0 {
		t.Errorf("no-op trial: expected no created protocols, got %v", created)
	}

	// The model ends with exactly one endpoint per protocol (no duplicates).
	models = listModelsViaAPI(t, ts.URL)
	m = models["retry-model"]
	if got := len(m["endpoints"].([]interface{})); got != 2 {
		t.Fatalf("expected exactly 2 endpoints, got %d", got)
	}
	if !hasEndpoint(m, "openai") || !hasEndpoint(m, "anthropic") {
		t.Errorf("expected both endpoints, got %v", m["endpoints"])
	}
}

// TestTrialModelUnreachableCreatesNothing pins W3: a re-trial that answers on
// no protocol creates no endpoint and keeps the model listed; an unknown
// model id is a 404.
func TestTrialModelUnreachableCreatesNothing(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	modelDBID := createEndpointlessManualModel(t, ts.URL, hubID, "dead-model")

	stub.setFailing("dead-model", "anthropic")
	stub.setFailing("dead-model", "openai")
	status, result := trialModelViaAPI(t, ts.URL, modelDBID)
	if status != http.StatusOK {
		t.Fatalf("trial: expected 200, got %d", status)
	}
	if created := createdProtocols(t, result); len(created) != 0 {
		t.Errorf("unreachable trial must create nothing, got %v", created)
	}
	if result["failures"].(string) == "" {
		t.Error("failures should explain why no endpoint was created")
	}

	// Nothing was created: the model stays listed with zero endpoints.
	models := listModelsViaAPI(t, ts.URL)
	m, ok := models["dead-model"]
	if !ok {
		t.Fatal("model must survive a failed re-trial")
	}
	if got := len(m["endpoints"].([]interface{})); got != 0 {
		t.Errorf("expected 0 endpoints after failed trial, got %d", got)
	}

	// Unknown model id: 404.
	resp := doPost(t, ts.URL+"/api/models/99999/trial", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("trial unknown model: expected 404, got %d", resp.StatusCode)
	}
}

// TestTrialModelDiscoveredEndpointless covers a discovered model that failed
// both protocol trials at sync time: the manual re-trial backfills endpoints
// once the hub serves the model, and the model stays undeletable (W3).
func TestTrialModelDiscoveredEndpointless(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, []string{"disc-model"})
	stub.setFailing("disc-model", "anthropic")
	stub.setFailing("disc-model", "openai")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// Both trials failed at sync: the model is registered with zero endpoints.
	models := listModelsViaAPI(t, ts.URL)
	m, ok := models["disc-model"]
	if !ok {
		t.Fatal("discovered model should be registered even with zero endpoints")
	}
	if m["origin"].(string) != "discovered" {
		t.Fatalf("expected discovered origin, got %v", m["origin"])
	}
	if got := len(m["endpoints"].([]interface{})); got != 0 {
		t.Fatalf("expected 0 endpoints after failed sync trials, got %d", got)
	}
	modelDBID := int64(m["id"].(float64))

	// Discovered models stay undeletable, even when endpointless.
	del := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelDBID))
	del.Body.Close()
	if del.StatusCode != http.StatusConflict {
		t.Fatalf("delete endpointless discovered model: expected 409, got %d", del.StatusCode)
	}

	// The hub heals; the manual re-trial backfills both protocols.
	stub.clearFailing("disc-model", "anthropic")
	stub.clearFailing("disc-model", "openai")
	status, result := trialModelViaAPI(t, ts.URL, modelDBID)
	if status != http.StatusOK {
		t.Fatalf("trial: expected 200, got %d", status)
	}
	if created := createdProtocols(t, result); len(created) != 2 {
		t.Fatalf("expected both protocols backfilled, got %v", created)
	}

	models = listModelsViaAPI(t, ts.URL)
	m = models["disc-model"]
	if !hasEndpoint(m, "openai") || !hasEndpoint(m, "anthropic") {
		t.Errorf("expected both endpoints after trial, got %v", m["endpoints"])
	}
}
