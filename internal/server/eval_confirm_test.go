package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// setupConfirmEnv builds an async-eval server with the jury confirmation
// gate enabled on a FakeClock: the countdown advances only when the test
// advances it (W4, zero real waiting).
func setupConfirmEnv(t *testing.T) (*httptest.Server, *evalStubHub, *store.DB, *scheduler.FakeClock) {
	t.Helper()
	db := openTempDB(t)
	seedTestUser(t, db)
	stub := newEvalStubHub()
	clock := scheduler.NewFakeClock(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSyncDiscovery(),
		server.WithEvalClock(clock),
		server.WithJuryConfirmTimeout(60*time.Second),
	))
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
		db.Close()
	})
	return ts, stub, db, clock
}

// campaignIDOfLatest returns the newest campaign's ID.
func campaignIDOfLatest(t *testing.T, base string) int64 {
	t.Helper()
	campaigns := listCampaigns(t, base)
	if len(campaigns) == 0 {
		t.Fatal("no campaigns")
	}
	return int64(campaigns[0]["id"].(float64))
}

func confirmReportAwaiting(t *testing.T, base string, campaignID int64) bool {
	t.Helper()
	report := getCampaignReport(t, base, campaignID, "")
	v, _ := report["awaiting_confirmation"].(bool)
	return v
}

// TestJuryConfirmationGate pins the manual-batch gate (2026-08-04 ruling):
// after probing, the batch pauses with the plan visible; no case burns
// until the operator confirms, and the confirm releases it immediately.
func TestJuryConfirmationGate(t *testing.T) {
	ts, stub, _, clock := setupConfirmEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	stub.resetCalls()
	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{modelID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("trigger: expected 202, got %d", resp.StatusCode)
	}

	// The gate holds the batch: probing finished (3 rounds) but no case
	// call ever starts, and the report shows the awaiting flag.
	waitFor(t, "batch parked at the confirmation gate", func() bool {
		return confirmReportAwaiting(t, ts.URL, campaignIDOfLatest(t, ts.URL))
	})
	if got := stub.callTotal("smart-model"); got != 3 {
		t.Fatalf("calls at the gate = %d, want 3 (probe rounds only, no case burned)", got)
	}

	// Confirming releases the gate immediately.
	campaignID := campaignIDOfLatest(t, ts.URL)
	conf := doPost(t, ts.URL+"/api/campaigns/"+itoa(campaignID)+"/confirm-jury", map[string]interface{}{})
	conf.Body.Close()
	if conf.StatusCode != http.StatusOK {
		t.Fatalf("confirm-jury: expected 200, got %d", conf.StatusCode)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")
	if got := stub.callTotal("smart-model"); got <= 3 {
		t.Errorf("after confirm the cases must burn, calls = %d", got)
	}
	_ = clock
}

// TestJuryConfirmationAutoStart pins the timeout path: unconfirmed batches
// auto-start when the countdown lapses.
func TestJuryConfirmationAutoStart(t *testing.T) {
	ts, stub, _, clock := setupConfirmEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	stub.resetCalls()
	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{modelID},
	})
	resp.Body.Close()

	campaignID := campaignIDOfLatest(t, ts.URL)
	waitFor(t, "batch parked at the confirmation gate", func() bool {
		return confirmReportAwaiting(t, ts.URL, campaignID)
	})
	clock.Advance(61 * time.Second)
	waitCampaignStatus(t, ts.URL, campaignID, "done")
	if got := stub.callTotal("smart-model"); got <= 3 {
		t.Errorf("after the timeout the batch must auto-start, calls = %d", got)
	}
}

// TestConfirmJuryConflict covers the 409: confirming a campaign that is
// not parked at the gate.
func TestConfirmJuryConflict(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	runID := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, runID)
	campaignID := campaignIDOfRun(t, getEvalRun(t, ts.URL, runID))

	resp := doPost(t, ts.URL+"/api/campaigns/"+itoa(campaignID)+"/confirm-jury", map[string]interface{}{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("confirm on a settled campaign: expected 409, got %d", resp.StatusCode)
	}
}
