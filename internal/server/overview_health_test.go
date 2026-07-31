package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
)

// healthOverview mirrors the global aggregate fields of the overview
// response relevant to the health index (spec 0018 decision 7, GH #111).
type healthOverview struct {
	Availability24h    *float64 `json:"availability_24h"`
	EnabledEndpoints   int      `json:"enabled_endpoints"`
	HealthScore24h     *float64 `json:"health_score_24h"`
	HealthScorePrev24h *float64 `json:"health_score_prev_24h"`
	HealthScoreDelta   *float64 `json:"health_score_delta"`
	Probes24h          int      `json:"probes_24h"`
}

// fetchHealthOverview decodes GET /api/overview and returns only the health
// index fields.
func fetchHealthOverview(t *testing.T, baseURL string) healthOverview {
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
	var payload healthOverview
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("unmarshal overview: %v", err)
	}
	return payload
}

// TestOverviewHealthScoreTwoWindows verifies the health index across both
// 24h windows: health_score_24h is the availability_24h aggregate itself
// (one definition, one computation), health_score_prev_24h applies the same
// probe weighting to the [now-48h, now-24h) window, and the delta is their
// difference. A probe at exactly now-24h belongs to the CURRENT window
// (inclusive lower bound), never to the previous one (exclusive upper
// bound) — the two windows must not double-count the boundary sample.
func TestOverviewHealthScoreTwoWindows(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, server.WithNow(clock.Now), server.WithSyncDiscovery()))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	ids := createModelEndpoints(t, ts.URL, stub.URL, "gpt-5")
	e1, e2 := int64(ids[0]), int64(ids[1])

	at := func(hoursAgo float64) time.Time {
		return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
	}
	// Current window — e1: 3 probes, 2 successful.
	seedProbe(t, db, e1, true, 100, nil, at(3))
	seedProbe(t, db, e1, false, 400, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, e1, true, 200, nil, at(1))
	// Current window — e2: 2 successful probes, one exactly at the window
	// boundary (now-24h counts as current).
	seedProbe(t, db, e2, true, 100, nil, at(1))
	seedProbe(t, db, e2, true, 100, nil, now.Add(-24*time.Hour))
	// Previous window [now-48h, now-24h) — e1: 2 probes, 1 successful.
	seedProbe(t, db, e1, true, 100, nil, at(30))
	seedProbe(t, db, e1, false, 500, strPtr("HTTP 503"), at(25))
	// Previous window — e2: 1 successful probe.
	seedProbe(t, db, e2, true, 100, nil, at(40))

	payload := fetchHealthOverview(t, ts.URL)

	// Current: (2 + 2) ok over (3 + 2) probes = 0.8; previous: (1 + 1) over
	// (2 + 1) = 2/3.
	if payload.Availability24h == nil || !approxEq(*payload.Availability24h, 0.8) {
		t.Errorf("availability_24h: expected 0.8, got %v", payload.Availability24h)
	}
	if payload.HealthScore24h == nil || !approxEq(*payload.HealthScore24h, 0.8) {
		t.Errorf("health_score_24h: expected 0.8 (same as availability_24h), got %v", payload.HealthScore24h)
	}
	if payload.HealthScorePrev24h == nil || !approxEq(*payload.HealthScorePrev24h, 2.0/3.0) {
		t.Errorf("health_score_prev_24h: expected 2/3, got %v", payload.HealthScorePrev24h)
	}
	if payload.HealthScoreDelta == nil || !approxEq(*payload.HealthScoreDelta, 0.8-2.0/3.0) {
		t.Errorf("health_score_delta: expected ~0.1333, got %v", payload.HealthScoreDelta)
	}
	if payload.Probes24h != 5 {
		t.Errorf("probes_24h: expected 5, got %d", payload.Probes24h)
	}
}

// TestOverviewHealthScorePrevWindowEmpty verifies the null semantics when
// only the current window has data: the score is present while the previous
// window and the delta stay null — the API must not fabricate a baseline.
// It also pins the fully-empty behavior first: with no probes at all every
// health field is null and probes_24h is 0 (never a fabricated 100%).
func TestOverviewHealthScorePrevWindowEmpty(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, server.WithNow(clock.Now), server.WithSyncDiscovery()))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	ids := createModelEndpoints(t, ts.URL, stub.URL, "qwen3-32b")
	e1 := int64(ids[0])

	// No probes at all: nothing to report.
	payload := fetchHealthOverview(t, ts.URL)
	if payload.HealthScore24h != nil {
		t.Errorf("empty health_score_24h: expected null, got %v", *payload.HealthScore24h)
	}
	if payload.HealthScorePrev24h != nil {
		t.Errorf("empty health_score_prev_24h: expected null, got %v", *payload.HealthScorePrev24h)
	}
	if payload.HealthScoreDelta != nil {
		t.Errorf("empty health_score_delta: expected null, got %v", *payload.HealthScoreDelta)
	}
	if payload.Probes24h != 0 {
		t.Errorf("empty probes_24h: expected 0, got %d", payload.Probes24h)
	}

	// Current window only: 2 successful probes.
	seedProbe(t, db, e1, true, 100, nil, now.Add(-2*time.Hour))
	seedProbe(t, db, e1, true, 100, nil, now.Add(-1*time.Hour))

	payload = fetchHealthOverview(t, ts.URL)
	if payload.HealthScore24h == nil || !approxEq(*payload.HealthScore24h, 1.0) {
		t.Errorf("health_score_24h: expected 1.0, got %v", payload.HealthScore24h)
	}
	if payload.HealthScorePrev24h != nil {
		t.Errorf("health_score_prev_24h: expected null (previous window empty), got %v", *payload.HealthScorePrev24h)
	}
	if payload.HealthScoreDelta != nil {
		t.Errorf("health_score_delta: expected null (previous window empty), got %v", *payload.HealthScoreDelta)
	}
	if payload.Probes24h != 2 {
		t.Errorf("probes_24h: expected 2, got %d", payload.Probes24h)
	}
}

// TestOverviewHealthScoreCurrentWindowEmpty verifies the mirror case: data
// only in the previous window leaves the current score and the delta null —
// an empty current window must never read as 100% healthy — while the
// previous window still reports its value.
func TestOverviewHealthScoreCurrentWindowEmpty(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, server.WithNow(clock.Now), server.WithSyncDiscovery()))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	ids := createModelEndpoints(t, ts.URL, stub.URL, "gpt-5")
	e1, e2 := int64(ids[0]), int64(ids[1])

	// Previous window only: e1 one success, e2 one failure → 0.5.
	seedProbe(t, db, e1, true, 100, nil, now.Add(-30*time.Hour))
	seedProbe(t, db, e2, false, 500, strPtr("HTTP 503"), now.Add(-26*time.Hour))

	payload := fetchHealthOverview(t, ts.URL)
	if payload.HealthScore24h != nil {
		t.Errorf("health_score_24h: expected null (current window empty), got %v", *payload.HealthScore24h)
	}
	if payload.Availability24h != nil {
		t.Errorf("availability_24h: expected null (existing no-data behavior), got %v", *payload.Availability24h)
	}
	if payload.HealthScorePrev24h == nil || !approxEq(*payload.HealthScorePrev24h, 0.5) {
		t.Errorf("health_score_prev_24h: expected 0.5, got %v", payload.HealthScorePrev24h)
	}
	if payload.HealthScoreDelta != nil {
		t.Errorf("health_score_delta: expected null (current window empty), got %v", *payload.HealthScoreDelta)
	}
	if payload.Probes24h != 0 {
		t.Errorf("probes_24h: expected 0, got %d", payload.Probes24h)
	}
}

// TestOverviewHealthScoreExcludesDisabled verifies disabled endpoints are
// excluded from BOTH windows: their stale history must not drag the health
// index, same as the existing availability semantics. With the disabled
// endpoint's failures excluded both windows are fully healthy, so the delta
// is a non-null 0 — distinct from the null delta of a missing window.
func TestOverviewHealthScoreExcludesDisabled(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, server.WithNow(clock.Now), server.WithSyncDiscovery()))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	ids := createModelEndpoints(t, ts.URL, stub.URL, "qwen3-32b")
	e1, e2 := int64(ids[0]), int64(ids[1])

	// e1 stays enabled: successful probes in both windows.
	seedProbe(t, db, e1, true, 100, nil, now.Add(-30*time.Hour))
	seedProbe(t, db, e1, true, 100, nil, now.Add(-2*time.Hour))
	seedProbe(t, db, e1, true, 100, nil, now.Add(-1*time.Hour))
	// e2 records failures in both windows, then gets disabled.
	seedProbe(t, db, e2, false, 500, strPtr("HTTP 503"), now.Add(-30*time.Hour))
	seedProbe(t, db, e2, false, 500, strPtr("HTTP 503"), now.Add(-1*time.Hour))
	resp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, e2), map[string]interface{}{"enabled": false})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable e2: expected 200, got %d", resp.StatusCode)
	}

	payload := fetchHealthOverview(t, ts.URL)
	if payload.EnabledEndpoints != 1 {
		t.Errorf("enabled endpoints: expected 1, got %d", payload.EnabledEndpoints)
	}
	if payload.HealthScore24h == nil || !approxEq(*payload.HealthScore24h, 1.0) {
		t.Errorf("health_score_24h: expected 1.0 (disabled endpoint excluded), got %v", payload.HealthScore24h)
	}
	if payload.HealthScorePrev24h == nil || !approxEq(*payload.HealthScorePrev24h, 1.0) {
		t.Errorf("health_score_prev_24h: expected 1.0 (disabled endpoint excluded), got %v", payload.HealthScorePrev24h)
	}
	if payload.HealthScoreDelta == nil || !approxEq(*payload.HealthScoreDelta, 0.0) {
		t.Errorf("health_score_delta: expected non-null 0, got %v", payload.HealthScoreDelta)
	}
	if payload.Probes24h != 2 {
		t.Errorf("probes_24h: expected 2 (disabled endpoint excluded), got %d", payload.Probes24h)
	}
}
