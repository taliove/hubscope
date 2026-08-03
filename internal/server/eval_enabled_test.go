package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// patchEvalEnabled flips a model's "join evaluations" switch (GH #170) via
// PATCH /api/models/{id} and asserts the response DTO reflects the new
// state.
func patchEvalEnabled(t *testing.T, base string, modelID int64, enabled bool) {
	t.Helper()
	resp := doPatch(t, fmt.Sprintf("%s/api/models/%d", base, modelID), map[string]interface{}{
		"eval_enabled": enabled,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PATCH eval_enabled=%v: expected 200, got %d: %s", enabled, resp.StatusCode, b)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var model map[string]interface{}
	_ = json.Unmarshal(env.Data, &model)
	if got, ok := model["eval_enabled"].(bool); !ok || got != enabled {
		t.Fatalf("PATCH response eval_enabled = %v, want %v", model["eval_enabled"], enabled)
	}
}

// getModelDTO reads GET /api/models and returns the DTO of one model.
func getModelDTO(t *testing.T, base string, modelID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/models")
	defer resp.Body.Close()
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)
	for _, m := range models {
		if int64(m["id"].(float64)) == modelID {
			return m
		}
	}
	t.Fatalf("model %d not in GET /api/models", modelID)
	return nil
}

// TestPatchModelEvalEnabledRoundTrip pins the PATCH ↔ DTO contract of the
// GH #170 switch: models default to eval-enabled, an eval_enabled-only
// PATCH flips the switch without touching capability, and GET /api/models
// exposes the persisted value.
func TestPatchModelEvalEnabledRoundTrip(t *testing.T) {
	db := openTempDB(t)
	stub := newEvalStubHub()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
	))
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	modelID := createEvalModel(t, ts.URL, stub.URL, "switch-model")

	// New models join evaluations by default (GH #170: default on).
	if got := getModelDTO(t, ts.URL, modelID)["eval_enabled"]; got != true {
		t.Fatalf("new model eval_enabled = %v, want true (default on)", got)
	}

	// Off: an eval_enabled-only PATCH must not touch the capability.
	resp := doPatch(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID), map[string]interface{}{
		"eval_enabled": false,
	})
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH eval_enabled=false: expected 200, got %d", resp.StatusCode)
	}
	var patched map[string]interface{}
	_ = json.Unmarshal(env.Data, &patched)
	if got := patched["capability"]; got != "chat" {
		t.Errorf("eval_enabled-only PATCH changed capability to %v, want chat", got)
	}
	if got := getModelDTO(t, ts.URL, modelID)["eval_enabled"]; got != false {
		t.Errorf("GET after opt-out: eval_enabled = %v, want false", got)
	}

	// On again: the switch is reversible.
	patchEvalEnabled(t, ts.URL, modelID, true)
	if got := getModelDTO(t, ts.URL, modelID)["eval_enabled"]; got != true {
		t.Errorf("GET after opt-in: eval_enabled = %v, want true", got)
	}
}

// TestEvalEnabledExcludesFromFullSweep pins the sweep half of GH #170: a
// model whose switch is off is not in the full-sweep candidate set — the
// Hub never sees a call for it — while enabled models are swept as before.
func TestEvalEnabledExcludesFromFullSweep(t *testing.T) {
	db := openTempDB(t)
	stub := newEvalStubHub()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
	))
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	createEvalModel(t, ts.URL, stub.URL, "sweep-on-model")
	offID := createEvalModel(t, ts.URL, stub.URL, "sweep-off-model")
	patchEvalEnabled(t, ts.URL, offID, false)
	stub.resetCalls()

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	final := waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone, store.CampaignStatusFailed)
	if final["status"] != store.CampaignStatusDone {
		t.Fatalf("sweep campaign status = %v, want done", final["status"])
	}
	if !stub.sawModel("sweep-on-model") {
		t.Error("sweep never called the eval-enabled model")
	}
	if stub.sawModel("sweep-off-model") {
		t.Error("sweep called the eval-disabled model — the switch must shrink the candidate set")
	}
}

// TestEvalEnabledExcludesFromWeeklyBatch pins the scheduler half of GH
// #170: the Sunday batch draws from the same candidate source as the sweep,
// so an opted-out model gets zero Hub calls there too.
func TestEvalEnabledExcludesFromWeeklyBatch(t *testing.T) {
	db := openTempDB(t)
	stub := newEvalStubHub()
	srv := server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
	)
	ts := httptest.NewServer(srv)
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	createEvalModel(t, ts.URL, stub.URL, "weekly-on-model")
	offID := createEvalModel(t, ts.URL, stub.URL, "weekly-off-model")
	patchEvalEnabled(t, ts.URL, offID, false)
	stub.resetCalls()

	clock := scheduler.NewFakeClock(time.Date(2026, 8, 1, 23, 30, 0, 0, time.UTC)) // Saturday
	startEvalWorker(t, db, srv, clock)
	suites := suiteCount(t, ts.URL)
	parkEvalWorker(t, clock, 1)

	// Sunday 01:30: the weekly batch fires.
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

	if !stub.sawModel("weekly-on-model") {
		t.Error("weekly batch never called the eval-enabled model")
	}
	if stub.sawModel("weekly-off-model") {
		t.Error("weekly batch called the eval-disabled model — the switch must shrink the candidate set")
	}
}

// TestManualTriggerOverridesEvalEnabled pins the explicit-override rule of
// GH #170: a manual single-suite trigger that names an opted-out model
// still executes it — the switch only narrows automatic batches.
func TestManualTriggerOverridesEvalEnabled(t *testing.T) {
	db := openTempDB(t)
	stub := newEvalStubHub()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
		server.WithSyncEval(),
	))
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	offID := createEvalModel(t, ts.URL, stub.URL, "manual-override-model")
	patchEvalEnabled(t, ts.URL, offID, false)
	stub.resetCalls()

	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	runID := triggerEval(t, ts.URL, suiteID, offID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("manual run over opted-out model status = %v, want done", run["status"])
	}
	if !stub.sawModel("manual-override-model") {
		t.Error("manual trigger named the opted-out model explicitly but it was never called")
	}
}

// TestEvalEnabledSwitchDoesNotAffectRunningBatch pins the snapshot
// semantics of GH #170: campaign members are fixed at creation, so flipping
// the switch mid-flight does not evict the model from the running batch.
func TestEvalEnabledSwitchDoesNotAffectRunningBatch(t *testing.T) {
	db := openTempDB(t)
	stub := newEvalStubHub()
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSyncDiscovery(),
	))
	t.Cleanup(func() {
		ts.Close()
		stub.Close()
	})

	createEvalModel(t, ts.URL, stub.URL, "snap-a-model")
	bID := createEvalModel(t, ts.URL, stub.URL, "snap-b-model")
	stub.resetCalls()

	// Freeze the batch mid-flight, flip the switch, then let it drain.
	stub.blockCalls()
	t.Cleanup(stub.release)
	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitFor(t, "cells blocked in flight", func() bool {
		return stub.grandTotalCalls() >= 2
	})

	patchEvalEnabled(t, ts.URL, bID, false)
	stub.release()

	final := waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone, store.CampaignStatusFailed)
	if final["status"] != store.CampaignStatusDone {
		t.Fatalf("snapshot campaign status = %v, want done", final["status"])
	}
	if !stub.sawModel("snap-b-model") {
		t.Error("model opted out mid-flight was evicted from the running batch — members must be a creation-time snapshot")
	}
	if !stub.sawModel("snap-a-model") {
		t.Error("enabled model missing from the running batch")
	}
}
