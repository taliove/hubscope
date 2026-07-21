package server_test

import (
	"testing"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/scheduler"
)

// TestSchedulerDefaultInterval verifies that enabled endpoints produce probe
// records at the default 300s interval once the fake clock is advanced.
func TestSchedulerDefaultInterval(t *testing.T) {
	db := openTempDB(t)
	stub := newDelayStubHub(t, 10*time.Millisecond)
	ts := newTestAPIServer(t, db)

	// One model yields two enabled endpoints (anthropic + openai).
	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "default-interval-model")

	clock := scheduler.NewFakeClock(time.Now())
	startScheduler(t, db, hubclient.New(), clock)

	// Startup: every enabled endpoint is due immediately, round = 2 records.
	for _, id := range endpointIDs {
		waitForProbeCount(t, ts.URL, id, 2)
	}

	// Advance one default interval: a second round appears.
	clock.Advance(300 * time.Second)
	for _, id := range endpointIDs {
		waitForProbeCount(t, ts.URL, id, 4)
	}

	// Advance another interval: a third round appears.
	clock.Advance(300 * time.Second)
	for _, id := range endpointIDs {
		waitForProbeCount(t, ts.URL, id, 6)
	}

	// Without further advances no extra rounds may fire.
	for _, id := range endpointIDs {
		assertProbeCountStable(t, ts.URL, id, 6, 300*time.Millisecond)
		for _, p := range probeRecords(t, ts.URL, id) {
			if p["ok"].(bool) != true {
				t.Errorf("endpoint %d: expected successful probe, got %v", id, p["error_summary"])
			}
		}
	}
}

// TestSchedulerIntervalOverrideAndEnableDisable verifies that PATCH changes
// take effect on the next scheduled round: a slower interval override, a
// reset back to the default, disabling, and re-enabling.
func TestSchedulerIntervalOverrideAndEnableDisable(t *testing.T) {
	db := openTempDB(t)
	stub := newDelayStubHub(t, 10*time.Millisecond)
	ts := newTestAPIServer(t, db)

	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "override-model")
	target, other := endpointIDs[0], endpointIDs[1]

	// Disable the sibling endpoint upfront so only the target produces records.
	resp := patchEndpoint(t, ts.URL, other, map[string]interface{}{"enabled": false})
	resp.Body.Close()

	clock := scheduler.NewFakeClock(time.Now())
	startScheduler(t, db, hubclient.New(), clock)

	// Round 1 fires immediately at startup with the default interval.
	waitForProbeCount(t, ts.URL, target, 2)

	// Slow the endpoint down to a 900s interval.
	resp = patchEndpoint(t, ts.URL, target, map[string]interface{}{"interval_seconds": 900})
	resp.Body.Close()

	// Advancing only 300s must not trigger another round.
	clock.Advance(300 * time.Second)
	assertProbeCountStable(t, ts.URL, target, 2, 300*time.Millisecond)

	// Reaching 900s since the last completion triggers round 2.
	clock.Advance(600 * time.Second)
	waitForProbeCount(t, ts.URL, target, 4)

	// Clearing the override restores the default 300s interval.
	resp = patchEndpoint(t, ts.URL, target, map[string]interface{}{"interval_seconds": nil})
	resp.Body.Close()
	clock.Advance(300 * time.Second)
	waitForProbeCount(t, ts.URL, target, 6)

	// Disabling stops scheduling entirely, even far into the future.
	resp = patchEndpoint(t, ts.URL, target, map[string]interface{}{"enabled": false})
	resp.Body.Close()
	clock.Advance(900 * time.Second)
	assertProbeCountStable(t, ts.URL, target, 6, 300*time.Millisecond)
	if got := len(probeRecords(t, ts.URL, other)); got != 0 {
		t.Fatalf("disabled endpoint produced %d probe records", got)
	}

	// Re-enabling resumes scheduling: the next poll tick picks it up as due.
	resp = patchEndpoint(t, ts.URL, target, map[string]interface{}{"enabled": true})
	resp.Body.Close()
	clock.Advance(time.Second)
	waitForProbeCount(t, ts.URL, target, 8)
}

// TestSchedulerNoReentryWhileSlow verifies that an endpoint whose round is
// still in flight is never re-entered, no matter how far the clock advances.
func TestSchedulerNoReentryWhileSlow(t *testing.T) {
	db := openTempDB(t)
	// 250ms per request makes one round take roughly half a second.
	stub := newDelayStubHub(t, 250*time.Millisecond)
	ts := newTestAPIServer(t, db)

	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "slow-model")
	target, other := endpointIDs[0], endpointIDs[1]

	resp := patchEndpoint(t, ts.URL, other, map[string]interface{}{"enabled": false})
	resp.Body.Close()

	clock := scheduler.NewFakeClock(time.Now())
	startScheduler(t, db, hubclient.New(), clock)

	// Round 1 completes after about 500ms of real time.
	waitForProbeCount(t, ts.URL, target, 2)

	// Dispatch round 2, then race the clock far ahead while it is in flight.
	clock.Advance(300 * time.Second)
	clock.Advance(300 * time.Second)
	clock.Advance(300 * time.Second)
	clock.Advance(300 * time.Second)

	// Round 2 finishes: exactly 4 records. The fake clock is far ahead of the
	// next due time, yet no catch-up rounds may pile up.
	waitForProbeCount(t, ts.URL, target, 4)
	assertProbeCountStable(t, ts.URL, target, 4, 800*time.Millisecond)

	// The next round is scheduled from the completion time, so one more
	// interval advance yields exactly one more round.
	clock.Advance(300 * time.Second)
	waitForProbeCount(t, ts.URL, target, 6)
	assertProbeCountStable(t, ts.URL, target, 6, 800*time.Millisecond)
}

// TestSchedulerReEnableAfterDisableInFlight verifies that an endpoint
// disabled while its round is in flight, once re-enabled, becomes due
// immediately instead of waiting a full interval anchored to that round's
// completion.
func TestSchedulerReEnableAfterDisableInFlight(t *testing.T) {
	db := openTempDB(t)
	// 250ms per request keeps round 1 in flight long enough to disable it.
	stub := newDelayStubHub(t, 250*time.Millisecond)
	ts := newTestAPIServer(t, db)

	endpointIDs := createModelEndpoints(t, ts.URL, stub.URL, "re-enable-model")
	target, other := endpointIDs[0], endpointIDs[1]

	resp := patchEndpoint(t, ts.URL, other, map[string]interface{}{"enabled": false})
	resp.Body.Close()

	clock := scheduler.NewFakeClock(time.Now())
	startScheduler(t, db, hubclient.New(), clock)

	// Wait until round 1 is actually in flight, then disable the endpoint.
	waitFor(t, "round 1 in flight", func() bool { return stub.totalRequests() > 0 })
	resp = patchEndpoint(t, ts.URL, target, map[string]interface{}{"enabled": false})
	resp.Body.Close()

	// Let the scheduler observe the disable, then jump far past the interval.
	clock.Advance(time.Second)
	clock.Advance(900 * time.Second)

	// Round 1 completes but no further round may be scheduled while disabled.
	waitForProbeCount(t, ts.URL, target, 2)
	assertProbeCountStable(t, ts.URL, target, 2, 800*time.Millisecond)

	// Re-enable: the endpoint is treated as new and becomes due immediately,
	// so a single poll tick yields round 2 without waiting a full interval.
	resp = patchEndpoint(t, ts.URL, target, map[string]interface{}{"enabled": true})
	resp.Body.Close()
	clock.Advance(time.Second)
	waitForProbeCount(t, ts.URL, target, 4)
}
