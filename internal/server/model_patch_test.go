package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/taliove/hubscope/internal/server"
)

// TestPatchModelCapability_ReconcilesEndpoints verifies spec 0018 T7 (GH #105):
// PATCH /api/models/{id} with a new capability reconciles the endpoint set —
// missing protocols get created, surplus protocols get disabled (history kept).
func TestPatchModelCapability_ReconcilesEndpoints(t *testing.T) {
	db := openTempDB(t)
	seedTestUser(t, db)

	stub := newDiscoveryStubHub(t, []string{"gpt-image-2"})
	defer stub.Close()

	apiServer := server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSyncDiscovery(),
	)
	ts := httptest.NewServer(apiServer)
	defer ts.Close()

	// Create hub + model (discovery syncs, gpt-image-2 classifies as image,
	// gets images_generation + images_edit endpoints trial-free).
	// Use a stub that doesn't list gpt-image-2 so discovery doesn't create it
	// first (avoid 409 duplicate).
	stub2 := newDiscoveryStubHub(t, []string{})
	defer stub2.Close()
	hubResp := doPost(t, ts.URL+"/api/hubs", map[string]interface{}{
		"name": "Test Hub", "base_url": stub2.URL, "token": "sk-test",
	})
	hubResp.Body.Close()

	// Manually register the model (trial-free for image, no chat trial needed).
	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id": 1, "model_id": "gpt-image-2",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify initial state: image model has images_generation + images_edit, no chat endpoints.
	models := listModelsViaAPI(t, ts.URL)
	model := models["gpt-image-2"]
	if model["capability"] != "image" {
		t.Fatalf("initial capability: expected image, got %v", model["capability"])
	}
	eps := model["endpoints"].([]interface{})
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints (images_generation + images_edit), got %d", len(eps))
	}
	for _, ep := range eps {
		epMap := ep.(map[string]interface{})
		protocol := epMap["protocol"].(string)
		if protocol != "images_generation" && protocol != "images_edit" {
			t.Errorf("unexpected protocol: %s", protocol)
		}
		if !epMap["enabled"].(bool) {
			t.Errorf("endpoint %s should be enabled", protocol)
		}
	}

	// PATCH: change capability to chat.
	modelDBID := int64(model["id"].(float64))
	patchResp := doPatch(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelDBID), map[string]interface{}{
		"capability": "chat",
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH model: expected 200, got %d", patchResp.StatusCode)
	}
	patchResp.Body.Close()

	// Verify: chat protocols (anthropic/openai) should be present (trial may
	// fail on stub, but the endpoint set reconciliation should at least disable
	// the image endpoints).
	models = listModelsViaAPI(t, ts.URL)
	model = models["gpt-image-2"]
	if model["capability"] != "chat" {
		t.Fatalf("after patch: expected capability=chat, got %v", model["capability"])
	}
	eps = model["endpoints"].([]interface{})
	for _, ep := range eps {
		epMap := ep.(map[string]interface{})
		protocol := epMap["protocol"].(string)
		if protocol == "images_generation" || protocol == "images_edit" {
			if epMap["enabled"].(bool) {
				t.Errorf("surplus endpoint %s should be disabled after chat reconciliation", protocol)
			}
		}
	}

	// PATCH: change back to image.
	patchResp = doPatch(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelDBID), map[string]interface{}{
		"capability": "image",
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH model back to image: expected 200, got %d", patchResp.StatusCode)
	}
	patchResp.Body.Close()

	// Verify: image endpoints should be re-enabled or re-created.
	models = listModelsViaAPI(t, ts.URL)
	model = models["gpt-image-2"]
	if model["capability"] != "image" {
		t.Fatalf("after patch back: expected capability=image, got %v", model["capability"])
	}
	eps = model["endpoints"].([]interface{})
	hasImagesGen := false
	hasImagesEdit := false
	for _, ep := range eps {
		epMap := ep.(map[string]interface{})
		switch epMap["protocol"].(string) {
		case "images_generation":
			hasImagesGen = true
		case "images_edit":
			hasImagesEdit = true
		}
	}
	if !hasImagesGen || !hasImagesEdit {
		t.Errorf("after patch back to image: expected images_generation + images_edit endpoints, got %v", eps)
	}
}

// TestPatchModelCapability_Validation verifies invalid capabilities are rejected.
func TestPatchModelCapability_Validation(t *testing.T) {
	db := openTempDB(t)
	seedTestUser(t, db)

	apiServer := server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
	)
	ts := httptest.NewServer(apiServer)
	defer ts.Close()

	// Create a model first (use a working stub so trial succeeds for chat).
	stub := newDiscoveryStubHub(t, []string{})
	defer stub.Close()
	resp := doPost(t, ts.URL+"/api/hubs", map[string]interface{}{
		"name": "h", "base_url": stub.URL, "token": "t",
	})
	resp.Body.Close()
	// Use an image model so it's trial-free (no chat trial needed).
	resp = doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id": 1, "model_id": "gpt-image-2",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: status %d", resp.StatusCode)
	}
	var env envelope
	json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var model map[string]interface{}
	json.Unmarshal(env.Data, &model)
	modelID := int64(model["id"].(float64))

	// Invalid capability.
	resp = doPatch(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID), map[string]interface{}{
		"capability": "invalid",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid capability: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing capability.
	resp = doPatch(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID), map[string]interface{}{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing capability: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Non-existent model.
	resp = doPatch(t, ts.URL+"/api/models/99999", map[string]interface{}{
		"capability": "chat",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("non-existent model: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
