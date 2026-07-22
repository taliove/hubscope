package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/scheduler"
	"github.com/taliove2009/hubscope/internal/server"
)

// globalOverview mirrors the global aggregate fields of the overview
// response (ticket 36).
type globalOverview struct {
	Availability24h  *float64 `json:"availability_24h"`
	EnabledEndpoints int      `json:"enabled_endpoints"`
}

// fetchGlobalOverview decodes GET /api/overview and returns only the global
// aggregate fields.
func fetchGlobalOverview(t *testing.T, baseURL string) globalOverview {
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
	var payload globalOverview
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("unmarshal overview: %v", err)
	}
	return payload
}

// TestOverviewGlobalAggregateWeighted verifies the global 24h availability is
// weighted by probe count — total successful probes over total probes across
// all enabled endpoints — not a simple average of per-endpoint rates. The
// numbers are chosen so the two calculations differ: endpoint e1 has 3
// probes with 2 successes (rate 2/3), endpoint e2 has 1 probe with 1 success
// (rate 1). Weighted = 3/4 = 0.75; simple average = (2/3 + 1)/2 ≈ 0.833.
func TestOverviewGlobalAggregateWeighted(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	ids := createModelEndpoints(t, ts.URL, stub.URL, "gpt-5")
	e1, e2 := int64(ids[0]), int64(ids[1])

	at := func(hoursAgo float64) time.Time {
		return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
	}
	// e1: 3 probes, 2 successful.
	seedProbe(t, db, e1, true, 100, nil, at(3))
	seedProbe(t, db, e1, false, 400, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, e1, true, 200, nil, at(1))
	// e2: 1 probe, successful.
	seedProbe(t, db, e2, true, 100, nil, at(1))

	payload := fetchGlobalOverview(t, ts.URL)

	if payload.EnabledEndpoints != 2 {
		t.Errorf("enabled endpoints: expected 2, got %d", payload.EnabledEndpoints)
	}
	if payload.Availability24h == nil || !approxEq(*payload.Availability24h, 0.75) {
		t.Errorf("global availability: expected probe-weighted 0.75, got %v", payload.Availability24h)
	}
}

// TestOverviewGlobalAggregateExcludesDisabled verifies disabled endpoints are
// excluded from both the enabled-endpoint count and the availability: their
// stale probe history must not dilute the global metric (same semantics as
// the per-group aggregates).
func TestOverviewGlobalAggregateExcludesDisabled(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	ids := createModelEndpoints(t, ts.URL, stub.URL, "qwen3-32b")
	e1, e2 := int64(ids[0]), int64(ids[1])

	at := func(hoursAgo float64) time.Time {
		return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
	}
	// e1 stays enabled: 2 successful probes.
	seedProbe(t, db, e1, true, 100, nil, at(2))
	seedProbe(t, db, e1, true, 100, nil, at(1))
	// e2 records a failure, then gets disabled: its history is excluded.
	seedProbe(t, db, e2, false, 500, strPtr("HTTP 503"), at(1))
	resp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, e2), map[string]interface{}{"enabled": false})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable e2: expected 200, got %d", resp.StatusCode)
	}

	payload := fetchGlobalOverview(t, ts.URL)

	if payload.EnabledEndpoints != 1 {
		t.Errorf("enabled endpoints: expected 1, got %d", payload.EnabledEndpoints)
	}
	if payload.Availability24h == nil || !approxEq(*payload.Availability24h, 1.0) {
		t.Errorf("global availability: expected 1.0 (disabled endpoint excluded), got %v", payload.Availability24h)
	}
}

// TestOverviewGlobalAggregateNoData verifies the empty-data behavior: with no
// endpoints at all the count is 0 and availability is null; with enabled
// endpoints that were never probed the count still reflects them but
// availability stays null — the API must not fabricate a number.
func TestOverviewGlobalAggregateNoData(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	// Empty database: no hubs, no models, no probes.
	payload := fetchGlobalOverview(t, ts.URL)
	if payload.EnabledEndpoints != 0 {
		t.Errorf("empty db enabled endpoints: expected 0, got %d", payload.EnabledEndpoints)
	}
	if payload.Availability24h != nil {
		t.Errorf("empty db availability: expected null, got %v", *payload.Availability24h)
	}

	// Enabled endpoints without any probe history.
	createModelEndpoints(t, ts.URL, stub.URL, "gpt-5")
	payload = fetchGlobalOverview(t, ts.URL)
	if payload.EnabledEndpoints != 2 {
		t.Errorf("unprobed enabled endpoints: expected 2, got %d", payload.EnabledEndpoints)
	}
	if payload.Availability24h != nil {
		t.Errorf("unprobed availability: expected null, got %v", *payload.Availability24h)
	}
}
