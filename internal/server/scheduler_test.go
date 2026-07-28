package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/prober"
	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// delayStubHub is a stub Hub with a configurable per-request delay and
// in-flight request tracking, used to drive scheduler tests.
type delayStubHub struct {
	*httptest.Server
	delay time.Duration

	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	total       int
}

func newDelayStubHub(t *testing.T, delay time.Duration) *delayStubHub {
	t.Helper()
	stub := &delayStubHub{delay: delay}
	stub.Server = httptest.NewServer(http.HandlerFunc(stub.handle))
	t.Cleanup(stub.Close)
	return stub
}

func (s *delayStubHub) handle(w http.ResponseWriter, r *http.Request) {
	// Model-listing calls come from the auto-sync triggered by hub creation.
	// Answer them with an empty list, without delay and outside the in-flight
	// accounting, so they stay invisible to scheduler concurrency assertions.
	if strings.HasSuffix(r.URL.Path, "/v1/models") {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
		return
	}

	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.maxInFlight {
		s.maxInFlight = s.inFlight
	}
	s.total++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()

	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)

	isAnthropic := strings.HasSuffix(r.URL.Path, "/v1/messages")
	if req.Stream {
		writeStubStream(w, isAnthropic)
		return
	}
	writeStubNonStream(w, isAnthropic)
}

func (s *delayStubHub) totalRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

func (s *delayStubHub) peakInFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxInFlight
}

// writeStubNonStream emits a minimal successful non-streaming response.
func writeStubNonStream(w http.ResponseWriter, isAnthropic bool) {
	w.Header().Set("Content-Type", "application/json")
	if isAnthropic {
		fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant",`+
			`"content":[{"type":"text","text":"pong"}],"usage":{"input_tokens":10,"output_tokens":5}}`)
		return
	}
	fmt.Fprint(w, `{"id":"chatcmpl_test","object":"chat.completion",`+
		`"choices":[{"message":{"role":"assistant","content":"pong"}}],`+
		`"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
}

// writeStubStream emits a minimal successful SSE stream for either protocol.
func writeStubStream(w http.ResponseWriter, isAnthropic bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var events []string
	if isAnthropic {
		events = []string{
			`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`{"type":"message_stop"}`,
		}
	} else {
		events = []string{
			`{"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"delta":{"content":"pong"},"index":0}]}`,
			`{"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
			`[DONE]`,
		}
	}
	for _, event := range events {
		fmt.Fprintf(w, "data: %s\n\n", event)
		flusher.Flush()
	}
}

// openTempDB opens a real SQLite database in a temporary directory and seeds
// the test super_admin (username "admin", password testAdminPassword) so any
// helper that goes through authedClient (doPost, doGet, …) can log in.
func openTempDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)
	return db
}

// newTestAPIServer starts the full API server over httptest. Rate limits
// are disabled (zero tiers): probe-heavy tests would otherwise trip the
// write budget. Limit behavior is covered by dedicated tests with tiny tiers.
// The fixed session secret lets forgeSessionToken reproduce tokens without
// reading the DB. The test user is seeded by openTempDB (or manually when
// the DB is opened via store.Open).
//
// Eval and discovery triggers run synchronously (WithSyncEval +
// WithSyncDiscovery, ticket 100): the structural seams guarantee no
// goroutine outlives the request, so no tail write can race TempDir
// cleanup. Tests that must observe in-flight semantics (sync conflicts,
// running reports, mid-flight progress) build an explicit async server
// instead and document their drain — see TestHubSyncEndpointConflictAndRerun
// and setupAsyncEvalEnv.
func newTestAPIServer(t *testing.T, db *store.DB) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSyncEval(),
		server.WithSyncDiscovery(),
	))
	t.Cleanup(ts.Close)
	return ts
}

// startScheduler runs a scheduler on the fake clock with a 1s poll interval
// and registers cleanup that verifies graceful shutdown.
func startScheduler(t *testing.T, db *store.DB, client *hubclient.Client, clock *scheduler.FakeClock) {
	t.Helper()
	sched := scheduler.New(db, prober.New(db, client), clock, scheduler.WithPollInterval(time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sched.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("scheduler did not stop within 10s of cancellation")
		}
	})
}

// createModelEndpoints creates a hub pointing at hubURL plus one model, and
// returns the IDs of its two auto-created endpoints in creation order.
func createModelEndpoints(t *testing.T, baseURL, hubURL, modelID string) []int {
	t.Helper()

	hubResp := doPost(t, baseURL+"/api/hubs", map[string]interface{}{
		"name":     "hub-" + modelID,
		"base_url": hubURL,
		"token":    "test-token-0000",
	})
	defer hubResp.Body.Close()
	var env envelope
	if err := json.NewDecoder(hubResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode hub: %v", err)
	}
	var hub map[string]interface{}
	if err := json.Unmarshal(env.Data, &hub); err != nil {
		t.Fatalf("unmarshal hub: %v", err)
	}

	modelResp := doPost(t, baseURL+"/api/models", map[string]interface{}{
		"hub_id":   hub["id"],
		"model_id": modelID,
	})
	defer modelResp.Body.Close()
	if modelResp.StatusCode != http.StatusCreated {
		t.Fatalf("create model %s: expected 201, got %d", modelID, modelResp.StatusCode)
	}
	if err := json.NewDecoder(modelResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	var model map[string]interface{}
	if err := json.Unmarshal(env.Data, &model); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}

	var ids []int
	for _, ep := range model["endpoints"].([]interface{}) {
		ids = append(ids, int(ep.(map[string]interface{})["id"].(float64)))
	}
	return ids
}

// probeRecords fetches the probe history of an endpoint via the API.
func probeRecords(t *testing.T, baseURL string, endpointID int) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes?limit=200", baseURL, endpointID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list probes for endpoint %d: status %d", endpointID, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode probes: %v", err)
	}
	var probes []map[string]interface{}
	if err := json.Unmarshal(env.Data, &probes); err != nil {
		t.Fatalf("unmarshal probes: %v", err)
	}
	return probes
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// waitForProbeCount waits until an endpoint has at least want probe records.
func waitForProbeCount(t *testing.T, baseURL string, endpointID, want int) {
	t.Helper()
	waitFor(t, fmt.Sprintf("%d probe records on endpoint %d", want, endpointID), func() bool {
		return len(probeRecords(t, baseURL, endpointID)) >= want
	})
}

// assertProbeCountStable waits a grace period and then asserts the endpoint
// has exactly want records, catching unwanted extra rounds.
func assertProbeCountStable(t *testing.T, baseURL string, endpointID, want int, grace time.Duration) {
	t.Helper()
	time.Sleep(grace)
	if got := len(probeRecords(t, baseURL, endpointID)); got != want {
		t.Fatalf("endpoint %d: expected exactly %d probe records, got %d", endpointID, want, got)
	}
}

// doPatch issues a PATCH request with a JSON body.
func doPatch(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest("PATCH", url, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := authedClient(t, url).Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	return resp
}

// patchEndpoint issues PATCH /api/endpoints/{id}.
func patchEndpoint(t *testing.T, baseURL string, id int, body map[string]interface{}) *http.Response {
	t.Helper()
	return doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", baseURL, id), body)
}
