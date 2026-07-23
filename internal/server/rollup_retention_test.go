package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/scheduler"
	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// startRollupWorker runs a rollup worker on the fake clock with short
// intervals and a short retention so tests can exercise rollup and cleanup
// without simulating 90 days.
func startRollupWorker(t *testing.T, db *store.DB, clock *scheduler.FakeClock) {
	t.Helper()
	worker := scheduler.NewRollupWorker(db, clock,
		scheduler.WithRollupInterval(time.Hour),
		scheduler.WithCleanupInterval(24*time.Hour),
		scheduler.WithRetention(48*time.Hour),
		scheduler.WithRollupLag(time.Hour),
		scheduler.WithRollupPollInterval(time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("rollup worker did not stop within 10s of cancellation")
		}
	})
}

// parkRollupWorker waits until the worker has finished its current tick and
// is parked on its poll timer. Advancing the fake clock while the worker is
// mid-tick fires no timer (none is armed yet) and the advance is lost: the
// worker then re-arms against the already-advanced clock and never wakes
// again. FakeClock.TimerCount is the documented synchronization hook for
// this.
func parkRollupWorker(t *testing.T, clock *scheduler.FakeClock) {
	t.Helper()
	waitFor(t, "rollup worker parked on its timer", func() bool {
		return clock.TimerCount() > 0
	})
}

// rawProbeCount returns how many raw probe records the API lists for an
// endpoint (the raw list only reflects un-deleted rows).
func rawProbeCount(t *testing.T, baseURL string, endpointID int64) int {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/endpoints/%d/probes?limit=200", baseURL, endpointID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list probes: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode probes: %v", err)
	}
	var probes []map[string]interface{}
	if err := json.Unmarshal(env.Data, &probes); err != nil {
		t.Fatalf("unmarshal probes: %v", err)
	}
	return len(probes)
}

// TestRollupAndRetention drives the rollup worker with a fake clock across
// day boundaries and asserts the acceptance criteria:
//   - hourly rollups are produced for old probes,
//   - raw probes past the retention window are deleted,
//   - the series API still serves the rolled-up history after the raw rows
//     are gone, without double counting rows that exist in both forms.
func TestRollupAndRetention(t *testing.T) {
	db := openTempDB(t)

	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(start)
	// Rate limits are disabled: the wait loops below poll the read API far
	// faster than the default read tier allows. Limit behavior is covered by
	// dedicated tests with tiny tiers.
	ts := httptest.NewServer(server.New(db,
		server.WithNow(clock.Now), server.WithRateLimits(server.RateLimits{})))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-rollup")
	ep := int64(ids[0])

	// Old probes: 3 days ago, beyond the test retention of 48h. They must be
	// rolled up and then raw-deleted, yet remain visible through the series.
	oldBase := start.Add(-3 * 24 * time.Hour) // 2026-07-17 00:00
	ttft40 := 40
	seedProbeFull(t, db, ep, false, true, 100, nil, nil, oldBase.Add(5*time.Minute)) // 07-17 00:05
	seedProbeFull(t, db, ep, true, true, 300, &ttft40, nil, oldBase.Add(25*time.Minute))

	// Recent probe: 30 minutes before the fake now, inside the raw tail.
	seedProbeFull(t, db, ep, false, true, 500, nil, nil, start.Add(-30*time.Minute)) // 07-19 23:30

	startRollupWorker(t, db, clock)

	// The worker's first tick rolls up everything older than 1h and deletes
	// raw probes older than 48h. Wait until both effects are observable.
	oldBucket := "2026-07-17T00:00:00Z"
	waitFor(t, "rollup of old probes", func() bool {
		buckets := fetchSeries(t, ts.URL, ep, "hours=240&streaming=all")
		for _, b := range buckets {
			if b.BucketStart == oldBucket {
				return true
			}
		}
		return false
	})
	waitFor(t, "retention cleanup of old raw probes", func() bool {
		return rawProbeCount(t, ts.URL, ep) == 1
	})

	// Rollup stats must match what raw aggregation produced before deletion:
	// total 2, no failures, p50=100 (nearest-rank of [100,300]), p95=300,
	// avg ttft 40 over the single streaming record.
	buckets := fetchSeries(t, ts.URL, ep, "hours=240&streaming=all")
	expectBucket(t, findBucket(t, buckets, oldBucket), 2, 0, f64(100), f64(300), f64(40))

	// Cross a day boundary: the recent probe ages past the rollup lag and gets
	// rolled up too. The watermark must keep series from double counting it.
	parkRollupWorker(t, clock)
	clock.Advance(25 * time.Hour) // now 2026-07-21 01:00
	waitFor(t, "rollup of the recent probe", func() bool {
		buckets := fetchSeries(t, ts.URL, ep, "hours=240&streaming=all")
		for _, b := range buckets {
			if b.BucketStart == "2026-07-19T23:00:00Z" && b.Total == 1 {
				return true
			}
		}
		return false
	})

	buckets = fetchSeries(t, ts.URL, ep, "hours=240&streaming=all")
	expectBucket(t, findBucket(t, buckets, oldBucket), 2, 0, f64(100), f64(300), f64(40))
	expectBucket(t, findBucket(t, buckets, "2026-07-19T23:00:00Z"), 1, 0, f64(500), f64(500), nil)
	// The recent raw row is still within retention, so the raw list keeps it.
	if got := rawProbeCount(t, ts.URL, ep); got != 1 {
		t.Fatalf("expected 1 raw probe after first cleanup, got %d", got)
	}

	// Advance past the retention of the recent probe as well. All raw rows are
	// gone, but the series must still return both buckets from rollups.
	parkRollupWorker(t, clock)
	clock.Advance(49 * time.Hour) // now 2026-07-23 02:00
	waitFor(t, "retention cleanup of all raw probes", func() bool {
		return rawProbeCount(t, ts.URL, ep) == 0
	})

	buckets = fetchSeries(t, ts.URL, ep, "hours=240&streaming=all")
	expectBucket(t, findBucket(t, buckets, oldBucket), 2, 0, f64(100), f64(300), f64(40))
	expectBucket(t, findBucket(t, buckets, "2026-07-19T23:00:00Z"), 1, 0, f64(500), f64(500), nil)

	// Rollups respect the streaming split as well.
	buckets = fetchSeries(t, ts.URL, ep, "hours=240&streaming=streaming")
	expectBucket(t, findBucket(t, buckets, oldBucket), 1, 0, f64(300), f64(300), f64(40))
}
