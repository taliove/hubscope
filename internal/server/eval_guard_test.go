package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// TestEvalTriggerConflict pins the cross-campaign mutex (GH #153): while a
// campaign is active (any run still pending/running), every trigger path —
// full sweep, single-suite — answers 409 instead of stacking a second cell
// pool on the Hub; once the batch settles the trigger opens again.
func TestEvalTriggerConflict(t *testing.T) {
	ts, stub, _ := setupAsyncEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "conflict-model")
	stub.resetCalls()

	// In-flight state observed: the first campaign is still running (its
	// cells blocked on the stub gate) while the second trigger arrives.
	stub.blockCalls()
	t.Cleanup(stub.release)
	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second full sweep during active campaign: expected 409, got %d", resp.StatusCode)
	}

	suiteID := suiteIDByKey(t, ts.URL, "mmlu")
	resp = doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{modelID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("single-suite trigger during active campaign: expected 409, got %d", resp.StatusCode)
	}

	// Drain (ticket 100): terminal status covers every tail write.
	stub.release()
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	// Settled: the trigger opens again. The accepted campaign runs to its
	// own terminal state before cleanup (drain).
	resp = doPost(t, ts.URL+"/api/evals", map[string]interface{}{})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("full sweep after settle: expected 202, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var second map[string]interface{}
	_ = json.Unmarshal(env.Data, &second)
	waitCampaignStatus(t, ts.URL, int64(second["id"].(float64)), store.CampaignStatusDone)
}

// TestWeeklyEvalSkipsWhenCampaignActive pins the scheduler half of the
// cross-campaign mutex (GH #153): the weekly worker does not stack a
// scheduled batch on top of an already-active campaign.
func TestWeeklyEvalSkipsWhenCampaignActive(t *testing.T) {
	db := openTempDB(t)
	srv := server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncEval(),
		server.WithSyncDiscovery(),
	)
	ts := httptest.NewServer(srv)
	stub := newEvalStubHub()
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	modelID := createEvalModel(t, ts.URL, stub.URL, "weekly-skip-model")
	stub.resetCalls()

	// Fixture: an already-active campaign (store-level seed, like the
	// isolation sweep's derived-data fixtures).
	if _, err := db.CreateCampaign("manual", []int64{modelID}, time.Now().UTC()); err != nil {
		t.Fatalf("seed active campaign: %v", err)
	}

	// A Sunday inside the early-morning window: the first tick fires
	// immediately, sees the active campaign and skips.
	clock := scheduler.NewFakeClock(time.Date(2026, 7, 19, 1, 30, 0, 0, time.UTC))
	startEvalWorker(t, db, srv, clock)
	parkEvalWorker(t, clock, 1)

	if got := len(listCampaigns(t, ts.URL)); got != 1 {
		t.Fatalf("campaigns after weekly tick with active campaign = %d, want 1 (scheduled batch skipped)", got)
	}
}
