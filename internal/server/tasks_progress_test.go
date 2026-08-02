package server_test

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

// evalRunOfTask resolves the run a task row links to via its detail.
func evalRunOfTask(t *testing.T, base string, runID int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, runID))
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode run %d detail: %v", runID, err)
	}
	var run map[string]interface{}
	if err := json.Unmarshal(env.Data, &run); err != nil {
		t.Fatalf("unmarshal run %d detail: %v", runID, err)
	}
	return run
}

// TestEvalRunTaskProgressAndCampaignLink pins the GH #156 task-center
// deep-link contract: every eval_run task carries its batch's campaign_id
// (the /eval?batch= deep link), and a RUNNING eval_run task additionally
// carries a 0~1 progress fraction — done (model, case) units over the
// batch's member models x the suite's enabled cases. Terminal eval_run
// tasks expose campaign_id but a null progress; non-eval tasks expose both
// as null.
func TestEvalRunTaskProgressAndCampaignLink(t *testing.T) {
	// Async eval: observes the mid-flight task list with the model frozen
	// after its first answer call; drained by releaseModel +
	// waitCampaignStatus(done).
	ts, stub, _ := setupAsyncEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	// Serial cell order (GH #26 pool at 1): case 1's result is durably
	// written before case 2's call is issued, so the freeze point below
	// pins exactly one done (model, case) unit.
	setEvalConcurrency(t, ts.URL, 1)
	stub.resetCalls()
	stub.blockModelAfter("smart-model", 1)
	t.Cleanup(func() { stub.releaseModel("smart-model") })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))

	// The second call being recorded proves the freeze point: the first
	// case settled, the second hangs on the stub gate.
	waitFor(t, "second eval call reaching the stub", func() bool {
		return stub.callTotal("smart-model") >= 2
	})

	items := taskItems(t, listTasks(t, ts.URL, "type=eval_run"))
	if len(items) == 0 {
		t.Fatal("no eval_run tasks listed at freeze point")
	}
	// GH #26: every suite's run (and its task) is created up front in
	// running status, so at the freeze point every eval_run task is
	// running — but only the first suite's run has results, the rest sit
	// at progress 0.
	var active map[string]interface{}
	for _, it := range items {
		if it["status"] != "running" {
			t.Errorf("eval_run task %v status = %v, want running at freeze point", it["id"], it["status"])
		}
		if it["campaign_id"] != float64(campaignID) {
			t.Errorf("eval_run task %v campaign_id = %v, want %d", it["id"], it["campaign_id"], campaignID)
		}
		progress, ok := it["progress"].(float64)
		if !ok {
			t.Fatalf("running eval_run task %v progress missing or not a number: %v", it["id"], it)
		}
		if progress > 0 {
			if active != nil {
				t.Fatalf("more than one run with done units at freeze point: %v", items)
			}
			active = it
		}
	}
	if active == nil {
		t.Fatalf("no run with done units at freeze point: %v", items)
	}

	// The executing task's progress: one done unit out of one member model x
	// the suite's enabled cases — strictly inside (0,1).
	run := evalRunOfTask(t, ts.URL, int64(active["entity_id"].(float64)))
	suiteID := int64(run["suite_id"].(float64))
	total := enabledCaseCount(t, ts.URL, suiteID)
	progress := active["progress"].(float64)
	if progress <= 0 || progress >= 1 {
		t.Errorf("mid-flight progress = %v, want within (0,1)", progress)
	}
	if want := 1.0 / float64(total); math.Abs(progress-want) > 1e-9 {
		t.Errorf("progress = %v, want 1/%d = %v (one done unit)", progress, total, want)
	}

	// Non-eval tasks (hub discovery syncs from the model setup) expose both
	// new fields as null.
	for _, it := range taskItems(t, listTasks(t, ts.URL, "type=discovery_sync")) {
		if it["progress"] != nil {
			t.Errorf("discovery_sync task %v progress = %v, want null", it["id"], it["progress"])
		}
		if it["campaign_id"] != nil {
			t.Errorf("discovery_sync task %v campaign_id = %v, want null", it["id"], it["campaign_id"])
		}
	}

	// After the batch settles, progress clears to null while campaign_id
	// keeps the deep link alive.
	stub.releaseModel("smart-model")
	waitCampaignStatus(t, ts.URL, campaignID, "done")
	for _, it := range taskItems(t, listTasks(t, ts.URL, "type=eval_run")) {
		if it["progress"] != nil {
			t.Errorf("settled eval_run task %v progress = %v, want null", it["id"], it["progress"])
		}
		if it["campaign_id"] != float64(campaignID) {
			t.Errorf("settled eval_run task %v campaign_id = %v, want %d", it["id"], it["campaign_id"], campaignID)
		}
	}
}
