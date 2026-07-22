package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/store"
)

// TestDiscoveryBackfillsMissingProtocol verifies that when a model gains
// support for a protocol it previously failed, the next sync creates the
// missing endpoint.
func TestDiscoveryBackfillsMissingProtocol(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"model-a"})
	// First sync: anthropic trial fails, only the openai endpoint is created.
	stub.setFailing("model-a", "anthropic")
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	models := listModelsViaAPI(t, ts.URL)
	if !hasEndpoint(models["model-a"], "openai") || hasEndpoint(models["model-a"], "anthropic") {
		t.Fatalf("expected only the openai endpoint, got %v", models["model-a"]["endpoints"])
	}

	// The hub starts speaking anthropic for the model; the next sync
	// backfills the missing endpoint.
	stub.clearFailing("model-a", "anthropic")
	stats := runDiscovery(t, ts.URL)
	if got := statNumber(t, stats, "endpoints_created"); got != 1 {
		t.Errorf("endpoints_created: expected 1 (backfill), got %d", got)
	}

	models = listModelsViaAPI(t, ts.URL)
	if !hasEndpoint(models["model-a"], "anthropic") {
		t.Fatal("anthropic endpoint should be backfilled after the protocol healed")
	}
	if endpointEnabled(t, models["model-a"], "anthropic") != true {
		t.Error("backfilled anthropic endpoint should be enabled")
	}
}

// TestManualCreateTrialsProtocols verifies manual model registration only
// creates endpoints for protocols that actually answer, and rejects models
// unreachable on both.
func TestManualCreateTrialsProtocols(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newDiscoveryStubHub(t, nil)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	create := func(modelID string) *http.Response {
		t.Helper()
		return doPost(t, ts.URL+"/api/models", map[string]interface{}{
			"hub_id":   hubID,
			"model_id": modelID,
		})
	}

	// Both protocols answer: two endpoints.
	resp := create("both-ok")
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("both-ok: expected 201, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	if got := len(models["both-ok"]["endpoints"].([]interface{})); got != 2 {
		t.Errorf("both-ok: expected 2 endpoints, got %d", got)
	}

	// Only openai answers: one endpoint, still 201.
	stub.setFailing("one-sided", "anthropic")
	resp = create("one-sided")
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("one-sided: expected 201, got %d", resp.StatusCode)
	}
	models = listModelsViaAPI(t, ts.URL)
	m := models["one-sided"]
	if !hasEndpoint(m, "openai") || hasEndpoint(m, "anthropic") {
		t.Errorf("one-sided: expected only the openai endpoint, got %v", m["endpoints"])
	}

	// Neither protocol answers: 400 and nothing is registered.
	stub.setFailing("dead-model", "anthropic")
	stub.setFailing("dead-model", "openai")
	resp = create("dead-model")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("dead-model: expected 400, got %d", resp.StatusCode)
	}
	if _, ok := listModelsViaAPI(t, ts.URL)["dead-model"]; ok {
		t.Error("dead-model must not be registered")
	}
}

// TestPruneDeadEndpoints verifies the cleanup removes disabled endpoints
// that never had a successful probe (with their history), keeps disabled
// endpoints that worked before, and is audited.
func TestPruneDeadEndpoints(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	hubID := createHubViaAPI(t, ts.URL, stubHub.URL)
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "prune-model",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	var openaiID, anthropicID int64
	for _, raw := range models["prune-model"]["endpoints"].([]interface{}) {
		ep := raw.(map[string]interface{})
		if ep["protocol"].(string) == "openai" {
			openaiID = int64(ep["id"].(float64))
		} else {
			anthropicID = int64(ep["id"].(float64))
		}
	}

	// openai endpoint: a successful probe, then disabled by the admin.
	runProbeRound(t, ts, openaiID)
	patchResp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, openaiID), map[string]interface{}{"enabled": false})
	patchResp.Body.Close()

	// anthropic endpoint: never probed successfully — simulate the legacy
	// state (disabled since a failed discovery trial) by seeding one failed
	// probe and disabling it directly in the store the test owns.
	if _, err := db.CreateProbe(store.Probe{
		EndpointID: anthropicID, OK: false, HTTPStatus: 503, LatencyMs: 100,
	}); err != nil {
		t.Fatalf("seed failed probe: %v", err)
	}
	enabled := false
	if _, err := db.UpdateEndpoint(anthropicID, &enabled, store.IntervalPatch{}); err != nil {
		t.Fatalf("disable anthropic endpoint: %v", err)
	}

	// Prune: only the never-succeeded endpoint goes.
	pruneResp := doPost(t, ts.URL+"/api/endpoints/prune-dead", nil)
	defer pruneResp.Body.Close()
	if pruneResp.StatusCode != http.StatusOK {
		t.Fatalf("prune: expected 200, got %d", pruneResp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(pruneResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode prune: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("unmarshal prune: %v", err)
	}
	if got := int(result["pruned"].(float64)); got != 1 {
		t.Errorf("pruned: expected 1, got %d", got)
	}

	// The dead endpoint and its history are gone; the once-working disabled
	// endpoint survives.
	probeResp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes", ts.URL, anthropicID))
	probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusNotFound {
		t.Errorf("dead endpoint probes: expected 404, got %d", probeResp.StatusCode)
	}
	probeResp = doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes", ts.URL, openaiID))
	probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusOK {
		t.Errorf("surviving endpoint probes: expected 200, got %d", probeResp.StatusCode)
	}

	// Model itself is untouched; the action is audited.
	if _, ok := listModelsViaAPI(t, ts.URL)["prune-model"]; !ok {
		t.Error("model must survive endpoint pruning")
	}
	page := fetchAuditLogs(t, ts.URL, "?action=endpoint.prune_dead")
	if got := len(auditItems(t, page)); got != 1 {
		t.Errorf("expected 1 endpoint.prune_dead audit entry, got %d", got)
	}
}

// TestPruneKeepsRolledUpSuccess verifies the retention edge: a disabled
// endpoint whose raw successes aged past retention (rollups kept, raw rows
// gone) still counts as "worked before" and survives pruning.
func TestPruneKeepsRolledUpSuccess(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	hubID := createHubViaAPI(t, ts.URL, stubHub.URL)
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "veteran-model",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", resp.StatusCode)
	}
	models := listModelsViaAPI(t, ts.URL)
	epID := int64(models["veteran-model"]["endpoints"].([]interface{})[0].(map[string]interface{})["id"].(float64))

	// A success 100 days ago, rolled up into probe_rollups, then the raw row
	// is deleted by retention. The endpoint is disabled afterwards.
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	seedProbe(t, db, epID, true, 200, nil, old)
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if err := db.RollupProbesBefore(cutoff); err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if _, err := db.DeleteProbesBefore(cutoff); err != nil {
		t.Fatalf("retention delete: %v", err)
	}
	enabled := false
	if _, err := db.UpdateEndpoint(epID, &enabled, store.IntervalPatch{}); err != nil {
		t.Fatalf("disable endpoint: %v", err)
	}

	pruneResp := doPost(t, ts.URL+"/api/endpoints/prune-dead", nil)
	defer pruneResp.Body.Close()
	if pruneResp.StatusCode != http.StatusOK {
		t.Fatalf("prune: expected 200, got %d", pruneResp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(pruneResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode prune: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("unmarshal prune: %v", err)
	}
	if got := int(result["pruned"].(float64)); got != 0 {
		t.Errorf("pruned: expected 0 (rollup success evidence must protect it), got %d", got)
	}

	// The endpoint still exists.
	detailResp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epID))
	detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Errorf("endpoint should survive pruning, got %d", detailResp.StatusCode)
	}
}
