package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/scheduler"
	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
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
		scheduler.WithEvalPollInterval(time.Minute))
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
			t.Error("eval worker did not stop within 10s of cancellation")
		}
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

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{}))
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
	clock := scheduler.NewFakeClock(time.Date(2026, 7, 18, 23, 30, 0, 0, time.UTC))
	startEvalWorker(t, db, srv, clock)

	runs := listEvalRuns(t, ts.URL)
	if got := countRunsByTrigger(runs, "scheduled"); got != 0 {
		t.Fatalf("before the window: expected 0 scheduled runs, got %d", got)
	}

	// Advance into Sunday 01:30: the weekly batch fires — one run per suite
	// (4 built-in suites), each covering only the chat model.
	clock.Advance(2 * time.Hour)
	waitFor(t, "weekly batch of 4 scheduled runs", func() bool {
		return countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled") == 4
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
	clock.Advance(3 * time.Hour)
	runs = listEvalRuns(t, ts.URL)
	if got := countRunsByTrigger(runs, "scheduled"); got != 4 {
		t.Fatalf("same Sunday: expected still 4 scheduled runs, got %d", got)
	}

	// Next Sunday (6 days 20 hours later, landing at 00:30): a fresh batch.
	clock.Advance(164 * time.Hour)
	waitFor(t, "next week's batch", func() bool {
		return countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled") == 8
	})
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

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{}))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	createEvalModel(t, ts.URL, stub.URL, "restart-model")

	// Start inside the window: the first worker fires the batch.
	clock := scheduler.NewFakeClock(time.Date(2026, 7, 19, 1, 30, 0, 0, time.UTC)) // a Sunday
	startEvalWorker(t, db, srv, clock)
	waitFor(t, "initial weekly batch", func() bool {
		return countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled") == 4
	})

	// Simulate a restart: a brand-new worker with empty in-memory state over
	// the same database must not fire again, even as the clock advances
	// within the window.
	startEvalWorker(t, db, srv, clock)
	clock.Advance(2 * time.Hour)
	time.Sleep(200 * time.Millisecond)
	if got := countRunsByTrigger(listEvalRuns(t, ts.URL), "scheduled"); got != 4 {
		t.Fatalf("after restart inside window: expected still 4 scheduled runs, got %d", got)
	}
}
