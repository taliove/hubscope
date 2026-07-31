package server_test

import (
	"net/http"
	"testing"
)

// TestModelCapabilityPatch covers spec 0018 T7 (GH #105): PATCH
// /api/models/{id} updates the model's capability and reconciles the endpoint
// set — protocols the new capability implies but are missing get created
// (chat via trial, image/video trial-free), protocols no longer implied get
// disabled with history preserved.
func TestModelCapabilityPatch(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	t.Run("image to chat: image endpoints disabled, chat trialed in", func(t *testing.T) {
		stub := newDiscoveryStubHub(t, nil)
		defer stub.Close()
		hubID := createHubViaAPI(t, ts.URL, stub.URL)
		waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

		// Manual image model: trial-free images_generation + images_edit, no
		// chat endpoints (GH #100).
		stub.setImageMode("dall-x", "success")
		resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "dall-x",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create dall-x: expected 201, got %d", resp.StatusCode)
		}

		models := listModelsViaAPI(t, ts.URL)
		model := models["dall-x"]
		if !hasEndpoint(model, "images_generation") || hasEndpoint(model, "anthropic") {
			t.Fatalf("dall-x should start with image endpoints only, got %v", model["endpoints"])
		}
		modelDBID := int64(model["id"].(float64))

		// Flip capability to chat: the stub answers both chat protocols, so
		// both get trialed in; the image endpoints are disabled (kept).
		patchResp := doPatch(t, ts.URL+"/api/models/"+itoa(modelDBID), map[string]interface{}{
			"capability": "chat",
		})
		patchResp.Body.Close()
		if patchResp.StatusCode != http.StatusOK {
			t.Fatalf("patch capability: expected 200, got %d", patchResp.StatusCode)
		}

		models = listModelsViaAPI(t, ts.URL)
		model = models["dall-x"]
		if model["capability"] != "chat" {
			t.Errorf("capability after patch: expected chat, got %v", model["capability"])
		}
		if !endpointEnabled(t, model, "anthropic") || !endpointEnabled(t, model, "openai") {
			t.Error("chat endpoints should be created and enabled after image→chat")
		}
		if endpointEnabled(t, model, "images_generation") || endpointEnabled(t, model, "images_edit") {
			t.Error("image endpoints should be disabled (history preserved) after image→chat")
		}
	})

	t.Run("chat to image: chat endpoints disabled, image created trial-free", func(t *testing.T) {
		stub := newDiscoveryStubHub(t, nil)
		defer stub.Close()
		hubID := createHubViaAPI(t, ts.URL, stub.URL)
		waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

		// Manual chat model: trial-probed anthropic + openai endpoints.
		resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "llm-x",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create llm-x: expected 201, got %d", resp.StatusCode)
		}
		models := listModelsViaAPI(t, ts.URL)
		modelDBID := int64(models["llm-x"]["id"].(float64))

		imageReqsBefore := len(stub.imageRequests("llm-x"))

		patchResp := doPatch(t, ts.URL+"/api/models/"+itoa(modelDBID), map[string]interface{}{
			"capability": "image",
		})
		patchResp.Body.Close()
		if patchResp.StatusCode != http.StatusOK {
			t.Fatalf("patch capability: expected 200, got %d", patchResp.StatusCode)
		}

		models = listModelsViaAPI(t, ts.URL)
		model := models["llm-x"]
		if !endpointEnabled(t, model, "images_generation") || !endpointEnabled(t, model, "images_edit") {
			t.Error("image endpoints should be created and enabled after chat→image")
		}
		if endpointEnabled(t, model, "anthropic") || endpointEnabled(t, model, "openai") {
			t.Error("chat endpoints should be disabled (history preserved) after chat→image")
		}
		// Trial-free: no image trial request must have reached the stub.
		if got := len(stub.imageRequests("llm-x")) - imageReqsBefore; got != 0 {
			t.Errorf("chat→image sent %d image trial requests, want 0 (trial-free)", got)
		}
	})

	t.Run("chat to video: video endpoint created trial-free", func(t *testing.T) {
		stub := newDiscoveryStubHub(t, nil)
		defer stub.Close()
		hubID := createHubViaAPI(t, ts.URL, stub.URL)
		waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

		resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "llm-vid",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create llm-vid: expected 201, got %d", resp.StatusCode)
		}
		models := listModelsViaAPI(t, ts.URL)
		modelDBID := int64(models["llm-vid"]["id"].(float64))

		patchResp := doPatch(t, ts.URL+"/api/models/"+itoa(modelDBID), map[string]interface{}{
			"capability": "video",
		})
		patchResp.Body.Close()
		if patchResp.StatusCode != http.StatusOK {
			t.Fatalf("patch capability: expected 200, got %d", patchResp.StatusCode)
		}

		models = listModelsViaAPI(t, ts.URL)
		model := models["llm-vid"]
		if !endpointEnabled(t, model, "video_generation") {
			t.Error("video_generation endpoint should be created and enabled after chat→video")
		}
		if endpointEnabled(t, model, "anthropic") || endpointEnabled(t, model, "openai") {
			t.Error("chat endpoints should be disabled after chat→video")
		}
	})

	t.Run("invalid capability rejected", func(t *testing.T) {
		stub := newDiscoveryStubHub(t, nil)
		defer stub.Close()
		hubID := createHubViaAPI(t, ts.URL, stub.URL)
		waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

		resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "llm-bad",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create llm-bad: expected 201, got %d", resp.StatusCode)
		}
		models := listModelsViaAPI(t, ts.URL)
		modelDBID := int64(models["llm-bad"]["id"].(float64))

		patchResp := doPatch(t, ts.URL+"/api/models/"+itoa(modelDBID), map[string]interface{}{
			"capability": "embedding",
		})
		patchResp.Body.Close()
		if patchResp.StatusCode != http.StatusBadRequest {
			t.Errorf("patch capability=embedding: expected 400, got %d", patchResp.StatusCode)
		}
	})
}
