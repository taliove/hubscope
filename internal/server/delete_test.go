package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestDeleteEndpointCascadesHistory verifies that deleting an endpoint removes
// its probe history and alert events, leaves the sibling endpoint untouched,
// and that a repeated delete returns 404.
func TestDeleteEndpointCascadesHistory(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// Point alerting at the stub webhook so failures record alert events.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	endpointID := createProbedEndpoint(t, ts, "Del Hub", stubHub.URL, "del-model")
	stubHub.SetMode("error_503")

	// Two rounds = four consecutive failures: probe history plus one down
	// alert event for this endpoint.
	runProbeRound(t, ts, endpointID)
	runProbeRound(t, ts, endpointID)
	if got := len(listAlerts(t, ts, "")); got != 1 {
		t.Fatalf("expected 1 alert event before delete, got %d", got)
	}

	resp := doDelete(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, endpointID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete endpoint: expected 204, got %d", resp.StatusCode)
	}

	// Probe history is gone together with the endpoint.
	probeResp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes", ts.URL, endpointID))
	probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusNotFound {
		t.Errorf("probes of deleted endpoint: expected 404, got %d", probeResp.StatusCode)
	}

	// Alert events of the endpoint are gone too.
	if got := len(listAlerts(t, ts, "")); got != 0 {
		t.Errorf("expected 0 alert events after delete, got %d", got)
	}

	// A repeated delete is a 404.
	resp = doDelete(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, endpointID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("re-delete endpoint: expected 404, got %d", resp.StatusCode)
	}

	// The model keeps its remaining (openai) endpoint.
	models := listModelsViaAPI(t, ts.URL)
	m, ok := models["del-model"]
	if !ok {
		t.Fatal("model should survive its endpoint's deletion")
	}
	endpoints := m["endpoints"].([]interface{})
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 remaining endpoint, got %d", len(endpoints))
	}
	if ep := endpoints[0].(map[string]interface{}); ep["protocol"].(string) != "openai" {
		t.Errorf("remaining endpoint should be the openai one, got %v", ep["protocol"])
	}
}

// TestDeleteManualModelCascades verifies that deleting a manual model removes
// its endpoints with their probe history and alert events, and that a
// repeated delete is a 404.
func TestDeleteManualModelCascades(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	lark := newStubLarkServer(t)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// Alerting on, hub failing: one endpoint accumulates four failed probes
	// and a down alert event, exercising the alert-events cascade too.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    true,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}
	stubHub.SetMode("success")

	hubID := createHubViaAPI(t, ts.URL, stubHub.URL)
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "manual-model",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", resp.StatusCode)
	}
	stubHub.SetMode("error_503")

	models := listModelsViaAPI(t, ts.URL)
	modelID := int64(models["manual-model"]["id"].(float64))
	for _, raw := range models["manual-model"]["endpoints"].([]interface{}) {
		ep := raw.(map[string]interface{})
		epID := int64(ep["id"].(float64))
		runProbeRound(t, ts, epID)
		if ep["protocol"].(string) == "anthropic" {
			runProbeRound(t, ts, epID) // second round crosses the alert threshold
		}
	}
	if got := len(listAlerts(t, ts, "")); got != 1 {
		t.Fatalf("expected 1 alert event before delete, got %d", got)
	}

	delResp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete model: expected 204, got %d", delResp.StatusCode)
	}

	if _, ok := listModelsViaAPI(t, ts.URL)["manual-model"]; ok {
		t.Error("deleted model should not be listed")
	}

	// Alert events of the model's endpoints are gone.
	if got := len(listAlerts(t, ts, "")); got != 0 {
		t.Errorf("expected 0 alert events after model delete, got %d", got)
	}

	// Every endpoint's detail/history is gone (probes cascaded).
	for _, raw := range models["manual-model"]["endpoints"].([]interface{}) {
		ep := raw.(map[string]interface{})
		epID := int64(ep["id"].(float64))
		probeResp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes", ts.URL, epID))
		probeResp.Body.Close()
		if probeResp.StatusCode != http.StatusNotFound {
			t.Errorf("endpoint %d probes after model delete: expected 404, got %d", epID, probeResp.StatusCode)
		}
	}

	delResp = doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("re-delete model: expected 404, got %d", delResp.StatusCode)
	}
}

// TestDeleteDiscoveredModelRejected verifies that a discovered model cannot be
// deleted (the next sync would resurrect it); disabling is the offered path.
func TestDeleteDiscoveredModelRejected(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"auto-model"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)
	auto, ok := models["auto-model"]
	if !ok {
		t.Fatal("auto-model should be discovered")
	}
	if auto["origin"].(string) != "discovered" {
		t.Fatalf("expected discovered origin, got %v", auto["origin"])
	}
	modelID := int64(auto["id"].(float64))

	delResp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID))
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusConflict {
		t.Fatalf("delete discovered model: expected 409, got %d", delResp.StatusCode)
	}

	// The model and both endpoints are untouched.
	models = listModelsViaAPI(t, ts.URL)
	auto, ok = models["auto-model"]
	if !ok {
		t.Fatal("discovered model must survive the rejected delete")
	}
	if got := len(auto["endpoints"].([]interface{})); got != 2 {
		t.Errorf("endpoints should be untouched, got %d", got)
	}
}
