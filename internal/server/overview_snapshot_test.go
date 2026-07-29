package server_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
)

// getOverviewRaw issues an anonymous GET /api/overview and returns the
// response with its body read and closed, for header-level assertions.
func getOverviewRaw(t *testing.T, baseURL string, ifNoneMatch string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/overview", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/overview: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

// TestOverviewSnapshotCachesUntilDirty pins the snapshot read model (spec
// 0015 decision 3): a repeated poll within the same hour and without any
// write must serve the SAME snapshot (identical generated_at proves no
// recompute), while every invalidation channel forces a rebuild —
//   - the clock crossing an hour boundary (time-window decay),
//   - a probe write (here seeded directly; the watermark sentinel must catch
//     writes that bypass the handlers),
//   - a structural write through an API handler (endpoint disable).
func TestOverviewSnapshotCachesUntilDirty(t *testing.T) {
	db := openTempDB(t)

	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-snapshot")
	ep := int64(ids[0])

	first := fetchOverview(t, ts.URL)
	if first.GeneratedAt != fakeNow.Format(time.RFC3339) {
		t.Fatalf("expected generated_at %s, got %s", fakeNow.Format(time.RFC3339), first.GeneratedAt)
	}

	// Same hour, no writes: the cached snapshot is served as-is.
	fakeNow = fakeNow.Add(30 * time.Second)
	second := fetchOverview(t, ts.URL)
	if second.GeneratedAt != first.GeneratedAt {
		t.Fatalf("snapshot must be served unchanged within the hour: generated_at moved %s -> %s",
			first.GeneratedAt, second.GeneratedAt)
	}

	// Crossing an hour boundary rebuilds (time windows slide).
	fakeNow = fakeNow.Add(time.Hour)
	third := fetchOverview(t, ts.URL)
	if third.GeneratedAt == second.GeneratedAt {
		t.Fatal("snapshot must rebuild when the clock crosses an hour boundary")
	}

	// A probe write rebuilds — seeded directly into the store, so only a
	// watermark sentinel (not a handler dirty flag) can catch it.
	fakeNow = fakeNow.Add(time.Minute)
	seedProbe(t, db, ep, true, 100, nil, fakeNow.Add(-time.Minute))
	fourth := fetchOverview(t, ts.URL)
	if fourth.GeneratedAt == third.GeneratedAt {
		t.Fatal("snapshot must rebuild after a probe write")
	}
	entry := findEntry(t, fourth, ep)
	expectStatus(t, entry, "healthy", "运行正常")

	// A structural write through a handler rebuilds: disable the endpoint.
	fakeNow = fakeNow.Add(time.Minute)
	resp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, ep), map[string]interface{}{"enabled": false})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable endpoint: expected 200, got %d", resp.StatusCode)
	}
	fifth := fetchOverview(t, ts.URL)
	if fifth.GeneratedAt == fourth.GeneratedAt {
		t.Fatal("snapshot must rebuild after an endpoint write")
	}
	if entry := findEntry(t, fifth, ep); entry.Enabled {
		t.Fatal("disabled endpoint must show enabled=false on the first read after the write")
	}
}

// TestOverviewSnapshotConcurrentConsistency hammers the overview endpoint
// with concurrent pollers (the status board's multi-tab reality): every
// request must succeed and return a byte-identical body, and the race
// detector must stay silent (go test -race).
func TestOverviewSnapshotConcurrentConsistency(t *testing.T) {
	db := openTempDB(t)

	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-concurrent")
	for i, id := range ids {
		seedProbe(t, db, int64(id), true, 100+i, nil, fakeNow.Add(-time.Minute))
	}

	const pollers = 16
	bodies := make([][]byte, pollers)
	var wg sync.WaitGroup
	for i := 0; i < pollers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, body := getOverviewRaw(t, ts.URL, "")
			if resp.StatusCode != http.StatusOK {
				t.Errorf("poller %d: expected 200, got %d", i, resp.StatusCode)
				return
			}
			bodies[i] = body
		}(i)
	}
	wg.Wait()

	for i := 1; i < pollers; i++ {
		if string(bodies[i]) != string(bodies[0]) {
			t.Fatalf("concurrent pollers must see identical snapshots (poller 0 vs %d differ)", i)
		}
	}
}

// TestOverviewETag pins the conditional-request contract (spec 0015
// decision 4): the overview response carries an ETag plus
// Cache-Control: no-cache (so browsers revalidate every poll instead of
// serving heuristic cache hits); an If-None-Match hit answers 304 with an
// empty body; any data change (probe write / endpoint write / probe round
// through the real prober) yields a new ETag and a 200 with the new body.
func TestOverviewETag(t *testing.T) {
	db := openTempDB(t)

	fakeNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithNow(func() time.Time { return fakeNow })))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-etag")
	ep := int64(ids[0])
	seedProbe(t, db, ep, true, 100, nil, fakeNow.Add(-time.Minute))

	resp, body := getOverviewRaw(t, ts.URL, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("overview must carry an ETag header")
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Fatalf("overview must carry Cache-Control: no-cache (got %q) — without it browsers may serve heuristic cache hits without revalidating", cc)
	}
	if len(body) == 0 {
		t.Fatal("200 response must carry the overview body")
	}

	// Unchanged data: the conditional request revalidates to 304, empty body.
	resp, body = getOverviewRaw(t, ts.URL, etag)
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match hit: expected 304, got %d", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Fatalf("304 must carry no body, got %d bytes", len(body))
	}

	// A probe write changes the data: the stale validator gets a 200 with a
	// new ETag and the full body.
	fakeNow = fakeNow.Add(time.Minute)
	seedProbe(t, db, ep, false, 100, nil, fakeNow.Add(-time.Minute))
	resp, body = getOverviewRaw(t, ts.URL, etag)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stale validator: expected 200, got %d", resp.StatusCode)
	}
	etag2 := resp.Header.Get("ETag")
	if etag2 == "" || etag2 == etag {
		t.Fatalf("data change must yield a new ETag (old %q, new %q)", etag, etag2)
	}
	if len(body) == 0 {
		t.Fatal("200 after data change must carry the overview body")
	}

	// The new validator revalidates to 304 again.
	resp, _ = getOverviewRaw(t, ts.URL, etag2)
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("new validator: expected 304, got %d", resp.StatusCode)
	}

	// A structural write (endpoint disable) also moves the ETag.
	resp = doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, ep), map[string]interface{}{"enabled": false})
	resp.Body.Close()
	resp, _ = getOverviewRaw(t, ts.URL, etag2)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("endpoint write must invalidate: expected 200, got %d", resp.StatusCode)
	}
	if etag3 := resp.Header.Get("ETag"); etag3 == etag2 {
		t.Fatal("endpoint write must yield a new ETag")
	}
}

// TestOverviewETagChangesAfterProbeRound drives the probe-round invalidation
// channel end to end: a real probe round through POST
// /api/endpoints/{id}/probe (whose AfterRound hook invalidates the snapshot)
// must move the ETag, so a polling board sees the new state on its next
// conditional request.
func TestOverviewETagChangesAfterProbeRound(t *testing.T) {
	db := openTempDB(t)
	stub := newSwitchStubHub(t)
	ts := newTestAPIServer(t, db)

	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-etag-round")
	ep := int64(ids[0])

	resp, _ := getOverviewRaw(t, ts.URL, "")
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("overview must carry an ETag header")
	}

	// A failing probe round changes the endpoint status.
	stub.failing.Store(true)
	probeResp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, ep), nil)
	probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusOK {
		t.Fatalf("probe round: expected 200, got %d", probeResp.StatusCode)
	}

	resp, _ = getOverviewRaw(t, ts.URL, etag)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe round must invalidate the snapshot: expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("ETag") == etag {
		t.Fatal("probe round must yield a new ETag")
	}
	entry := findEntry(t, fetchOverview(t, ts.URL), ep)
	if entry.Status != "failing" {
		t.Fatalf("expected failing after a failed round, got %q", entry.Status)
	}
}
