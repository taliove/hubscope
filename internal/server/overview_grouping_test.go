package server_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
)

// groupPayload mirrors one group aggregate of the overview response.
type groupPayload struct {
	Key             string         `json:"key"`
	EndpointCount   int            `json:"endpoint_count"`
	StatusCounts    map[string]int `json:"status_counts"`
	Availability24h *float64       `json:"availability_24h"`
	AvgLatencyMs    *float64       `json:"avg_latency_ms"`
}

// groupedOverview mirrors the overview response with group aggregates.
type groupedOverview struct {
	GeneratedAt  string          `json:"generated_at"`
	Endpoints    []overviewEntry `json:"endpoints"`
	ByFamily     []groupPayload  `json:"by_family"`
	ByCapability []groupPayload  `json:"by_capability"`
	ByProtocol   []groupPayload  `json:"by_protocol"`
}

// fetchGroupedOverview decodes GET /api/overview including group aggregates.
func fetchGroupedOverview(t *testing.T, baseURL string) groupedOverview {
	t.Helper()
	resp := doGet(t, baseURL+"/api/overview")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	var payload groupedOverview
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("unmarshal overview: %v", err)
	}
	return payload
}

// findGroup locates a group by key.
func findGroup(t *testing.T, groups []groupPayload, key string) groupPayload {
	t.Helper()
	for _, g := range groups {
		if g.Key == key {
			return g
		}
	}
	t.Fatalf("group %q not found in %v", key, groups)
	return groupPayload{}
}

// approxEq compares floats with a small tolerance.
func approxEq(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

// TestOverviewGrouping verifies the per-group health aggregates: status
// distribution (with disabled bucketed separately), probe-weighted 24h
// availability, and mean 24h latency, along both grouping dimensions.
func TestOverviewGrouping(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	// Three models across two families and two capabilities; each yields an
	// anthropic and an openai endpoint.
	gpt5 := createModelEndpoints(t, ts.URL, stub.URL, "gpt-5")
	gptImg := createModelEndpoints(t, ts.URL, stub.URL, "gpt-image-2")
	qwen := createModelEndpoints(t, ts.URL, stub.URL, "qwen3-32b")
	e1, e2 := int64(gpt5[0]), int64(gpt5[1])
	e3 := int64(gptImg[0])
	e5, e6 := int64(qwen[0]), int64(qwen[1])

	at := func(hoursAgo float64) time.Time {
		return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
	}
	// e1: degraded (24h rate 0.75 < 0.95), latest probe is a success.
	seedProbe(t, db, e1, true, 100, nil, at(4))
	seedProbe(t, db, e1, false, 400, strPtr("HTTP 503"), at(3))
	seedProbe(t, db, e1, true, 200, nil, at(2))
	seedProbe(t, db, e1, true, 300, nil, at(1))
	// e2: healthy, one success.
	seedProbe(t, db, e2, true, 100, nil, at(1))
	// e5: failing (latest two probes failed, below the down threshold).
	seedProbe(t, db, e5, true, 1000, nil, at(4))
	seedProbe(t, db, e5, true, 1000, nil, at(3))
	seedProbe(t, db, e5, false, 2000, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, e5, false, 2000, strPtr("HTTP 503"), at(1))
	// e6: disabled after a failure — its samples are excluded from the group
	// metrics (a disabled endpoint is not monitored).
	seedProbe(t, db, e6, false, 5000, strPtr("HTTP 503"), at(1))
	resp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, e6), map[string]interface{}{"enabled": false})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable e6: expected 200, got %d", resp.StatusCode)
	}

	payload := fetchGroupedOverview(t, ts.URL)

	// Entries carry both classification dimensions.
	byID := map[int64]overviewEntry{}
	for _, e := range payload.Endpoints {
		byID[e.EndpointID] = e
	}
	if fam := byID[e1].Family; fam != "gpt" {
		t.Errorf("e1 family: expected gpt, got %q", fam)
	}
	if cap := byID[e5].Capability; cap != "chat" {
		t.Errorf("e5 capability: expected chat, got %q", cap)
	}
	if cap := byID[e3].Capability; cap != "image" {
		t.Errorf("e3 capability: expected image, got %q", cap)
	}

	// Family groups.
	gpt := findGroup(t, payload.ByFamily, "gpt")
	if gpt.EndpointCount != 4 {
		t.Errorf("gpt endpoints: expected 4, got %d", gpt.EndpointCount)
	}
	if gpt.StatusCounts["degraded"] != 1 || gpt.StatusCounts["healthy"] != 3 {
		t.Errorf("gpt status counts: expected degraded=1 healthy=3, got %v", gpt.StatusCounts)
	}
	if gpt.Availability24h == nil || !approxEq(*gpt.Availability24h, 0.8) {
		t.Errorf("gpt availability: expected 0.8, got %v", gpt.Availability24h)
	}
	if gpt.AvgLatencyMs == nil || !approxEq(*gpt.AvgLatencyMs, 220) {
		t.Errorf("gpt avg latency: expected 220, got %v", gpt.AvgLatencyMs)
	}

	qwenG := findGroup(t, payload.ByFamily, "qwen")
	if qwenG.StatusCounts["failing"] != 1 || qwenG.StatusCounts["disabled"] != 1 {
		t.Errorf("qwen status counts: expected failing=1 disabled=1, got %v", qwenG.StatusCounts)
	}
	// 0.5 from e5 alone; e6's failed probe must not dilute the metric.
	if qwenG.Availability24h == nil || !approxEq(*qwenG.Availability24h, 0.5) {
		t.Errorf("qwen availability: expected 0.5, got %v", qwenG.Availability24h)
	}

	// Capability groups.
	chat := findGroup(t, payload.ByCapability, "chat")
	if chat.EndpointCount != 4 {
		t.Errorf("chat endpoints: expected 4, got %d", chat.EndpointCount)
	}
	want := map[string]int{"degraded": 1, "healthy": 1, "failing": 1, "disabled": 1}
	for k, v := range want {
		if chat.StatusCounts[k] != v {
			t.Errorf("chat status counts: expected %v, got %v", want, chat.StatusCounts)
			break
		}
	}
	if chat.Availability24h == nil || !approxEq(*chat.Availability24h, 6.0/9.0) {
		t.Errorf("chat availability: expected 0.667, got %v", chat.Availability24h)
	}

	image := findGroup(t, payload.ByCapability, "image")
	if image.EndpointCount != 2 || image.StatusCounts["healthy"] != 2 {
		t.Errorf("image group: expected 2 healthy endpoints, got %+v", image)
	}
	if image.Availability24h != nil || image.AvgLatencyMs != nil {
		t.Errorf("image group has no 24h data: expected null metrics, got %+v", image)
	}
}

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }
