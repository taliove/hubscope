package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// detailPayload mirrors the GET /api/endpoints/{id} response.
type detailPayload struct {
	ID           int64  `json:"id"`
	ModelID      int64  `json:"model_id"`
	Protocol     string `json:"protocol"`
	Enabled      bool   `json:"enabled"`
	ModelIDStr   string `json:"model_id_str"`
	HubName      string `json:"hub_name"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
}

// seriesBucket mirrors one bucket of the GET /api/endpoints/{id}/series response.
type seriesBucket struct {
	BucketStart string   `json:"bucket_start"`
	Total       int      `json:"total"`
	Failures    int      `json:"failures"`
	P50Ms       *float64 `json:"p50_ms"`
	P95Ms       *float64 `json:"p95_ms"`
	AvgTTFTMs   *float64 `json:"avg_ttft_ms"`
}

// seedProbeFull inserts a probe record with full control over streaming mode,
// TTFT, and timestamp. Unlike seedProbe (overview tests) it supports streaming
// records, which the series aggregation needs.
func seedProbeFull(t *testing.T, db *store.DB, endpointID int64, streaming, ok bool, latencyMs int, ttftMs *int, errSummary *string, at time.Time) {
	t.Helper()
	httpStatus := 200
	if !ok {
		httpStatus = 503
	}
	_, err := db.CreateProbe(store.Probe{
		EndpointID:   endpointID,
		Streaming:    streaming,
		OK:           ok,
		HTTPStatus:   httpStatus,
		ErrorSummary: errSummary,
		LatencyMs:    latencyMs,
		TTFTMs:       ttftMs,
		CreatedAt:    at,
	})
	if err != nil {
		t.Fatalf("seed probe: %v", err)
	}
}

// fetchDetail GETs /api/endpoints/{id} and decodes the payload.
func fetchDetail(t *testing.T, baseURL string, endpointID int64) detailPayload {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d", baseURL, endpointID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET detail: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	var payload detailPayload
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	return payload
}

// fetchSeries GETs /api/endpoints/{id}/series with the given query string and
// decodes the bucket list.
func fetchSeries(t *testing.T, baseURL string, endpointID int64, query string) []seriesBucket {
	t.Helper()
	url := fmt.Sprintf("%s/api/endpoints/%d/series", baseURL, endpointID)
	if query != "" {
		url += "?" + query
	}
	resp := doGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET series?%s: expected 200, got %d", query, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode series: %v", err)
	}
	var buckets []seriesBucket
	if err := json.Unmarshal(env.Data, &buckets); err != nil {
		t.Fatalf("unmarshal series: %v", err)
	}
	return buckets
}

// findBucket returns the bucket starting at the given RFC3339 hour or fails.
func findBucket(t *testing.T, buckets []seriesBucket, bucketStart string) seriesBucket {
	t.Helper()
	for _, b := range buckets {
		if b.BucketStart == bucketStart {
			return b
		}
	}
	t.Fatalf("bucket %s not present in series (%d buckets)", bucketStart, len(buckets))
	return seriesBucket{}
}

// expectBucket asserts all fields of one bucket.
func expectBucket(t *testing.T, b seriesBucket, total, failures int, p50, p95, avgTTFT *float64) {
	t.Helper()
	if b.Total != total || b.Failures != failures {
		t.Fatalf("bucket %s: expected total=%d failures=%d, got total=%d failures=%d",
			b.BucketStart, total, failures, b.Total, b.Failures)
	}
	assertFloatPtrEq(t, b.BucketStart, "p50_ms", b.P50Ms, p50)
	assertFloatPtrEq(t, b.BucketStart, "p95_ms", b.P95Ms, p95)
	assertFloatPtrEq(t, b.BucketStart, "avg_ttft_ms", b.AvgTTFTMs, avgTTFT)
}

// assertFloatPtrEq asserts two nullable floats are both null or equal.
func assertFloatPtrEq(t *testing.T, ctx, field string, got, want *float64) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s: expected %s null, got %v", ctx, field, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s: expected %s %v, got %v", ctx, field, *want, got)
	}
}

func f64(v float64) *float64 { return &v }

// TestEndpointDetail verifies the detail API returns endpoint fields plus
// model string, hub name, and a status that follows the same state machine
// as the overview.
func TestEndpointDetail(t *testing.T) {
	db := openTempDB(t)
	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db, server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-detail")
	ep := int64(ids[0])

	// Never probed: healthy with the no-data reason.
	d := fetchDetail(t, ts.URL, ep)
	if d.ID != ep || d.Protocol != "anthropic" || !d.Enabled {
		t.Fatalf("unexpected endpoint fields: %+v", d)
	}
	if d.ModelIDStr != "model-detail" {
		t.Fatalf("expected model_id_str %q, got %q", "model-detail", d.ModelIDStr)
	}
	if d.HubName != "hub-model-detail" {
		t.Fatalf("expected hub_name %q, got %q", "hub-model-detail", d.HubName)
	}
	if d.Status != "healthy" || d.StatusReason != "暂无探测数据" {
		t.Fatalf("expected healthy/暂无探测数据, got %s/%s", d.Status, d.StatusReason)
	}

	// Three consecutive failures flip the endpoint to down.
	failErr := "HTTP 503: No available providers"
	for i := 3; i >= 1; i-- {
		seedProbeFull(t, db, ep, false, false, 120, nil, &failErr, fakeNow.Add(-time.Duration(i)*time.Minute))
	}
	d = fetchDetail(t, ts.URL, ep)
	if d.Status != "down" {
		t.Fatalf("expected down, got %s (%s)", d.Status, d.StatusReason)
	}
	wantReason := "连续 3 次失败,最近错误: HTTP 503: No available providers"
	if d.StatusReason != wantReason {
		t.Fatalf("expected reason %q, got %q", wantReason, d.StatusReason)
	}

	// Unknown endpoint must 404.
	resp := doGet(t, ts.URL+"/api/endpoints/999999")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown endpoint: expected 404, got %d", resp.StatusCode)
	}
}

// TestEndpointSeriesAggregation verifies hourly bucket math for the three
// streaming modes against seeded multi-hour history.
func TestEndpointSeriesAggregation(t *testing.T) {
	db := openTempDB(t)
	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db, server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-series")
	ep := int64(ids[0])

	ttft50, ttft150, ttft100 := 50, 150, 100
	failErr := "HTTP 503: boom"

	// Bucket 10:00 — two non-streaming (one failure) and two streaming probes.
	seedProbeFull(t, db, ep, false, true, 100, nil, nil, fakeNow.Add(-115*time.Minute))       // 10:05
	seedProbeFull(t, db, ep, false, false, 300, nil, &failErr, fakeNow.Add(-100*time.Minute)) // 10:20
	seedProbeFull(t, db, ep, true, true, 200, &ttft50, nil, fakeNow.Add(-110*time.Minute))    // 10:10
	seedProbeFull(t, db, ep, true, true, 400, &ttft150, nil, fakeNow.Add(-95*time.Minute))    // 10:25
	// Bucket 11:00 — one streaming probe.
	seedProbeFull(t, db, ep, true, true, 600, &ttft100, nil, fakeNow.Add(-50*time.Minute)) // 11:10
	// Outside the 24h window: only visible with hours=48.
	seedProbeFull(t, db, ep, false, true, 999, nil, nil, fakeNow.Add(-30*time.Hour))

	// streaming=all merges both probe kinds.
	buckets := fetchSeries(t, ts.URL, ep, "hours=24&streaming=all")
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %+v", len(buckets), buckets)
	}
	expectBucket(t, findBucket(t, buckets, "2026-07-20T10:00:00Z"), 4, 1, f64(200), f64(400), f64(100))
	expectBucket(t, findBucket(t, buckets, "2026-07-20T11:00:00Z"), 1, 0, f64(600), f64(600), f64(100))

	// streaming=non_streaming drops the streaming-only bucket.
	buckets = fetchSeries(t, ts.URL, ep, "hours=24&streaming=non_streaming")
	if len(buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d: %+v", len(buckets), buckets)
	}
	expectBucket(t, findBucket(t, buckets, "2026-07-20T10:00:00Z"), 2, 1, f64(100), f64(300), nil)

	// streaming=streaming keeps both buckets with streaming-only stats.
	buckets = fetchSeries(t, ts.URL, ep, "hours=24&streaming=streaming")
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %+v", len(buckets), buckets)
	}
	expectBucket(t, findBucket(t, buckets, "2026-07-20T10:00:00Z"), 2, 0, f64(200), f64(400), f64(100))
	expectBucket(t, findBucket(t, buckets, "2026-07-20T11:00:00Z"), 1, 0, f64(600), f64(600), f64(100))

	// Defaults (no query params) behave like hours=24&streaming=all.
	buckets = fetchSeries(t, ts.URL, ep, "")
	if len(buckets) != 2 {
		t.Fatalf("default params: expected 2 buckets, got %d", len(buckets))
	}

	// A wider window includes the 30h-old probe.
	buckets = fetchSeries(t, ts.URL, ep, "hours=48&streaming=non_streaming")
	if len(buckets) != 2 {
		t.Fatalf("hours=48: expected 2 buckets, got %d", len(buckets))
	}
	expectBucket(t, findBucket(t, buckets, "2026-07-19T06:00:00Z"), 1, 0, f64(999), f64(999), nil)
}

// TestEndpointSeriesValidation covers parameter validation of the series API.
func TestEndpointSeriesValidation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-series-validation")
	ep := ids[0]

	badQueries := []string{
		"hours=0",
		"hours=-5",
		"hours=2161",
		"hours=abc",
		"streaming=bogus",
	}
	for _, q := range badQueries {
		resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/series?%s", ts.URL, ep, q))
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("series?%s: expected 400, got %d", q, resp.StatusCode)
		}
	}

	// Boundary values are accepted.
	for _, q := range []string{"hours=1", "hours=2160"} {
		resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/series?%s", ts.URL, ep, q))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("series?%s: expected 200, got %d", q, resp.StatusCode)
		}
	}

	resp := doGet(t, ts.URL+"/api/endpoints/999999/series")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown endpoint series: expected 404, got %d", resp.StatusCode)
	}
}

// TestProbesOkFilter covers the ok=true|false filter on the probes API.
func TestProbesOkFilter(t *testing.T) {
	db := openTempDB(t)
	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db, server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-ok-filter")
	ep := ids[0]

	failErr := "HTTP 500: boom"
	for i := 0; i < 3; i++ {
		seedProbeFull(t, db, int64(ep), false, false, 100, nil, &failErr, fakeNow.Add(-time.Duration(i+1)*time.Minute))
	}
	for i := 0; i < 2; i++ {
		seedProbeFull(t, db, int64(ep), false, true, 100, nil, nil, fakeNow.Add(-time.Duration(i+10)*time.Minute))
	}

	fetchProbes := func(query string) []map[string]interface{} {
		t.Helper()
		resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes?%s", ts.URL, ep, query))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("probes?%s: expected 200, got %d", query, resp.StatusCode)
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

	failures := fetchProbes("ok=false")
	if len(failures) != 3 {
		t.Fatalf("ok=false: expected 3 records, got %d", len(failures))
	}
	for _, p := range failures {
		if p["ok"].(bool) {
			t.Fatal("ok=false returned a successful probe")
		}
		if p["error_summary"].(string) != failErr {
			t.Fatalf("expected error summary %q, got %v", failErr, p["error_summary"])
		}
	}

	successes := fetchProbes("ok=true")
	if len(successes) != 2 {
		t.Fatalf("ok=true: expected 2 records, got %d", len(successes))
	}
	for _, p := range successes {
		if !p["ok"].(bool) {
			t.Fatal("ok=true returned a failed probe")
		}
	}

	// The filter composes with limit.
	limited := fetchProbes("ok=false&limit=2")
	if len(limited) != 2 {
		t.Fatalf("ok=false&limit=2: expected 2 records, got %d", len(limited))
	}

	// Invalid filter values are rejected.
	resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes?ok=maybe", ts.URL, ep))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("ok=maybe: expected 400, got %d", resp.StatusCode)
	}
}
