package hubclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/hubclient"
)

// Why this lives off the W1 seam: the server wires hubclient.New() (60s chat
// budget, 180s image budget), so distinguishing the two budgets through the
// HTTP API layer would require a stub delay of 60-180s of real time, which
// the no-sleep discipline forbids. Building the client with a 50ms budget
// makes the dispatch observable in milliseconds: chat-family calls die at
// ~50ms while image calls survive a 300ms delay because they always get
// ImageRequestTimeout. The stub is a real HTTP server (no mocked internals)
// and assertions stay on public API results — the leaf-package precedent of
// internal/store/users_test.go.
func TestProtocolTimeoutDispatch(t *testing.T) {
	const stubDelay = 300 * time.Millisecond

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(stubDelay)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/images/generations"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]string{{"b64_json": "aGVsbG8="}},
			})
		case strings.HasSuffix(r.URL.Path, "/v1/models"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]string{{"id": "m"}},
			})
		default: // /v1/chat/completions
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "pong"}},
				},
			})
		}
	}))
	defer stub.Close()

	// A 50ms client budget must bind chat-family calls (regression guard for
	// the ListModels bound too: it bypasses call(), so its budget comes from
	// its own wrap).
	fast := hubclient.NewWithTimeout(50 * time.Millisecond)

	chat := fast.Probe(context.Background(), stub.URL, "test-token", "openai", "m", false)
	if chat.OK {
		t.Error("chat probe: expected timeout failure against the 300ms stub, got success")
	}
	if chat.ErrorSummary == nil || !strings.HasPrefix(*chat.ErrorSummary, "timeout:") {
		t.Errorf("chat probe: expected a timeout: error summary, got %v", chat.ErrorSummary)
	}

	if _, err := fast.ListModels(context.Background(), stub.URL, "test-token"); err == nil {
		t.Error("ListModels: expected timeout failure against the 300ms stub, got success")
	}

	// Image calls never use the client budget: they always get the 180s
	// image timeout, so the same 50ms client still succeeds.
	image := fast.Probe(context.Background(), stub.URL, "test-token", "images_generation", "m", false)
	if !image.OK {
		t.Errorf("image probe: expected success despite the 50ms client budget, got %v", image.ErrorSummary)
	}

	// Control: with the standard 60s budget the 300ms delay is no obstacle
	// for either protocol, proving the failures above came from the budget
	// and not from a broken stub shape.
	standard := hubclient.New()
	if res := standard.Probe(context.Background(), stub.URL, "test-token", "openai", "m", false); !res.OK {
		t.Errorf("chat probe with standard budget: expected success, got %v", res.ErrorSummary)
	}
	if res := standard.Probe(context.Background(), stub.URL, "test-token", "images_generation", "m", false); !res.OK {
		t.Errorf("image probe with standard budget: expected success, got %v", res.ErrorSummary)
	}
}
