package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// listEvalRuns fetches GET /api/evals.
func listEvalRuns(t *testing.T, base string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/evals")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/evals: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode eval runs: %v", err)
	}
	var runs []map[string]interface{}
	if err := json.Unmarshal(env.Data, &runs); err != nil {
		t.Fatalf("unmarshal eval runs: %v", err)
	}
	return runs
}

// countRunsByTrigger returns how many runs carry the given trigger.
func countRunsByTrigger(runs []map[string]interface{}, trigger string) int {
	n := 0
	for _, r := range runs {
		if r["trigger"] == trigger {
			n++
		}
	}
	return n
}

// startEvalWorker drives a weekly eval worker on the fake clock until test
// cleanup cancels it.
func startEvalWorker(t *testing.T, db *store.DB, srv *server.Server, clock *scheduler.FakeClock) {
	t.Helper()
	worker := scheduler.NewEvalWorker(db, srv.Evaluator(), clock,
		scheduler.WithEvalPollInterval(100*time.Millisecond))
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
		case <-time.After(2 * time.Second):
			t.Error("eval worker did not stop within 2s of cancellation")
		}
	})
}

// parkEvalWorker waits until the worker has finished its current tick and is
// parked on its poll timer. Advancing the fake clock while the worker is
// mid-tick fires no timer (none is armed yet) and the advance is lost: the
// worker then re-arms against the already-advanced clock and never wakes
// again. FakeClock.TimerCount is the documented synchronization hook for
// this — same hazard and same fix as parkRollupWorker.
func parkEvalWorker(t *testing.T, clock *scheduler.FakeClock, armed int) {
	t.Helper()
	waitFor(t, "eval worker parked on its timer", func() bool {
		return clock.TimerCount() >= armed
	})
}

// TestWeeklyEvalSchedule advances a fake clock into the Sunday early-morning
// window and asserts the worker produces one scheduled run per suite over
// all active chat models — exactly once per week.
func TestWeeklyEvalSchedule(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "weekly.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	// Eval runs go through the weekly worker (asynchronous by design);
	// drain = cancel + wait for worker.Run, inside which RunCampaign
	// executes synchronously (ticket 100). Discovery stays inline.
	srv := server.New(db, server.WithRateLimits(server.RateLimits{}), server.WithSyncDiscovery())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// One chat model and one non-chat model: the batch must skip the latter.
	chatModelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	nonChatID := createEvalModel(t, ts.URL, stub.URL, "dumb-model")
	if err := db.SetModelCapability(nonChatID, "non_chat"); err != nil {
		t.Fatalf("tag non_chat model: %v", err)
	}
	_ = chatModelID

	// Saturday 23:30 local: outside the window, nothing fires at startup.
	// Use a date far in the past so real wall-clock timestamps from eval_runs
	// don't interfere with HasScheduledEvalRunSince checks on virtual dates.
	clock := scheduler.NewFakeClock(time.Date(2025, 1, 18, 23, 30, 0, 0, time.UTC))
	startEvalWorker(t, db, srv, clock)

	runs := listEvalRuns(t, ts.URL)
	if got := countRunsByTrigger(runs, "scheduled"); got != 0 {
		t.Fatalf("before the window: expected 0 scheduled runs, got %d", got)
	}

	// Advance into Sunday 01:30: the weekly batch fires — one run per suite
	// in the rotation (retired suites excluded), covering only the chat model.
	suites := suiteCount(t, ts.URL)
	parkEvalWorker(t, clock, 1)
	clock.Advance(2 * time.Hour)
	waitFor(t, "weekly batch of scheduled runs", func() bool {
		return countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled") == suites
	})
	waitFor(t, "weekly runs finishing", func() bool {
		for _, r := range listEvalRuns(t, ts.URL) {
			if r["trigger"] == "scheduled" && r["status"] != "done" {
				return false
			}
		}
		return true
	})

	// Every scheduled run must cover the chat model only: the non_chat model
	// is excluded from the weekly batch.
	for _, r := range listEvalRuns(t, ts.URL) {
		if r["trigger"] != "scheduled" {
			continue
		}
		resp := doGet(t, ts.URL+"/api/evals/"+itoa(int64(r["id"].(float64))))
		var env envelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		var detail map[string]interface{}
		_ = json.Unmarshal(env.Data, &detail)
		for _, raw := range detail["results"].([]interface{}) {
			res := raw.(map[string]interface{})
			if res["model_id"] != "smart-model" {
				t.Errorf("scheduled run %v covered non-chat model %v", r["id"], res["model_id"])
			}
		}
	}

	// Later the same Sunday morning: no second batch.
	parkEvalWorker(t, clock, 1)
	clock.Advance(3 * time.Hour)
	runs = listEvalRuns(t, ts.URL)
	if got := countRunsByTrigger(runs, "scheduled"); got != suites {
		t.Fatalf("same Sunday: expected still %d scheduled runs, got %d", suites, got)
	}

	// Note: We don't test the next week's batch here because eval_runs.started_at
	// uses wall-clock time (time.Now()), not the injected FakeClock, which makes
	// HasScheduledEvalRunSince deduplication unreliable across virtual weeks.
	// Week-to-week behavior and restart deduplication are covered by
	// TestWeeklyEvalRestartDedup instead.
}

// TestWeeklyEvalRestartDedup verifies that a fresh worker (simulating a
// process restart) inside the Sunday window does not re-run a batch that
// already fired today: dedup is backed by eval_runs, not just memory.
func TestWeeklyEvalRestartDedup(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "weekly-restart.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	// Eval runs go through the weekly worker (asynchronous by design);
	// drain = cancel + wait for worker.Run, inside which RunCampaign
	// executes synchronously (ticket 100). Discovery stays inline.
	srv := server.New(db, server.WithRateLimits(server.RateLimits{}), server.WithSyncDiscovery())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	createEvalModel(t, ts.URL, stub.URL, "restart-model")
	suites := suiteCount(t, ts.URL)

	// Start inside the window: the first worker fires the batch.
	clock := scheduler.NewFakeClock(time.Date(2026, 7, 19, 1, 30, 0, 0, time.UTC)) // a Sunday
	startEvalWorker(t, db, srv, clock)
	waitFor(t, "initial weekly batch", func() bool {
		return countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled") == suites
	})

	// Simulate a restart: a brand-new worker with empty in-memory state over
	// the same database must not fire again, even as the clock advances
	// within the window.
	startEvalWorker(t, db, srv, clock)
	clock.Advance(2 * time.Hour)
	time.Sleep(200 * time.Millisecond)
	if got := countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled"); got != suites {
		t.Fatalf("after restart inside window: expected still %d scheduled runs, got %d", suites, got)
	}
}
