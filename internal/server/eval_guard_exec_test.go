package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// TestCampaignBudgetExceeded pins the campaign-level time budget (GH #153):
// once the batch outlives eval_campaign_budget_minutes (read on the
// injected clock), unstarted cells are dropped, their runs fail with the
// budget reason, and the campaign settles failed — a half-dead Hub can no
// longer hold a batch for hours.
func TestCampaignBudgetExceeded(t *testing.T) {
	db := openTempDB(t)
	clock := scheduler.NewFakeClock(time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithNow(clock.Now),
		server.WithSyncDiscovery(),
	))
	stub := newEvalStubHub()
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	createEvalModel(t, ts.URL, stub.URL, "budget-model")
	stub.resetCalls()

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"eval_campaign_budget_minutes": 60,
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put budget setting: expected 200, got %d", putResp.StatusCode)
	}

	// In-flight state observed: four cells (one suite each, pool of four)
	// blocked mid-flight when the budget lapses; the fifth must never start.
	// The count-based gate lets the probe stage's three rounds through
	// first (GH #174), then freezes the cell wave.
	stub.blockCallsAfter(3)
	t.Cleanup(stub.releaseGlobal)
	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	// The budget deadline is computed when execution starts; advancing the
	// clock before the pool is in flight would let the deadline slip past
	// the advance. Wait for the full wave of blocked calls first.
	waitFor(t, "four cells blocked in flight", func() bool {
		return stub.grandTotalCalls() >= 7
	})
	clock.Advance(61 * time.Minute)
	stub.releaseGlobal()

	// Drain (ticket 100): terminal status covers every tail write.
	final := waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusFailed, store.CampaignStatusDone)
	if final["status"] != store.CampaignStatusFailed {
		t.Fatalf("over-budget campaign status = %v, want failed (unstarted run must fail)", final["status"])
	}
	runs, _ := final["runs"].([]interface{})
	var done, failed int
	for _, r := range runs {
		switch r.(map[string]interface{})["status"] {
		case "done":
			done++
		case "failed":
			failed++
		}
	}
	if done == 0 || failed == 0 {
		t.Errorf("over-budget campaign runs: done=%d failed=%d, want both > 0 (in-flight finish, unstarted fail)", done, failed)
	}
}

// TestCircuitBreakerSkipsFailingModel pins the per-model circuit breaker
// (GH #153): after five consecutive cases whose answer calls all fail, the
// model's remaining cases are recorded unscored without further Hub calls —
// a broken model burns ten calls per run instead of two per case.
func TestCircuitBreakerSkipsFailingModel(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	brokenID := createEvalModel(t, ts.URL, stub.URL, "cb-broken")
	smartID := createEvalModel(t, ts.URL, stub.URL, "cb-smart")
	// Case-broken (GH #174): the model passes the probe gate and fails at
	// case time — the circuit breaker is the backstop behind the gate.
	stub.markCaseBroken("cb-broken", true)
	stub.resetCalls()

	suiteID := suiteIDByKey(t, ts.URL, "mmlu")
	caseCount := enabledCaseCount(t, ts.URL, suiteID)
	if caseCount < 6 {
		t.Fatalf("mmlu enabled cases = %d, need ≥ 6 to observe the circuit opening", caseCount)
	}

	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{brokenID, smartID},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("single-suite trigger: expected 202, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()

	// 5 consecutive failed cases x 2 attempts (one retry each), then the
	// circuit opens; the healthy model answers every case exactly once.
	// Both models also paid their three probe-gate rounds up front (GH #174).
	if got := stub.callTotal("cb-broken"); got != 13 {
		t.Errorf("broken model calls = %d, want 13 (3 probe rounds + 5 cases x 2 attempts, circuit open)", got)
	}
	if got := stub.callTotal("cb-smart"); got != caseCount+3 {
		t.Errorf("smart model calls = %d, want %d (3 probe rounds + one per case)", got, caseCount+3)
	}

	// The grid stays complete: every case has a row for the broken model,
	// the skipped ones named by the circuit reason.
	var campaign map[string]interface{}
	_ = json.Unmarshal(env.Data, &campaign)
	runs, _ := campaign["runs"].([]interface{})
	runID := int64(runs[0].(map[string]interface{})["id"].(float64))
	detailResp := doGet(t, fmt.Sprintf("%s/api/evals/%d", ts.URL, runID))
	var detailEnv envelope
	_ = json.NewDecoder(detailResp.Body).Decode(&detailEnv)
	detailResp.Body.Close()
	var detail map[string]interface{}
	_ = json.Unmarshal(detailEnv.Data, &detail)
	results, _ := detail["results"].([]interface{})
	var brokenRows, circuitRows int
	for _, r := range results {
		row := r.(map[string]interface{})
		if row["model_id"] != "cb-broken" {
			continue
		}
		brokenRows++
		if d, _ := row["verdict_detail"].(string); strings.Contains(d, "circuit open") {
			circuitRows++
		}
	}
	if brokenRows != caseCount {
		t.Errorf("broken model result rows = %d, want %d (complete grid)", brokenRows, caseCount)
	}
	if circuitRows != caseCount-5 {
		t.Errorf("circuit-skipped rows = %d, want %d (cases after the 5th consecutive failure)", circuitRows, caseCount-5)
	}
}

// TestCampaignAbortWhenAllCellsFail pins the campaign-level abort (GH #153):
// when the first three cells all come back dead, the batch is hopeless and
// unstarted cells are dropped — the campaign settles failed instead of
// burning every remaining case against a dead Hub. Under the GH #169
// model-major order the first wave is model 1's first four suite cells
// (pool capacity 4), so the abort lands before any run has all its models
// — every run fails and exactly four cells burn their circuit budget.
func TestCampaignAbortWhenAllCellsFail(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	for _, m := range []string{"abort-m1", "abort-m2", "abort-m3"} {
		createEvalModel(t, ts.URL, stub.URL, m)
		// Case-broken (GH #174): the gate admits them, every case fails —
		// the all-dead abort lives behind the probe gate.
		stub.markCaseBroken(m, true)
	}
	stub.resetCalls()

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	// Synchronous trigger: the response follows the terminal state.
	final := waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusFailed, store.CampaignStatusDone)
	if final["status"] != store.CampaignStatusFailed {
		t.Fatalf("all-dead campaign status = %v, want failed", final["status"])
	}
	runs, _ := final["runs"].([]interface{})
	var done, failed int
	for _, r := range runs {
		switch r.(map[string]interface{})["status"] {
		case "done":
			done++
		case "failed":
			failed++
		}
	}
	if done != 0 || failed != len(runs) {
		t.Errorf("all-dead campaign runs: done=%d failed=%d, want 0 done and all %d failed (the abort lands inside model 1's wave, so no run completes)", done, failed, len(runs))
	}

	// Without the abort every cell would burn its circuit budget: 15 cells
	// x 10 calls = 150. The abort drops everything past the first wave:
	// the three cells whose completion triggers it burn their full 10-call
	// budget, the fourth in-flight cell is cut short wherever the cancel
	// lands, and a worker that finishes before the third completion may
	// take one more cell — bounding the total at seven cells. The three
	// models' probe-gate rounds add 9 calls up front (GH #174).
	if got := stub.grandTotalCalls(); got < 39 || got > 79 {
		t.Errorf("total calls = %d, want within [39, 79] (9 probe rounds + three completed cells plus in-flight tails; abort dropped the rest)", got)
	}
}
