package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// overviewPayload mirrors the GET /api/overview response for assertions.
type overviewPayload struct {
	GeneratedAt string          `json:"generated_at"`
	Endpoints   []overviewEntry `json:"endpoints"`
}

// overviewEntry mirrors one entry of the overview response.
type overviewEntry struct {
	EndpointID     int64         `json:"endpoint_id"`
	ModelID        string        `json:"model_id"`
	Protocol       string        `json:"protocol"`
	Enabled        bool          `json:"enabled"`
	Status         string        `json:"status"`
	StatusReason   string        `json:"status_reason"`
	SuccessRate24h *float64      `json:"success_rate_24h"`
	P50Ms          *float64      `json:"p50_ms"`
	P95Ms          *float64      `json:"p95_ms"`
	LastProbeAt    *string       `json:"last_probe_at"`
	Family         string        `json:"family"`
	Capability     string        `json:"capability"`
	Score          *int          `json:"score"`
	ScoreReasons   []string      `json:"score_reasons"`
	Dots24h        []overviewDot `json:"dots_24h"`
}

// overviewDot mirrors one hourly bucket of the dots_24h array.
type overviewDot struct {
	BucketStart string `json:"bucket_start"`
	Total       int    `json:"total"`
	Failures    int    `json:"failures"`
}

// switchStubHub is a stub Hub whose success/failure behavior can be flipped
// mid-test to drive status transitions through real probing.
type switchStubHub struct {
	*httptest.Server
	failing atomic.Bool
}

func newSwitchStubHub(t *testing.T) *switchStubHub {
	t.Helper()
	stub := &switchStubHub{}
	stub.Server = httptest.NewServer(http.HandlerFunc(stub.handle))
	t.Cleanup(stub.Close)
	return stub
}

func (s *switchStubHub) handle(w http.ResponseWriter, r *http.Request) {
	isAnthropic := strings.HasSuffix(r.URL.Path, "/v1/messages")
	if s.failing.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"message":"No available providers"}}`)
		return
	}
	var req struct {
		Stream bool `json:"stream"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &req)
	if req.Stream {
		writeStubStream(w, isAnthropic)
		return
	}
	writeStubNonStream(w, isAnthropic)
}

// seedProbe inserts a probe record directly into the store at a controlled
// timestamp. Used to build fine-grained histories that the two-records-per-
// round probe API cannot produce.
func seedProbe(t *testing.T, db *store.DB, endpointID int64, ok bool, latencyMs int, errSummary *string, at time.Time) {
	t.Helper()
	httpStatus := 200
	if !ok {
		httpStatus = 503
	}
	_, err := db.CreateProbe(store.Probe{
		EndpointID:   endpointID,
		OK:           ok,
		HTTPStatus:   httpStatus,
		ErrorSummary: errSummary,
		LatencyMs:    latencyMs,
		CreatedAt:    at,
	})
	if err != nil {
		t.Fatalf("seed probe: %v", err)
	}
}

// fetchOverview GETs /api/overview and decodes the payload.
func fetchOverview(t *testing.T, baseURL string) overviewPayload {
	t.Helper()
	resp := doGet(t, baseURL+"/api/overview")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/overview: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	var payload overviewPayload
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("unmarshal overview: %v", err)
	}
	return payload
}

// findEntry returns the overview entry for one endpoint or fails the test.
func findEntry(t *testing.T, payload overviewPayload, endpointID int64) overviewEntry {
	t.Helper()
	for _, e := range payload.Endpoints {
		if e.EndpointID == endpointID {
			return e
		}
	}
	t.Fatalf("endpoint %d not present in overview (%d entries)", endpointID, len(payload.Endpoints))
	return overviewEntry{}
}

// expectStatus asserts both the status and the exact reason of an entry.
func expectStatus(t *testing.T, entry overviewEntry, wantStatus, wantReason string) {
	t.Helper()
	if entry.Status != wantStatus {
		t.Fatalf("expected status %q, got %q (reason: %s)", wantStatus, entry.Status, entry.StatusReason)
	}
	if entry.StatusReason != wantReason {
		t.Fatalf("expected reason %q, got %q", wantReason, entry.StatusReason)
	}
}

// TestOverviewStatusTransitions drives the full state machine with seeded
// probe history against a server running on a controllable clock, asserting
// the healthy -> failing -> down -> degraded -> recovered transitions and
// their exact reason texts through the API.
func TestOverviewStatusTransitions(t *testing.T) {
	db := openTempDB(t)

	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-transition")
	ep := int64(ids[0])
	failErr := "HTTP 503: No available providers"

	entry := findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "healthy", "暂无探测数据")
	if entry.SuccessRate24h != nil || entry.P50Ms != nil || entry.P95Ms != nil || entry.LastProbeAt != nil {
		t.Fatalf("never-probed endpoint must report null stats, got %+v", entry)
	}

	// Three successes: healthy with a perfect 24h record.
	for i := 3; i >= 1; i-- {
		seedProbe(t, db, ep, true, 100, nil, fakeNow.Add(-time.Duration(i)*time.Minute-time.Hour))
	}
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "healthy", "运行正常")
	if entry.SuccessRate24h == nil || *entry.SuccessRate24h != 1.0 {
		t.Fatalf("expected 24h success rate 1.0, got %v", entry.SuccessRate24h)
	}

	// First failure: failing, below the down threshold.
	seedProbe(t, db, ep, false, 120, &failErr, fakeNow.Add(-50*time.Minute))
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "failing", "连续 1 次失败,最近错误: HTTP 503: No available providers")

	// Second failure: still failing.
	seedProbe(t, db, ep, false, 120, &failErr, fakeNow.Add(-49*time.Minute))
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "failing", "连续 2 次失败,最近错误: HTTP 503: No available providers")

	// Third consecutive failure: down.
	seedProbe(t, db, ep, false, 120, &failErr, fakeNow.Add(-48*time.Minute))
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "down", "连续 3 次失败,最近错误: HTTP 503: No available providers")

	// One success breaks the streak, but the 24h success rate (4/7) is too
	// low, so the endpoint lands in degraded.
	seedProbe(t, db, ep, true, 100, nil, fakeNow.Add(-47*time.Minute))
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "degraded", "24h 成功率 57.1% 低于 95%")

	// Advancing the clock past the 24h window expires the bad record; with
	// no failures in a row the endpoint recovers to healthy.
	fakeNow = fakeNow.Add(25 * time.Hour)
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "healthy", "运行正常")
	if entry.SuccessRate24h != nil || entry.P50Ms != nil || entry.P95Ms != nil {
		t.Fatalf("empty 24h window must report null stats, got %+v", entry)
	}
	if entry.LastProbeAt == nil {
		t.Fatal("last_probe_at must survive an empty 24h window")
	}
}

// TestOverviewTransitionsViaRealProbes drives status transitions through the
// real probe path: a stub Hub flips between success and failure while the
// test triggers probe rounds via the API.
func TestOverviewTransitionsViaRealProbes(t *testing.T) {
	db := openTempDB(t)
	stub := newSwitchStubHub(t)
	ts := newTestAPIServer(t, db)

	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-real")
	ep := int64(ids[0])

	// Seed enough successes that the endpoint can recover to healthy later:
	// after 4 failures and 2 more successes the 24h rate must stay >= 0.95.
	past := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 80; i++ {
		seedProbe(t, db, ep, true, 100, nil, past)
	}

	entry := findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "healthy", "运行正常")
	if entry.P50Ms == nil || *entry.P50Ms != 100 || entry.P95Ms == nil || *entry.P95Ms != 100 {
		t.Fatalf("expected p50/p95 of 100ms, got %+v", entry)
	}

	probeRound := func() {
		t.Helper()
		resp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, ep), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("probe round: expected 200, got %d", resp.StatusCode)
		}
	}

	// One failing round yields two failed records: failing, not yet down.
	stub.failing.Store(true)
	probeRound()
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "failing", "连续 2 次失败,最近错误: HTTP 503: No available providers")

	// A second failing round pushes the streak past the down threshold.
	probeRound()
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "down", "连续 4 次失败,最近错误: HTTP 503: No available providers")

	// Hub recovers: one successful round flips the endpoint back to healthy
	// because the 24h success rate (82/86) is still above 95%.
	stub.failing.Store(false)
	probeRound()
	entry = findEntry(t, fetchOverview(t, ts.URL), ep)
	expectStatus(t, entry, "healthy", "运行正常")
	if entry.SuccessRate24h == nil || *entry.SuccessRate24h < 0.95 {
		t.Fatalf("expected recovered 24h success rate >= 0.95, got %v", entry.SuccessRate24h)
	}
}

// TestOverviewWindowStats asserts the numeric 24h statistics and both
// degraded rules (low success rate, latency above baseline) through the API.
func TestOverviewWindowStats(t *testing.T) {
	db := openTempDB(t)

	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-stats")
	rateEp := int64(ids[0])
	latEp := int64(ids[1])

	// Endpoint A: 10 probes in the 24h window with latencies 100..1000ms and
	// 2 failures. Rate 0.8 -> degraded by success rate; p50=500, p95=1000
	// under nearest-rank percentiles. Failures are seeded first so the
	// streak is broken and the rate rule decides.
	failErr := "HTTP 500: boom"
	for i := 1; i <= 10; i++ {
		ok := i > 2
		var errSummary *string
		if !ok {
			errSummary = &failErr
		}
		seedProbe(t, db, rateEp, ok, i*100, errSummary, fakeNow.Add(-time.Duration(11-i)*time.Minute))
	}
	entry := findEntry(t, fetchOverview(t, ts.URL), rateEp)
	expectStatus(t, entry, "degraded", "24h 成功率 80.0% 低于 95%")
	if entry.SuccessRate24h == nil || *entry.SuccessRate24h != 0.8 {
		t.Fatalf("expected 24h success rate 0.8, got %v", entry.SuccessRate24h)
	}
	if entry.P50Ms == nil || *entry.P50Ms != 500 {
		t.Fatalf("expected p50 500, got %v", entry.P50Ms)
	}
	if entry.P95Ms == nil || *entry.P95Ms != 1000 {
		t.Fatalf("expected p95 1000, got %v", entry.P95Ms)
	}

	// Endpoint B: 20 baseline probes at ~1s spread over the past 6 days
	// (outside the 24h window) plus 5 in-window probes at 3s. The 7-day P50
	// baseline is 1s, so the 24h P95 of 3s exceeds twice the baseline.
	for i := 0; i < 20; i++ {
		at := fakeNow.Add(-time.Duration(24*(i%5+1)) * time.Hour)
		seedProbe(t, db, latEp, true, 1000, nil, at)
	}
	for i := 1; i <= 5; i++ {
		seedProbe(t, db, latEp, true, 3000, nil, fakeNow.Add(-time.Duration(i)*time.Minute))
	}
	entry = findEntry(t, fetchOverview(t, ts.URL), latEp)
	expectStatus(t, entry, "degraded", "P95 延迟 3.0s 超过基线 2 倍(基线 1.0s)")

	// Endpoint C would fail the latency check but has too few probes for a
	// baseline, so the check is skipped and it stays healthy.
	stub2 := newStubHubServer()
	defer stub2.Close()
	ids2 := createModelEndpoints(t, ts.URL, stub2.URL, "model-no-baseline")
	thinEp := int64(ids2[0])
	for i := 1; i <= 3; i++ {
		seedProbe(t, db, thinEp, true, 9000, nil, fakeNow.Add(-time.Duration(i)*time.Minute))
	}
	entry = findEntry(t, fetchOverview(t, ts.URL), thinEp)
	expectStatus(t, entry, "healthy", "运行正常")
}
