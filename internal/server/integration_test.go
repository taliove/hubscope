package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// envelope wraps successful responses
type envelope struct {
	Data json.RawMessage `json:"data"`
}

// errorEnvelope wraps error responses
type errorEnvelope struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// TestWalkingSkeleton is the main black-box integration test that covers ticket 01.
// It uses a real SQLite database and a stubbed Hub HTTP server.
func TestWalkingSkeleton(t *testing.T) {
	// Setup temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Setup stub Hub server
	stubHub := newStubHubServer()
	defer stubHub.Close()

	// Setup API server (seed the test user so authedClient can log in).
	seedTestUser(t, db)
	apiServer := server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
	)
	ts := httptest.NewServer(apiServer)
	defer ts.Close()

	t.Run("create_hub_and_verify_token_masking", func(t *testing.T) {
		// Create a hub with a full token
		createReq := map[string]interface{}{
			"name":     "Test Hub",
			"base_url": stubHub.URL,
			"token":    "sk-ant-1234567890abcdef",
		}
		resp := doPost(t, ts.URL+"/api/hubs", createReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		// List hubs and verify token is masked
		listResp := doGet(t, ts.URL+"/api/hubs")
		if listResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", listResp.StatusCode)
		}

		var env envelope
		if err := json.NewDecoder(listResp.Body).Decode(&env); err != nil {
			t.Fatalf("decode error: %v", err)
		}

		var hubs []map[string]interface{}
		if err := json.Unmarshal(env.Data, &hubs); err != nil {
			t.Fatalf("unmarshal hubs: %v", err)
		}

		if len(hubs) != 1 {
			t.Fatalf("expected 1 hub, got %d", len(hubs))
		}

		hub := hubs[0]
		if _, exists := hub["token"]; exists {
			t.Error("token should not be in response")
		}

		tokenHint, ok := hub["token_hint"].(string)
		if !ok || tokenHint != "…cdef" {
			t.Errorf("expected token_hint '…cdef', got %v", tokenHint)
		}
	})

	t.Run("update_hub_name_without_changing_token", func(t *testing.T) {
		// Get existing hub ID
		listResp := doGet(t, ts.URL+"/api/hubs")
		var env envelope
		json.NewDecoder(listResp.Body).Decode(&env)
		var hubs []map[string]interface{}
		json.Unmarshal(env.Data, &hubs)
		hubID := int(hubs[0]["id"].(float64))

		// Update only the name
		updateReq := map[string]interface{}{
			"name": "Updated Hub Name",
		}
		resp := doPut(t, fmt.Sprintf("%s/api/hubs/%d", ts.URL, hubID), updateReq)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		// Verify name changed
		json.NewDecoder(resp.Body).Decode(&env)
		var updatedHub map[string]interface{}
		json.Unmarshal(env.Data, &updatedHub)
		if updatedHub["name"] != "Updated Hub Name" {
			t.Errorf("name not updated: %v", updatedHub["name"])
		}
	})

	t.Run("delete_hub_with_models_returns_409", func(t *testing.T) {
		// Get hub ID
		listResp := doGet(t, ts.URL+"/api/hubs")
		var env envelope
		json.NewDecoder(listResp.Body).Decode(&env)
		var hubs []map[string]interface{}
		json.Unmarshal(env.Data, &hubs)
		hubID := int(hubs[0]["id"].(float64))

		// Create a model under this hub
		createModelReq := map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "claude-opus-4-8",
		}
		doPost(t, ts.URL+"/api/models", createModelReq)

		// Try to delete hub - should fail with 409
		resp := doDelete(t, fmt.Sprintf("%s/api/hubs/%d", ts.URL, hubID))
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.StatusCode)
		}
	})

	t.Run("create_model_auto_creates_two_endpoints", func(t *testing.T) {
		// Get hub ID
		listResp := doGet(t, ts.URL+"/api/hubs")
		var env envelope
		json.NewDecoder(listResp.Body).Decode(&env)
		var hubs []map[string]interface{}
		json.Unmarshal(env.Data, &hubs)
		hubID := int(hubs[0]["id"].(float64))

		// Create model
		createModelReq := map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "test-model-1",
		}
		resp := doPost(t, ts.URL+"/api/models", createModelReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}

		// Verify response contains 2 endpoints
		json.NewDecoder(resp.Body).Decode(&env)
		var model map[string]interface{}
		json.Unmarshal(env.Data, &model)

		endpoints, ok := model["endpoints"].([]interface{})
		if !ok || len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %v", endpoints)
		}

		// Verify one anthropic, one openai
		protocols := make(map[string]bool)
		for _, ep := range endpoints {
			endpoint := ep.(map[string]interface{})
			protocol := endpoint["protocol"].(string)
			protocols[protocol] = true

			if !endpoint["enabled"].(bool) {
				t.Error("endpoint should be enabled by default")
			}
		}

		if !protocols["anthropic"] || !protocols["openai"] {
			t.Error("expected both anthropic and openai endpoints")
		}
	})

	t.Run("duplicate_model_id_returns_409", func(t *testing.T) {
		// Get hub ID
		listResp := doGet(t, ts.URL+"/api/hubs")
		var env envelope
		json.NewDecoder(listResp.Body).Decode(&env)
		var hubs []map[string]interface{}
		json.Unmarshal(env.Data, &hubs)
		hubID := int(hubs[0]["id"].(float64))

		// Try to create duplicate model
		createModelReq := map[string]interface{}{
			"hub_id":   hubID,
			"model_id": "test-model-1", // Already created above
		}
		resp := doPost(t, ts.URL+"/api/models", createModelReq)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.StatusCode)
		}
	})

	t.Run("probe_endpoint_success_both_streaming_modes", func(t *testing.T) {
		// Get an endpoint ID
		modelsResp := doGet(t, ts.URL+"/api/models")
		var env envelope
		json.NewDecoder(modelsResp.Body).Decode(&env)
		var models []map[string]interface{}
		json.Unmarshal(env.Data, &models)

		endpoints := models[0]["endpoints"].([]interface{})
		endpointID := int(endpoints[0].(map[string]interface{})["id"].(float64))

		// Trigger probe
		resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, endpointID), nil)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		json.NewDecoder(resp.Body).Decode(&env)
		var result map[string]interface{}
		json.Unmarshal(env.Data, &result)

		results, ok := result["results"].([]interface{})
		if !ok || len(results) != 2 {
			t.Fatalf("expected 2 probe results, got %v", results)
		}

		// First result: non-streaming
		nonStream := results[0].(map[string]interface{})
		if nonStream["streaming"].(bool) != false {
			t.Error("first result should be non-streaming")
		}
		if nonStream["ok"].(bool) != true {
			t.Error("probe should succeed")
		}
		if nonStream["http_status"].(float64) != 200 {
			t.Error("expected http_status 200")
		}
		if nonStream["latency_ms"].(float64) < 0 {
			t.Error("latency_ms should be non-negative")
		}
		if nonStream["ttft_ms"] != nil {
			t.Error("non-streaming ttft_ms should be null")
		}
		if nonStream["input_tokens"] == nil {
			t.Error("input_tokens should not be null")
		}
		if nonStream["output_tokens"] == nil {
			t.Error("output_tokens should not be null")
		}

		// Second result: streaming
		stream := results[1].(map[string]interface{})
		if stream["streaming"].(bool) != true {
			t.Error("second result should be streaming")
		}
		if stream["ok"].(bool) != true {
			t.Error("streaming probe should succeed")
		}
		if stream["ttft_ms"] == nil {
			t.Error("streaming ttft_ms should not be null")
		}

		// The probed model ID must reach the hub verbatim (regression guard:
		// a hardcoded placeholder model ID made every probe a 503 in dogfood).
		if want := models[0]["model_id"].(string); stubHub.lastModel != want {
			t.Errorf("stub hub saw model %q, want %q", stubHub.lastModel, want)
		}
	})

	t.Run("probe_endpoint_handles_hub_503_error", func(t *testing.T) {
		// Create the hub and model while the stub is healthy (manual
		// creation trial-probes the protocols), then flip it to failing.
		stubHub.SetMode("success")
		createHubReq := map[string]interface{}{
			"name":     "Error Hub",
			"base_url": stubHub.URL,
			"token":    "test-token",
		}
		hubResp := doPost(t, ts.URL+"/api/hubs", createHubReq)
		var env envelope
		json.NewDecoder(hubResp.Body).Decode(&env)
		var errorHub map[string]interface{}
		json.Unmarshal(env.Data, &errorHub)
		errorHubID := int(errorHub["id"].(float64))

		// Create model
		createModelReq := map[string]interface{}{
			"hub_id":   errorHubID,
			"model_id": "error-model",
		}
		modelResp := doPost(t, ts.URL+"/api/models", createModelReq)
		json.NewDecoder(modelResp.Body).Decode(&env)
		var model map[string]interface{}
		json.Unmarshal(env.Data, &model)
		endpoints := model["endpoints"].([]interface{})
		endpointID := int(endpoints[0].(map[string]interface{})["id"].(float64))

		// Probe should fail gracefully
		stubHub.SetMode("error_503")
		resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, endpointID), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 even with upstream error, got %d", resp.StatusCode)
		}

		json.NewDecoder(resp.Body).Decode(&env)
		var result map[string]interface{}
		json.Unmarshal(env.Data, &result)
		results := result["results"].([]interface{})
		probeResult := results[0].(map[string]interface{})

		if probeResult["ok"].(bool) != false {
			t.Error("probe should fail when hub returns 503")
		}
		if probeResult["http_status"].(float64) != 503 {
			t.Error("should capture http_status 503")
		}
		errorSummary := probeResult["error_summary"].(string)
		if errorSummary == "" {
			t.Error("error_summary should not be empty")
		}
	})

	t.Run("probe_endpoint_handles_network_failure", func(t *testing.T) {
		// Create the hub and model while the stub is healthy, then break
		// the hub's address so probes hit a network error.
		stubHub.SetMode("success")
		createHubReq := map[string]interface{}{
			"name":     "Broken Hub",
			"base_url": stubHub.URL,
			"token":    "test-token",
		}
		hubResp := doPost(t, ts.URL+"/api/hubs", createHubReq)
		var env envelope
		json.NewDecoder(hubResp.Body).Decode(&env)
		var brokenHub map[string]interface{}
		json.Unmarshal(env.Data, &brokenHub)
		brokenHubID := int(brokenHub["id"].(float64))

		// Create model
		createModelReq := map[string]interface{}{
			"hub_id":   brokenHubID,
			"model_id": "broken-model",
		}
		modelResp := doPost(t, ts.URL+"/api/models", createModelReq)
		json.NewDecoder(modelResp.Body).Decode(&env)
		var model map[string]interface{}
		json.Unmarshal(env.Data, &model)
		endpoints := model["endpoints"].([]interface{})
		endpointID := int(endpoints[0].(map[string]interface{})["id"].(float64))

		// Break the hub: its base_url now points at an unreachable address.
		deadURL := "http://localhost:1"
		putResp := doPut(t, fmt.Sprintf("%s/api/hubs/%d", ts.URL, brokenHubID), map[string]interface{}{
			"base_url": deadURL,
		})
		putResp.Body.Close()
		if putResp.StatusCode != http.StatusOK {
			t.Fatalf("break hub: expected 200, got %d", putResp.StatusCode)
		}

		// Probe should handle network error
		resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, endpointID), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		json.NewDecoder(resp.Body).Decode(&env)
		var result map[string]interface{}
		json.Unmarshal(env.Data, &result)
		results := result["results"].([]interface{})
		probeResult := results[0].(map[string]interface{})

		if probeResult["ok"].(bool) != false {
			t.Error("probe should fail on network error")
		}
		if probeResult["http_status"].(float64) != 0 {
			t.Error("network failure should have http_status 0")
		}
		if probeResult["error_summary"].(string) == "" {
			t.Error("error_summary should describe network error")
		}
	})

	t.Run("get_probes_history_with_limit", func(t *testing.T) {
		// Get an endpoint that has probes
		modelsResp := doGet(t, ts.URL+"/api/models")
		var env envelope
		json.NewDecoder(modelsResp.Body).Decode(&env)
		var models []map[string]interface{}
		json.Unmarshal(env.Data, &models)
		endpoints := models[0]["endpoints"].([]interface{})
		endpointID := int(endpoints[0].(map[string]interface{})["id"].(float64))

		// Get probes history
		resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes?limit=10", ts.URL, endpointID))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		json.NewDecoder(resp.Body).Decode(&env)
		var probes []map[string]interface{}
		json.Unmarshal(env.Data, &probes)

		if len(probes) < 2 {
			t.Error("should have at least 2 probe records from previous test")
		}

		// Verify records are sorted by time descending
		if len(probes) >= 2 {
			first := probes[0]["created_at"].(string)
			second := probes[1]["created_at"].(string)
			if first < second {
				t.Error("probes should be sorted by created_at descending")
			}
		}
	})
}

// stubHubServer simulates an AI Hub for testing
type stubHubServer struct {
	*httptest.Server
	mode string
	// lastModel records the model field of the most recent request, so tests
	// can assert the probed model ID actually reached the wire. Like a real
	// hub, the stub rejects empty model IDs with 503.
	lastModel string
}

func newStubHubServer() *stubHubServer {
	stub := &stubHubServer{mode: "success"}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.handleRequest(w, r)
	}))
	return stub
}

func (s *stubHubServer) SetMode(mode string) {
	s.mode = mode
}

func (s *stubHubServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if s.mode == "error_503" {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "No available providers for this model",
			},
		})
		return
	}

	// Determine protocol by path
	isAnthropic := strings.HasSuffix(r.URL.Path, "/v1/messages")
	isOpenAI := strings.HasSuffix(r.URL.Path, "/v1/chat/completions")

	if !isAnthropic && !isOpenAI {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Parse request to determine streaming
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	json.Unmarshal(body, &req)
	streaming, _ := req["stream"].(bool)

	// Record the requested model and reject empty ones like a real hub would.
	model, _ := req["model"].(string)
	s.lastModel = model
	if model == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "No available providers for this model",
			},
		})
		return
	}

	// Simulate successful responses
	if streaming {
		s.handleStreaming(w, isAnthropic)
	} else {
		s.handleNonStreaming(w, isAnthropic)
	}
}

func (s *stubHubServer) handleNonStreaming(w http.ResponseWriter, isAnthropic bool) {
	w.Header().Set("Content-Type", "application/json")

	if isAnthropic {
		resp := map[string]interface{}{
			"id":      "msg_test",
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]string{{"type": "text", "text": "pong"}},
			"usage": map[string]int{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		json.NewEncoder(w).Encode(resp)
	} else {
		resp := map[string]interface{}{
			"id":     "chatcmpl_test",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{"message": map[string]string{"role": "assistant", "content": "pong"}},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *stubHubServer) handleStreaming(w http.ResponseWriter, isAnthropic bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Simulate TTFT delay
	time.Sleep(50 * time.Millisecond)

	if isAnthropic {
		// Anthropic streaming format
		events := []string{
			`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"pong"}}`,
			`{"type":"content_block_stop","index":1}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`{"type":"message_stop"}`,
		}
		for _, event := range events {
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", event)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		// OpenAI streaming format
		chunks := []string{
			`{"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant","reasoning_content":"hmm"},"index":0}]}`,
			`{"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"delta":{"content":"pong"},"index":0}]}`,
			`{"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

// HTTP helper functions. All helpers go through authedClient so write
// requests carry a valid admin session cookie after ticket 07.
func doGet(t *testing.T, url string) *http.Response {
	resp, err := authedClient(t, url).Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func doPost(t *testing.T, url string, body interface{}) *http.Response {
	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}
	resp, err := authedClient(t, url).Post(url, "application/json", reqBody)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func doPut(t *testing.T, url string, body interface{}) *http.Response {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := authedClient(t, url).Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return resp
}

func doDelete(t *testing.T, url string) *http.Response {
	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := authedClient(t, url).Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp
}
