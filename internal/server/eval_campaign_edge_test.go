package server_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/scheduler"
	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// TestCampaignPartialFailureAggregatesFailed drives a weekly campaign where
// the first suite completes and the second is aborted mid-run: the campaign
// must settle to failed with progress showing exactly one done and one
// failed run, and the remaining suites must never get runs.
func TestCampaignPartialFailureAggregatesFailed(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "partial-campaign.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{}))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Several chat models widen the in-flight window of a suite run, so the
	// gate deterministically catches the second suite mid-flight.
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")
	createEvalModel(t, ts.URL, stub.URL, "chat-three")

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
		stub.release()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("eval worker did not stop within 10s of cancellation")
		}
	})

	// Step the campaign one stub call at a time: the gate is armed before
	// the campaign starts, and each release is immediately followed by
	// re-arming, so at most a call or two slips through per step. Polling
	// for "first suite done" with the gate open would race the whole
	// campaign settling inside one poll window (this made the test flaky).
	stub.blockCalls()
	stepUntil := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			stub.release()
			stub.blockCalls()
		}
		t.Fatalf("timed out stepping the campaign: %s", what)
	}

	var campaignID int64
	stepUntil("campaign created", func() bool {
		campaigns := listCampaigns(t, ts.URL)
		if len(campaigns) != 1 {
			return false
		}
		campaignID = int64(campaigns[0]["id"].(float64))
		return true
	})
	stepUntil("first suite done", func() bool {
		campaign := getCampaign(t, ts.URL, campaignID)
		progress := campaignProgress(t, campaign)
		return int(progress["done"].(float64)) >= 1
	})
	// The first suite being done does not mean the second run exists yet:
	// cancelling in that gap would end the campaign as done with a single
	// run. Step until the second run is created — the armed gate keeps it
	// frozen — then cancel: its context check fails the run deterministically.
	stepUntil("second suite run created", func() bool {
		campaign := getCampaign(t, ts.URL, campaignID)
		return len(campaignRuns(t, campaign)) == 2
	})

	cancel()
	stub.release()

	final := waitCampaignStatus(t, ts.URL, campaignID, "failed")
	progress := campaignProgress(t, final)
	if got := int(progress["done"].(float64)); got != 1 {
		t.Errorf("progress.done = %d, want 1 (first suite completed)", got)
	}
	if got := int(progress["failed"].(float64)); got != 1 {
		t.Errorf("progress.failed = %d, want 1 (second suite aborted)", got)
	}
	if got := int(progress["running"].(float64)); got != 0 {
		t.Errorf("progress.running = %d, want 0 after settling", got)
	}
	runs := campaignRuns(t, final)
	if len(runs) != 2 {
		t.Fatalf("campaign has %d runs, want 2 (remaining suites skipped)", len(runs))
	}
	if runs[0]["status"] != "done" || runs[1]["status"] != "failed" {
		t.Errorf("run statuses = %v/%v, want done/failed", runs[0]["status"], runs[1]["status"])
	}
	if final["finished_at"] == nil {
		t.Error("failed campaign must carry finished_at")
	}

	// The failed campaign's report (ticket 52): the settled board carries
	// per-suite cells — the completed suite done with full coverage, the
	// aborted suite failed for every model with zero judged cases but the
	// suite's planned case count as expected. The first two suites in the
	// rotation are cap_instruction (done) and cap_reasoning (aborted).
	report := getCampaignReport(t, ts.URL, campaignID, "")
	rows := reportRows(t, report)
	if len(rows) != 3 {
		t.Fatalf("failed campaign report rows = %v, want all three models", rows)
	}
	for _, row := range rows {
		assertCell(t, row, "cap_instruction", "done", 10, 10)
		assertCell(t, row, "cap_reasoning", "failed", 0, 10)
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
	ts := httptest.NewServer(server.New(db2, testAdminPassword, server.WithRateLimits(server.RateLimits{})))
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
