package server_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// TestCancelCampaign pins the operator cancel path (GH #152): a running
// batch can be stopped on demand — in-flight cells run to completion,
// unstarted cells are dropped, their runs fail and the campaign settles
// failed, releasing the cross-campaign mutex so a fresh trigger is
// accepted. Canceling a settled or unknown campaign is a conflict / 404,
// and the endpoint stays session-gated and hub-isolated.
func TestCancelCampaign(t *testing.T) {
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "cancel-model")
	stub.resetCalls()

	// Settled and unknown campaigns reject the cancel up front.
	settled := triggerFullSweep(t, ts.URL)
	settledID := int64(settled["id"].(float64))
	waitCampaignStatus(t, ts.URL, settledID, store.CampaignStatusDone)
	settledPath := "/api/campaigns/" + strconv.FormatInt(settledID, 10) + "/cancel"
	resp := doPost(t, ts.URL+settledPath, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("cancel settled campaign: expected 409, got %d", resp.StatusCode)
	}
	resp = doPost(t, ts.URL+"/api/campaigns/99999/cancel", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("cancel unknown campaign: expected 404, got %d", resp.StatusCode)
	}

	// In-flight state observed: cells blocked on the stub gate when the
	// cancel lands; the batch must drop every unstarted cell.
	stub.blockCalls()
	t.Cleanup(stub.release)
	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	cancelPath := "/api/campaigns/" + strconv.FormatInt(campaignID, 10) + "/cancel"
	waitFor(t, "cells blocked in flight", func() bool {
		return stub.grandTotalCalls() >= 1
	})

	resp = doPost(t, ts.URL+cancelPath, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("cancel running campaign: expected 202, got %d", resp.StatusCode)
	}
	// A second cancel while the first is still unwinding is a conflict.
	resp = doPost(t, ts.URL+cancelPath, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("double cancel: expected 409, got %d", resp.StatusCode)
	}

	// Drain (ticket 100): terminal status covers every tail write.
	stub.release()
	final := waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusFailed, store.CampaignStatusDone)
	if final["status"] != store.CampaignStatusFailed {
		t.Fatalf("canceled campaign status = %v, want failed", final["status"])
	}

	// The mutex released with the settle: a fresh sweep is accepted.
	resp = doPost(t, ts.URL+"/api/evals", map[string]interface{}{})
	if resp.StatusCode != http.StatusAccepted {
		resp.Body.Close()
		t.Fatalf("sweep after cancel-settle: expected 202, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var third map[string]interface{}
	_ = json.Unmarshal(env.Data, &third)
	waitCampaignStatus(t, ts.URL, int64(third["id"].(float64)), store.CampaignStatusDone)
}

// TestCancelCampaignAnonymous pins the session gate on the cancel endpoint.
func TestCancelCampaignAnonymous(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	req, err := http.NewRequest("POST", ts.URL+"/api/campaigns/1/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous cancel: expected 401, got %d", resp.StatusCode)
	}
}
