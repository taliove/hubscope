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

// setEvalConcurrency writes the eval_concurrency setting through the API.
// Tests whose stub-gate scenarios pin the pre-GH-#26 serial execution order
// (suite by suite, model by model) set 1 so cell scheduling stays
// deterministic; the concurrency behavior itself is pinned by the tests in
// this file.
func setEvalConcurrency(t *testing.T, base string, n int) {
	t.Helper()
	resp := doPut(t, base+"/api/settings", map[string]interface{}{
		"eval_concurrency": n,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put eval_concurrency=%d: expected 200, got %d", n, resp.StatusCode)
	}
}

// TestEvalConcurrencySettingRoundTrip covers the eval_concurrency settings
// key (GH #26): default 4, partial PUT updates, out-of-range rejection —
// the same shape as default_sample_count.
func TestEvalConcurrencySettingRoundTrip(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	get := func() map[string]interface{} {
		resp := doGet(t, ts.URL+"/api/settings")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get settings: expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		var settings map[string]interface{}
		if err := json.Unmarshal(env.Data, &settings); err != nil {
			t.Fatalf("unmarshal settings: %v", err)
		}
		return settings
	}

	if got := get()["eval_concurrency"].(float64); got != 4 {
		t.Errorf("default eval_concurrency = %v, want 4", got)
	}

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"eval_concurrency": 8,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put eval_concurrency: expected 200, got %d", putResp.StatusCode)
	}
	if got := get()["eval_concurrency"].(float64); got != 8 {
		t.Errorf("eval_concurrency after PUT = %v, want 8", got)
	}

	// Out-of-range values are rejected, like default_sample_count.
	for _, bad := range []int{0, 17} {
		badResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
			"eval_concurrency": bad,
		})
		badResp.Body.Close()
		if badResp.StatusCode != http.StatusBadRequest {
			t.Errorf("eval_concurrency=%d: expected 400, got %d", bad, badResp.StatusCode)
		}
	}
	if got := get()["eval_concurrency"].(float64); got != 8 {
		t.Errorf("rejected PUT must not change eval_concurrency, got %v", got)
	}
}

// TestEvalConcurrencyBoundedPool asserts the (suite × model) worker pool
// (GH #26): with eval_concurrency=2 and two models, two completion calls are
// in flight simultaneously (proving fan-out) and the peak never exceeds the
// configured bound. The stub's gate holds every response, so the observed
// in-flight count equals the number of cells executing at once.
func TestEvalConcurrencyBoundedPool(t *testing.T) {
	// Async eval: observes the in-flight concurrency peak mid-sweep (all
	// calls blocked on the gate); drained by stub.release +
	// waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"eval_concurrency": 2,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put eval_concurrency: expected 200, got %d", putResp.StatusCode)
	}

	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	waitFor(t, "two concurrent eval calls in flight", func() bool {
		return stub.inflight() >= 2
	})
	if peak := stub.peakInflight(); peak != 2 {
		t.Errorf("peak in-flight eval calls = %d, want exactly 2 (eval_concurrency=2)", peak)
	}

	stub.release()
	final := waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")
	if final["status"] != "done" {
		t.Fatalf("campaign status = %v, want done", final["status"])
	}
	if peak := stub.peakInflight(); peak > 2 {
		t.Errorf("peak in-flight eval calls = %d after release, must stay ≤ 2", peak)
	}
}

// TestEvalConcurrencyOneRunsSerially pins the low end of the clamp: with
// eval_concurrency=1 the pool degenerates to the old serial cadence — the
// in-flight peak never passes 1.
func TestEvalConcurrencyOneRunsSerially(t *testing.T) {
	// Async eval: observes the in-flight peak mid-sweep under concurrency 1;
	// drained by stub.release + waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"eval_concurrency": 1,
	})
	putResp.Body.Close()

	stub.resetCalls()
	stub.blockCalls()
	t.Cleanup(stub.release)

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	waitFor(t, "first eval call reaching the stub", func() bool {
		return stub.inflight() >= 1
	})
	if peak := stub.peakInflight(); peak != 1 {
		t.Errorf("peak in-flight eval calls = %d, want 1 (eval_concurrency=1)", peak)
	}

	stub.release()
	waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")
	if peak := stub.peakInflight(); peak > 1 {
		t.Errorf("peak in-flight eval calls = %d after release, must stay ≤ 1", peak)
	}
}

// TestCancelStopsNewCells pins the pool's cancellation guarantee (GH #26):
// once the context is canceled, no worker takes a new cell. The freeze
// construction is deterministic — smart-model's last mmlu call is
// blocked mid-flight, the cancel lands while the worker is still inside that
// cell, so the next loop iteration provably starts with a canceled context.
// The observable of a stolen cell is not the (ctx-aborted, possibly unsent)
// answer call but its persisted rows: a stolen chat-two cell would record
// null results for every case, so chat-two must have zero result rows.
func TestCancelStopsNewCells(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "cancel-cells.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	// Eval runs go through the weekly worker (cancelable ctx); drain = cancel
	// + wait for worker.Run. Discovery stays inline.
	srv := server.New(db, server.WithRateLimits(server.RateLimits{}), server.WithSyncDiscovery())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "chat-two")
	setEvalConcurrency(t, ts.URL, 1)

	stub.resetCalls()
	// Freeze the last call of smart-model's first cell: with the pool at 1,
	// chat-two's cell is next in line, and the blocked call guarantees the
	// cancel lands before the worker could take it. The first run covers
	// mmlu (first suite in bank order), so the cell costs exactly its
	// enabled-case count of calls.
	firstCellCalls := enabledCaseCount(t, ts.URL, suiteIDByKey(t, ts.URL, "mmlu"))
	stub.blockModelAfter("smart-model", firstCellCalls-1)
	t.Cleanup(func() { stub.releaseModel("smart-model") })

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
		stub.releaseModel("smart-model")
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("eval worker did not stop within 10s of cancellation")
		}
	})

	waitFor(t, "smart-model's last mmlu call reaching the stub", func() bool {
		return stub.callTotal("smart-model") >= firstCellCalls
	})
	cancel()
	stub.releaseModel("smart-model")

	waitFor(t, "campaign settling after cancellation", func() bool {
		campaigns := listCampaigns(t, ts.URL)
		return len(campaigns) == 1 &&
			(campaigns[0]["status"] == "done" || campaigns[0]["status"] == "failed")
	})
	campaigns := listCampaigns(t, ts.URL)
	campaignID := int64(campaigns[0]["id"].(float64))
	if got := stub.callTotal("chat-two"); got != 0 {
		t.Errorf("chat-two calls = %d, want 0 (a canceled pool must not take new cells)", got)
	}

	// A stolen chat-two cell would have persisted null results for every
	// case of the first run; chat-two must not appear there at all.
	campaign := getCampaign(t, ts.URL, campaignID)
	runs := campaignRuns(t, campaign)
	if len(runs) == 0 {
		t.Fatalf("campaign has no runs: %v", campaign)
	}
	firstRunID := int64(runs[0]["id"].(float64))
	detail := runDetail(t, ts.URL, firstRunID)
	if got := resultsByModel(detail, "chat-two"); len(got) != 0 {
		t.Errorf("chat-two has %d result rows in run %d, want 0 (stolen cell)", len(got), firstRunID)
	}
	if got := resultsByModel(detail, "smart-model"); len(got) != firstCellCalls {
		t.Errorf("smart-model has %d result rows in run %d, want its %d recorded cases", len(got), firstRunID, firstCellCalls)
	}
}
