package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/scheduler"
)

// TestPatchEndpoint covers the PATCH /api/endpoints/{id} contract: interval
// override set/clear, validation, enable toggling, and 404 handling.
func TestPatchEndpoint(t *testing.T) {
	db := openTempDB(t)
	stub := newDelayStubHub(t, 0)
	ts := newTestAPIServer(t, db)

	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "patch-model")
	id := endpointIDs[0]

	t.Run("interval_defaults_to_null", func(t *testing.T) {
		resp := doGet(t, ts.URL+"/api/models")
		defer resp.Body.Close()
		var env envelope
		json.NewDecoder(resp.Body).Decode(&env)
		var models []map[string]interface{}
		json.Unmarshal(env.Data, &models)
		ep := models[0]["endpoints"].([]interface{})[0].(map[string]interface{})
		v, present := ep["interval_seconds"]
		if !present {
			t.Fatal("interval_seconds field must be present")
		}
		if v != nil {
			t.Fatalf("interval_seconds should default to null, got %v", v)
		}
	})

	t.Run("set_interval_override", func(t *testing.T) {
		resp := patchEndpoint(t, ts.URL, id, map[string]interface{}{"interval_seconds": 900})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		json.NewDecoder(resp.Body).Decode(&env)
		var ep map[string]interface{}
		json.Unmarshal(env.Data, &ep)
		if ep["interval_seconds"].(float64) != 900 {
			t.Fatalf("expected interval_seconds 900, got %v", ep["interval_seconds"])
		}
	})

	t.Run("interval_below_minimum_rejected", func(t *testing.T) {
		resp := patchEndpoint(t, ts.URL, id, map[string]interface{}{"interval_seconds": 30})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("clear_interval_override", func(t *testing.T) {
		resp := patchEndpoint(t, ts.URL, id, map[string]interface{}{"interval_seconds": nil})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		json.NewDecoder(resp.Body).Decode(&env)
		var ep map[string]interface{}
		json.Unmarshal(env.Data, &ep)
		if ep["interval_seconds"] != nil {
			t.Fatalf("expected interval_seconds null after clear, got %v", ep["interval_seconds"])
		}
	})

	t.Run("toggle_enabled", func(t *testing.T) {
		resp := patchEndpoint(t, ts.URL, id, map[string]interface{}{"enabled": false})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		var ep map[string]interface{}
		json.Unmarshal(env.Data, &ep)
		if ep["enabled"].(bool) != false {
			t.Fatal("expected enabled false")
		}

		resp = patchEndpoint(t, ts.URL, id, map[string]interface{}{"enabled": true})
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("unknown_endpoint_returns_404", func(t *testing.T) {
		resp := patchEndpoint(t, ts.URL, 999999, map[string]interface{}{"enabled": false})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

// TestSchedulerConcurrencyLimit verifies that ten enabled endpoints probed
// concurrently never exceed the global cap of eight in-flight rounds.
func TestSchedulerConcurrencyLimit(t *testing.T) {
	db := openTempDB(t)
	stub := newDelayStubHub(t, 100*time.Millisecond)
	ts := newTestAPIServer(t, db)

	// Five models yield ten enabled endpoints. Manual creation trial-probes
	// each protocol (2 requests per model), so the round assertions count
	// from this baseline.
	endpointIDs := []int{}
	for i := 0; i < 5; i++ {
		endpointIDs = append(endpointIDs, createModelEndpoints(t, ts.URL, stub.URL, fmt.Sprintf("conc-model-%d", i))...)
	}
	baseline := stub.totalRequests()

	clock := scheduler.NewFakeClock(time.Now())
	startScheduler(t, db, hubclient.New(), clock)

	// Every endpoint runs exactly one round (2 requests) at startup.
	waitFor(t, "20 stub requests past baseline", func() bool {
		return stub.totalRequests() >= baseline+20
	})
	for _, id := range endpointIDs {
		waitForProbeCount(t, ts.URL, id, 2)
	}

	if peak := stub.peakInFlight(); peak > 8 {
		t.Fatalf("concurrency cap exceeded: peak in-flight requests = %d, want <= 8", peak)
	}

	// No clock advance means no further rounds.
	assertProbeCountStable(t, ts.URL, endpointIDs[0], 2, 300*time.Millisecond)
	if total := stub.totalRequests(); total != baseline+20 {
		t.Fatalf("expected exactly %d stub requests, got %d", baseline+20, total)
	}
}

// TestSchedulerProbeTimeout verifies that a hub slower than the client
// timeout produces failed records with a recognizable timeout summary.
func TestSchedulerProbeTimeout(t *testing.T) {
	db := openTempDB(t)
	stub := newDelayStubHub(t, time.Second)
	ts := newTestAPIServer(t, db)

	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "timeout-model")
	target, other := endpointIDs[0], endpointIDs[1]

	// Disable the sibling so the test finishes faster.
	resp := patchEndpoint(t, ts.URL, other, map[string]interface{}{"enabled": false})
	resp.Body.Close()

	clock := scheduler.NewFakeClock(time.Now())
	// A 100ms client timeout against a 1s stub forces every request to time out.
	startScheduler(t, db, hubclient.NewWithTimeout(100*time.Millisecond), clock)

	waitForProbeCount(t, ts.URL, target, 2)

	for _, p := range probeRecords(t, ts.URL, target) {
		if p["ok"].(bool) != false {
			t.Error("timed-out probe must not be ok")
		}
		if p["http_status"].(float64) != 0 {
			t.Errorf("timeout should record http_status 0, got %v", p["http_status"])
		}
		summary, _ := p["error_summary"].(string)
		if !strings.Contains(strings.ToLower(summary), "timeout") {
			t.Errorf("error_summary should mention timeout, got %q", summary)
		}
	}
}

// TestPatchEndpointEmptyBody verifies that a PATCH without a body is a no-op
// returning the current endpoint rather than a 400.
func TestPatchEndpointEmptyBody(t *testing.T) {
	db := openTempDB(t)
	stub := newDelayStubHub(t, 0)
	ts := newTestAPIServer(t, db)

	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "empty-patch-model")

	resp := patchEndpoint(t, ts.URL, endpointIDs[0], nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("empty PATCH: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var ep map[string]interface{}
	if err := json.Unmarshal(env.Data, &ep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ep["enabled"].(bool) != true {
		t.Error("enabled should stay true after empty PATCH")
	}
	if ep["interval_seconds"] != nil {
		t.Errorf("interval_seconds should stay null, got %v", ep["interval_seconds"])
	}
}
