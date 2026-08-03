package server_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// TestCampaignPartialFailureAggregatesFailed drives a weekly campaign where
// two suites complete and the rest are aborted mid-batch: the campaign must
// settle to failed with progress showing exactly the completed runs done
// and every other run failed. Since GH #26 every suite's run is created up
// front, so an aborted batch fails the runs it never executed instead of
// never creating them — the settled aggregate is the same (failed).
//
// Why two suites: under the GH #169 model-major order the last model
// (chat-three) is the only one with unstarted cells when the cancel lands,
// and its in-flight agieval_zh cell runs to completion through the
// cancellation (failed cases, no answers) — so agieval_zh settles done
// alongside mmlu while the suites chat-three never started settle failed.
func TestCampaignPartialFailureAggregatesFailed(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "partial-campaign.db"))
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

	// Several chat models widen the in-flight window of a suite run, so the
	// gate deterministically catches the second suite mid-flight.
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")
	createEvalModel(t, ts.URL, stub.URL, "chat-three")

	// Serial cell order (GH #26 pool at 1, GH #169 model-major): smart-model
	// and chat-two run every suite before chat-three starts, so freezing
	// chat-three at its second suite deterministically lands at "first
	// suite done for every model, the rest mid-flight".
	setEvalConcurrency(t, ts.URL, 1)

	suites := suiteCount(t, ts.URL)

	// Freeze point by construction, not by stepping: the first suite in the
	// rotation (mmlu) is all rule cases answered exactly once, so
	// chat-three's mmlu cell costs exactly its enabled-case count of calls.
	// Arming the count-based model gate after that many calls blocks
	// chat-three's first agieval_zh answer deterministically — the N+1-th
	// call is recorded before it blocks, and no release/re-arm window
	// exists for calls to slip through. The gate is armed BEFORE the worker
	// starts: arming mid-flight would let the fast local execution
	// overshoot the threshold first.
	stub.resetCalls()
	mmluCalls := enabledCaseCount(t, ts.URL, suiteIDByKey(t, ts.URL, "mmlu"))
	stub.blockModelAfter("chat-three", mmluCalls)
	t.Cleanup(func() { stub.releaseModel("chat-three") })

	clock := scheduler.NewFakeClock(time.Date(2026, 7, 19, 1, 30, 0, 0, time.UTC)) // a Sunday
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
		stub.releaseModel("chat-three")
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("eval worker did not stop within 10s of cancellation")
		}
	})

	var campaignID int64
	waitFor(t, "campaign created", func() bool {
		campaigns := listCampaigns(t, ts.URL)
		if len(campaigns) != 1 {
			return false
		}
		campaignID = int64(campaigns[0]["id"].(float64))
		return true
	})
	waitFor(t, "chat-three's first agieval_zh call reaching the stub", func() bool {
		return stub.callTotal("chat-three") >= mmluCalls+1
	})

	// Suite 1 is fully done (its last call completing precedes the blocked
	// N+1-th call), and every suite's run exists from the start (GH #26).
	mid := getCampaign(t, ts.URL, campaignID)
	if got := int(campaignProgress(t, mid)["done"].(float64)); got != 1 {
		t.Fatalf("progress.done at freeze point = %d, want 1 (first suite completed)", got)
	}
	if got := len(campaignRuns(t, mid)); got != suites {
		t.Fatalf("campaign has %d runs mid-flight, want one per suite (%d) created up front", got, suites)
	}

	cancel()
	// The gate stays closed: a parked request is then aborted deterministically
	// by ctx cancellation (net/http returns ctx.Err() before any response
	// arrives), with no response race. Releasing the gate here would hand the
	// outcome to transport cancellation vs response arrival (~0.4% flake).
	// The registered cleanup above releases the gate for final shutdown.

	final := waitCampaignStatus(t, ts.URL, campaignID, "failed")
	progress := campaignProgress(t, final)
	if got := int(progress["done"].(float64)); got != 2 {
		t.Errorf("progress.done = %d, want 2 (mmlu plus chat-three's in-flight agieval_zh cell completing through the cancel)", got)
	}
	if got := int(progress["failed"].(float64)); got != suites-2 {
		t.Errorf("progress.failed = %d, want %d (unexecuted suites aborted)", got, suites-2)
	}
	if got := int(progress["running"].(float64)); got != 0 {
		t.Errorf("progress.running = %d, want 0 after settling", got)
	}
	runs := campaignRuns(t, final)
	if len(runs) != suites {
		t.Fatalf("campaign has %d runs, want one per suite (%d)", len(runs), suites)
	}
	for _, run := range runs[:2] {
		if run["status"] != "done" {
			t.Errorf("completed run %v status = %v, want done", run["id"], run["status"])
		}
	}
	for _, run := range runs[2:] {
		if run["status"] != "failed" {
			t.Errorf("aborted run %v status = %v, want failed", run["id"], run["status"])
		}
	}
	if final["finished_at"] == nil {
		t.Error("failed campaign must carry finished_at")
	}

	// The failed campaign's report (ticket 52): the settled board carries
	// per-suite cells — the completed suites done, the aborted suites
	// failed with the suite's planned case count as expected. Under the GH
	// #169 model-major order smart-model and chat-two had already recorded
	// their results everywhere when the cancel landed, while chat-three's
	// in-flight agieval_zh cell completed with zero judged cases and its
	// unstarted suites (gsm8k on) failed without a single result.
	report := getCampaignReport(t, ts.URL, campaignID, "")
	rows := reportRows(t, report)
	if len(rows) != 3 {
		t.Fatalf("failed campaign report rows = %v, want all three models", rows)
	}
	for _, row := range rows {
		assertCell(t, row, "mmlu", "done", 100, 100)
		judged := 100
		if row["model_id"] == "chat-three" {
			judged = 0
		}
		assertCell(t, row, "agieval_zh", "done", judged, 100)
		assertCell(t, row, "gsm8k", "failed", judged, 100)
	}
}

// TestRestartClosesStaleRunningRuns stages a database with a campaign and
// its run both stuck in running (as a crash mid-batch would leave them) and
// asserts the startup cleanup closes both as failed, so campaign progress
// never shows phantom running members.
func TestRestartClosesStaleRunningRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale-campaign.db")

	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	seedTestUser(t, db)
	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	campaign, err := db.CreateCampaign("scheduled", nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	if _, err := db.CreateEvalRun(campaign.ID, suites[0].ID, "scheduled", "fake-judge"); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// Reopening runs the startup cleanup; the server then exposes the
	// settled state over the API.
	db2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	t.Cleanup(func() { db2.Close() })
	seedTestUser(t, db2)
	ts := httptest.NewServer(server.New(db2, server.WithRateLimits(server.RateLimits{})))
	t.Cleanup(ts.Close)

	final := getCampaign(t, ts.URL, campaign.ID)
	if final["status"] != "failed" {
		t.Errorf("campaign status = %v, want failed after restart", final["status"])
	}
	if final["finished_at"] == nil {
		t.Error("cleaned-up campaign must carry finished_at")
	}
	progress := campaignProgress(t, final)
	if got := int(progress["running"].(float64)); got != 0 {
		t.Errorf("progress.running = %d, want 0 (no phantom running members)", got)
	}
	if got := int(progress["failed"].(float64)); got != 1 {
		t.Errorf("progress.failed = %d, want 1", got)
	}
	runs := campaignRuns(t, final)
	if len(runs) != 1 || runs[0]["status"] != "failed" {
		t.Fatalf("runs = %v, want a single failed run", runs)
	}
}
