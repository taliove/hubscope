package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/taliove/hubscope/internal/server"
)

// TestPingMonitoring_ImageEndpointsNotProbed verifies that the prober skips
// Ping protocol endpoints (images_generation, images_edit, video_generation),
// so they never produce probe records even when manually triggered (spec 0018
// T1, GH #97). Chat protocols (anthropic, openai) continue to be probed.
func TestPingMonitoring_ImageEndpointsNotProbed(t *testing.T) {
	db := openTempDB(t)
	seedTestUser(t, db)

	stubHub := newDiscoveryStubHub(t, []string{"test-model"})
	defer stubHub.Close()

	apiServer := server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSyncDiscovery(),
	)
	ts := httptest.NewServer(apiServer)
	defer ts.Close()

	// Create a hub
	hubResp := doPost(t, ts.URL+"/api/hubs", map[string]interface{}{
		"name":     "Ping Test Hub",
		"base_url": stubHub.URL,
		"token":    "sk-test-token",
	})
	if hubResp.StatusCode != http.StatusCreated {
		t.Fatalf("create hub: status %d", hubResp.StatusCode)
	}
	var hubEnv envelope
	json.NewDecoder(hubResp.Body).Decode(&hubEnv)
	hubResp.Body.Close()
	var hub map[string]interface{}
	json.Unmarshal(hubEnv.Data, &hub)
	hubID := int64(hub["id"].(float64))

	// Directly create endpoints via DB: one chat (anthropic) and one image
	// (images_generation). This bypasses discovery's trial-probe logic, which
	// is modified in a later ticket (spec 0018 T2+). Here we only test that
	// the prober skips Ping protocols.
	chatModel, err := db.CreateModel(hubID, "chat-model", []string{"anthropic"})
	if err != nil {
		t.Fatalf("create chat model: %v", err)
	}
	imageModel, err := db.CreateModel(hubID, "image-model", []string{"images_generation"})
	if err != nil {
		t.Fatalf("create image model: %v", err)
	}

	chatEndpoints, err := db.ListEndpointsByModelID(chatModel.ID)
	if err != nil || len(chatEndpoints) == 0 {
		t.Fatalf("list chat endpoints: %v", err)
	}
	chatEndpointID := chatEndpoints[0].ID

	imageEndpoints, err := db.ListEndpointsByModelID(imageModel.ID)
	if err != nil || len(imageEndpoints) == 0 {
		t.Fatalf("list image endpoints: %v", err)
	}
	imageEndpointID := imageEndpoints[0].ID

	// Trigger manual probe on chat endpoint - should succeed and create records
	chatProbeResp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, chatEndpointID), nil)
	if chatProbeResp.StatusCode != http.StatusOK {
		t.Fatalf("chat probe: expected 200, got %d", chatProbeResp.StatusCode)
	}
	chatProbeResp.Body.Close()

	// Trigger manual probe on image endpoint - prober should skip it (no records)
	imageProbeResp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, imageEndpointID), nil)
	imageProbeResp.Body.Close()

	// Check probe history: chat should have records, image should have zero
	chatHistoryResp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes", ts.URL, chatEndpointID))
	if chatHistoryResp.StatusCode != http.StatusOK {
		t.Fatalf("get chat history: status %d", chatHistoryResp.StatusCode)
	}
	var chatEnv envelope
	json.NewDecoder(chatHistoryResp.Body).Decode(&chatEnv)
	chatHistoryResp.Body.Close()
	var chatHistory []map[string]interface{}
	json.Unmarshal(chatEnv.Data, &chatHistory)

	if len(chatHistory) == 0 {
		t.Errorf("chat endpoint (anthropic) should have probe records, got 0")
	}

	imageHistoryResp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes", ts.URL, imageEndpointID))
	if imageHistoryResp.StatusCode != http.StatusOK {
		t.Fatalf("get image history: status %d", imageHistoryResp.StatusCode)
	}
	var imageEnv envelope
	json.NewDecoder(imageHistoryResp.Body).Decode(&imageEnv)
	imageHistoryResp.Body.Close()
	var imageHistory []map[string]interface{}
	json.Unmarshal(imageEnv.Data, &imageHistory)

	if len(imageHistory) != 0 {
		t.Errorf("image endpoint (images_generation) should have 0 probe records (prober skipped), got %d", len(imageHistory))
	}
}
