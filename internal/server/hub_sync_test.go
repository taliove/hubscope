package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// getHubViaAPI fetches GET /api/hubs and returns the hub with the given ID.
func getHubViaAPI(t *testing.T, baseURL string, hubID int) map[string]interface{} {
	t.Helper()
	resp := doGet(t, baseURL+"/api/hubs")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list hubs: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode hubs: %v", err)
	}
	var hubs []map[string]interface{}
	if err := json.Unmarshal(env.Data, &hubs); err != nil {
		t.Fatalf("unmarshal hubs: %v", err)
	}
	for _, h := range hubs {
		if int(h["id"].(float64)) == hubID {
			return h
		}
	}
	t.Fatalf("hub %d not found in list", hubID)
	return nil
}

// waitForHubSyncStatus polls the hub until its sync_status equals want.
func waitForHubSyncStatus(t *testing.T, baseURL string, hubID int, want string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		hub := getHubViaAPI(t, baseURL, hubID)
		if hub["sync_status"] == want {
			return hub
		}
		time.Sleep(20 * time.Millisecond)
	}
	hub := getHubViaAPI(t, baseURL, hubID)
	t.Fatalf("hub %d sync_status: expected %q within timeout, last seen %v", hubID, want, hub["sync_status"])
	return nil
}

// syncHubViaAPI posts to the per-hub sync endpoint and returns the response.
func syncHubViaAPI(t *testing.T, baseURL string, hubID int) *http.Response {
	t.Helper()
	return doPost(t, fmt.Sprintf("%s/api/hubs/%d/sync", baseURL, hubID), nil)
}

// TestCreateHubTriggersAutoSync verifies that registering a hub kicks off an
// asynchronous model sync without waiting for the periodic full sync, and
// that the resulting status lands on the hub itself.
func TestCreateHubTriggersAutoSync(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"model-a"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)

	hub := waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")
	if hub["last_synced_at"] == nil || hub["last_synced_at"] == "" {
		t.Errorf("last_synced_at should be set after a successful sync, got %v", hub["last_synced_at"])
	}
	if hub["last_sync_error"] != nil {
		t.Errorf("last_sync_error should be null after success, got %v", hub["last_sync_error"])
	}

	models := listModelsViaAPI(t, ts.URL)
	if _, ok := models["model-a"]; !ok {
		t.Fatalf("auto-sync after hub creation should register model-a, got %v", models)
	}
}

// TestHubSyncEndpointConflictAndRerun verifies the manual per-hub sync
// trigger: it returns 409 while a sync is already in flight, 202 otherwise,
// and 404 for an unknown hub.
func TestHubSyncEndpointConflictAndRerun(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"model-a"})
	// The auto-sync triggered by hub creation blocks inside /v1/models,
	// keeping the sync in flight deterministically. The cleanup guarantees a
	// failed assertion cannot park the stub handler and hang the test binary.
	stub.holdList()
	t.Cleanup(stub.releaseList)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)

	resp := syncHubViaAPI(t, ts.URL, hubID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("sync while in flight: expected 409, got %d", resp.StatusCode)
	}

	stub.releaseList()
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// A manual re-trigger picks up listing changes.
	stub.setModels([]string{"model-a", "model-b"})
	resp = syncHubViaAPI(t, ts.URL, hubID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("manual sync: expected 202, got %d", resp.StatusCode)
	}

	// Wait for the outcome (model-b registered), then for the terminal status.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := listModelsViaAPI(t, ts.URL)["model-b"]; ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := listModelsViaAPI(t, ts.URL)["model-b"]; !ok {
		t.Fatal("manual re-sync should register model-b")
	}
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	resp = syncHubViaAPI(t, ts.URL, 99999)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("sync unknown hub: expected 404, got %d", resp.StatusCode)
	}
}

// TestHubSyncFailurePersistsStatus verifies that a hub whose model listing
// fails ends up with sync_status=failed and a non-empty error message.
func TestHubSyncFailurePersistsStatus(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, nil)
	stub.setListFailing(true)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)

	hub := waitForHubSyncStatus(t, ts.URL, hubID, "failed")
	msg, _ := hub["last_sync_error"].(string)
	if msg == "" {
		t.Errorf("last_sync_error should describe the failure, got %v", hub["last_sync_error"])
	}

	// A later recovery is reflected too: the listing heals, a manual re-sync
	// succeeds, and the error clears.
	stub.setListFailing(false)
	stub.setModels([]string{"model-a"})
	resp := syncHubViaAPI(t, ts.URL, hubID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("re-sync after failure: expected 202, got %d", resp.StatusCode)
	}
	hub = waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")
	if hub["last_sync_error"] != nil {
		t.Errorf("last_sync_error should clear after a successful sync, got %v", hub["last_sync_error"])
	}
}
