package server_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/scheduler"
	"github.com/taliove2009/hubscope/internal/server"
)

// findGroupedEntry returns the overview entry for one endpoint or fails.
func findGroupedEntry(t *testing.T, payload groupedOverview, endpointID int64) overviewEntry {
	t.Helper()
	for _, e := range payload.Endpoints {
		if e.EndpointID == endpointID {
			return e
		}
	}
	t.Fatalf("endpoint %d not present in overview (%d entries)", endpointID, len(payload.Endpoints))
	return overviewEntry{}
}

// expectReasons asserts the exact score reason list.
func expectReasons(t *testing.T, entry overviewEntry, want []string) {
	t.Helper()
	if len(entry.ScoreReasons) != len(want) {
		t.Fatalf("expected reasons %v, got %v", want, entry.ScoreReasons)
	}
	for i, w := range want {
		if entry.ScoreReasons[i] != w {
			t.Fatalf("reason %d: expected %q, got %q (all: %v)", i, w, entry.ScoreReasons[i], entry.ScoreReasons)
		}
	}
}

// dotAt returns the bucket starting at the given hour-aligned time.
func dotAt(t *testing.T, dots []overviewDot, bucketStart time.Time) overviewDot {
	t.Helper()
	want := bucketStart.UTC().Truncate(time.Hour).Format(time.RFC3339)
	for _, d := range dots {
		if d.BucketStart == want {
			return d
		}
	}
	t.Fatalf("bucket %q not found in dots (%d buckets)", want, len(dots))
	return overviewDot{}
}

// TestOverviewByProtocol verifies the protocol dimension of the group
// aggregates: every endpoint belongs to exactly one of the anthropic/openai
// groups, and the availability/latency math matches the seeded probes.
func TestOverviewByProtocol(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	// Each model yields one anthropic and one openai endpoint (in this order).
	alpha := createModelEndpoints(t, ts.URL, stub.URL, "proto-alpha")
	beta := createModelEndpoints(t, ts.URL, stub.URL, "proto-beta")
	e1, e2 := int64(alpha[0]), int64(alpha[1])
	e3 := int64(beta[0])

	at := func(hoursAgo float64) time.Time {
		return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
	}
	// anthropic: e1 healthy (3/3 ok), e3 failing (1/3 ok).
	seedProbe(t, db, e1, true, 100, nil, at(3))
	seedProbe(t, db, e1, true, 100, nil, at(2))
	seedProbe(t, db, e1, true, 100, nil, at(1))
	seedProbe(t, db, e3, true, 200, nil, at(3))
	seedProbe(t, db, e3, false, 500, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, e3, false, 500, strPtr("HTTP 503"), at(1))
	// openai: e2 degraded (2/4 ok), beta's openai endpoint never probed.
	seedProbe(t, db, e2, true, 1000, nil, at(4))
	seedProbe(t, db, e2, true, 1000, nil, at(3))
	seedProbe(t, db, e2, false, 1000, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, e2, false, 1000, strPtr("HTTP 503"), at(1))

	payload := fetchGroupedOverview(t, ts.URL)

	anthropic := findGroup(t, payload.ByProtocol, "anthropic")
	if anthropic.EndpointCount != 2 {
		t.Errorf("anthropic endpoints: expected 2, got %d", anthropic.EndpointCount)
	}
	if anthropic.StatusCounts["healthy"] != 1 || anthropic.StatusCounts["failing"] != 1 {
		t.Errorf("anthropic status counts: expected healthy=1 failing=1, got %v", anthropic.StatusCounts)
	}
	// 4 ok out of 6 probes; mean latency (300+1200)/6.
	if anthropic.Availability24h == nil || !approxEq(*anthropic.Availability24h, 4.0/6.0) {
		t.Errorf("anthropic availability: expected 0.667, got %v", anthropic.Availability24h)
	}
	if anthropic.AvgLatencyMs == nil || !approxEq(*anthropic.AvgLatencyMs, 250) {
		t.Errorf("anthropic avg latency: expected 250, got %v", anthropic.AvgLatencyMs)
	}

	openai := findGroup(t, payload.ByProtocol, "openai")
	if openai.EndpointCount != 2 {
		t.Errorf("openai endpoints: expected 2, got %d", openai.EndpointCount)
	}
	if openai.StatusCounts["failing"] != 1 || openai.StatusCounts["healthy"] != 1 {
		t.Errorf("openai status counts: expected failing=1 healthy=1, got %v", openai.StatusCounts)
	}
	if openai.Availability24h == nil || !approxEq(*openai.Availability24h, 0.5) {
		t.Errorf("openai availability: expected 0.5, got %v", openai.Availability24h)
	}
}

// TestOverviewDots24h verifies the hourly bucketing of the 24h probe
// samples: 24 hour-aligned buckets ending at the current hour, probes land
// in their hour bucket, failures are counted, empty buckets stay present.
func TestOverviewDots24h(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 34, 56, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "dots-model")
	ep := int64(ids[0])

	// The current hour is 12:00; the 24 buckets span 13:00 (prev day)..12:00.
	currentHour := now.Truncate(time.Hour)
	// Two probes inside one hour (one failure), one probe three hours earlier.
	seedProbe(t, db, ep, true, 100, nil, currentHour.Add(-2*time.Hour).Add(5*time.Minute))
	seedProbe(t, db, ep, false, 100, strPtr("HTTP 503"), currentHour.Add(-2*time.Hour).Add(40*time.Minute))
	seedProbe(t, db, ep, true, 100, nil, currentHour.Add(-5*time.Hour).Add(10*time.Minute))

	entry := findGroupedEntry(t, fetchGroupedOverview(t, ts.URL), ep)

	if len(entry.Dots24h) != 24 {
		t.Fatalf("expected 24 dots, got %d", len(entry.Dots24h))
	}
	// Buckets end at the current hour and are hour-aligned RFC3339.
	last := entry.Dots24h[23]
	if last.BucketStart != currentHour.Format(time.RFC3339) {
		t.Errorf("last bucket: expected %q, got %q", currentHour.Format(time.RFC3339), last.BucketStart)
	}
	firstWant := currentHour.Add(-23 * time.Hour)
	if entry.Dots24h[0].BucketStart != firstWant.Format(time.RFC3339) {
		t.Errorf("first bucket: expected %q, got %q", firstWant.Format(time.RFC3339), entry.Dots24h[0].BucketStart)
	}

	d := dotAt(t, entry.Dots24h, currentHour.Add(-2*time.Hour))
	if d.Total != 2 || d.Failures != 1 {
		t.Errorf("bucket -2h: expected total=2 failures=1, got %+v", d)
	}
	d = dotAt(t, entry.Dots24h, currentHour.Add(-5*time.Hour))
	if d.Total != 1 || d.Failures != 0 {
		t.Errorf("bucket -5h: expected total=1 failures=0, got %+v", d)
	}
	// Empty hours stay present with zero counts.
	d = dotAt(t, entry.Dots24h, currentHour.Add(-1*time.Hour))
	if d.Total != 0 || d.Failures != 0 {
		t.Errorf("bucket -1h: expected empty bucket, got %+v", d)
	}
}

// TestOverviewScore verifies the deterministic scoring rules through the
// API: a perfect record scores 100 with no reasons, failures and low success
// rates deduct and cap, latency degradation deducts 15 and caps at 80, and
// an unprobed endpoint reports a null score with empty reasons.
func TestOverviewScore(t *testing.T) {
	db := openTempDB(t)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(now)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(clock.Now)))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	alpha := createModelEndpoints(t, ts.URL, stub.URL, "score-alpha")
	beta := createModelEndpoints(t, ts.URL, stub.URL, "score-beta")
	gamma := createModelEndpoints(t, ts.URL, stub.URL, "score-gamma")
	healthy, failing := int64(alpha[0]), int64(alpha[1])
	down, noData := int64(beta[0]), int64(beta[1])
	slow := int64(gamma[0])

	at := func(hoursAgo float64) time.Time {
		return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
	}

	// healthy: three successes -> 100, no reasons.
	seedProbe(t, db, healthy, true, 100, nil, at(3))
	seedProbe(t, db, healthy, true, 100, nil, at(2))
	seedProbe(t, db, healthy, true, 100, nil, at(1))

	// failing: 2 successes then 2 consecutive failures. Rate 0.5 deducts
	// round((0.95-0.5)*100)=45 -> 55, capped at 50 by the failure streak.
	seedProbe(t, db, failing, true, 100, nil, at(4))
	seedProbe(t, db, failing, true, 100, nil, at(3))
	seedProbe(t, db, failing, false, 100, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, failing, false, 100, strPtr("HTTP 503"), at(1))

	// down: 3 consecutive failures. Rate 0 deducts 95 -> 5, under the cap 20.
	seedProbe(t, db, down, false, 100, strPtr("HTTP 503"), at(3))
	seedProbe(t, db, down, false, 100, strPtr("HTTP 503"), at(2))
	seedProbe(t, db, down, false, 100, strPtr("HTTP 503"), at(1))

	// noData: never probed -> null score, empty reasons.

	// slow: 7-day baseline P50 of 1s, 24h P95 of 3s -> -15, capped at 80.
	for i := 0; i < 20; i++ {
		seedProbe(t, db, slow, true, 1000, nil, now.Add(-time.Duration(24*(i%5+1))*time.Hour))
	}
	for i := 1; i <= 5; i++ {
		seedProbe(t, db, slow, true, 3000, nil, now.Add(-time.Duration(i)*time.Minute))
	}

	payload := fetchGroupedOverview(t, ts.URL)

	entry := findGroupedEntry(t, payload, healthy)
	if entry.Score == nil || *entry.Score != 100 {
		t.Errorf("healthy score: expected 100, got %v", entry.Score)
	}
	expectReasons(t, entry, []string{})

	entry = findGroupedEntry(t, payload, failing)
	if entry.Score == nil || *entry.Score != 50 {
		t.Errorf("failing score: expected 50, got %v", entry.Score)
	}
	expectReasons(t, entry, []string{
		"24h 成功率 50.0%,扣 45 分",
		"连续 2 次失败,封顶 50 分",
	})

	entry = findGroupedEntry(t, payload, down)
	if entry.Score == nil || *entry.Score != 5 {
		t.Errorf("down score: expected 5, got %v", entry.Score)
	}
	expectReasons(t, entry, []string{
		"24h 成功率 0.0%,扣 95 分",
		"连续 3 次失败,封顶 20 分",
	})

	entry = findGroupedEntry(t, payload, noData)
	if entry.Score != nil {
		t.Errorf("no-data score: expected null, got %v", *entry.Score)
	}
	expectReasons(t, entry, []string{})

	entry = findGroupedEntry(t, payload, slow)
	if entry.Score == nil || *entry.Score != 80 {
		t.Errorf("slow score: expected 80, got %v", entry.Score)
	}
	expectReasons(t, entry, []string{
		"P95 延迟 3000 ms 超基线 2 倍,扣 15 分",
		"性能降级,封顶 80 分",
	})
}
