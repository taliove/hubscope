package server_test

import (
	"testing"
)

// TestEvalSkipsDownModelPreflight pins the pre-flight offline skip: a model
// whose enabled chat endpoints are all down on the status board is skipped
// before the first answer call — zero Hub calls, no dead result rows, one
// warn line in the task log — while a healthy, never-probed model in the
// same run scores as usual (unknown probe state never kills a cell).
func TestEvalSkipsDownModelPreflight(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	// Register while the hub is healthy (creation trial-probes), then break
	// the model: every completion call for it 503s from here on.
	downID := createEvalModel(t, ts.URL, stub.URL, "down-model")
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, ts.URL, suiteID, "PRE-A:请作答", "好的", nil)
	createRuleCase(t, ts.URL, suiteID, "PRE-B:请作答", "好的", nil)

	// Drive every enabled chat endpoint of down-model down through real
	// probe rounds: one failing round records two failed probes
	// (non-streaming + streaming), so two rounds cross the board's down
	// threshold of 3 consecutive failures. The endpoint detail API (the
	// board's own read path) must agree before the eval starts.
	stub.markBroken("down-model", true)
	endpoints, err := db.ListEndpointsByModelID(downID)
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	probed := 0
	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		probed++
		runProbeRound(t, ts, ep.ID)
		runProbeRound(t, ts, ep.ID)
		if d := fetchDetail(t, ts.URL, ep.ID); d.Status != "down" {
			t.Fatalf("endpoint %d status = %q, want down before eval", ep.ID, d.Status)
		}
	}
	if probed == 0 {
		t.Fatal("down-model has no enabled endpoints to drive down")
	}
	// The hub recovers before the eval (GH #174): the probe gate samples
	// the model live and admits it (3 probe calls), so the stale board's
	// down caliber stays the pre-flight skip's job — the two layers pin
	// exactly their own signals.
	stub.markBroken("down-model", false)
	stub.resetCalls()

	runID := triggerEval(t, ts.URL, suiteID, downID, smartID)
	run := waitEvalDone(t, ts.URL, runID)

	// The gate's three probe rounds are the only calls: the pre-flight
	// skip fires before the case loop, so no answer call ever happens.
	if got := stub.callTotal("down-model"); got != 3 {
		t.Errorf("down-model completion calls = %d, want 3 (probe gate rounds only, then pre-flight skip)", got)
	}
	// No dead rows are stamped for the skipped model (GH #154 precedent).
	if res := resultsByModel(run, "down-model"); len(res) != 0 {
		t.Errorf("down-model results = %d, want 0 (no dead rows): %v", len(res), res)
	}
	// The healthy model has no probe data at all (creation trials persist
	// none) and must score normally — unknown is not down.
	smartResults := resultsByModel(run, "smart-model")
	if len(smartResults) != 2 {
		t.Fatalf("smart-model results = %d, want 2", len(smartResults))
	}
	for _, r := range smartResults {
		if r["score"] != 1.0 {
			t.Errorf("smart-model case %v score = %v, want 1", r["case_id"], r["score"])
		}
	}

	// The task log names the skip and its pre-flight reason.
	task := waitTaskStatus(t, ts.URL, runID, "success")
	logs := taskLogs(t, getTaskDetail(t, ts.URL, int64(task["id"].(float64))))
	if got := countLogLines(logs, "warn", "model down-model skipped: endpoints down (pre-flight)"); got != 1 {
		t.Errorf("expected 1 pre-flight skip line, got %d (logs: %v)", got, logs)
	}
}

// TestEvalRunsFailingNotDownModel pins the boundary of the pre-flight skip:
// consecutive probe failures below the down threshold (status failing) must
// NOT skip the model — it still runs and scores.
func TestEvalRunsFailingNotDownModel(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "recover-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	retireSuiteCases(t, db, suiteID)
	createRuleCase(t, ts.URL, suiteID, "PRE-C:请作答", "好的", nil)

	// One failing round = 2 consecutive failures, below the down threshold
	// of 3: the board reads failing, not down.
	stub.markBroken("recover-model", true)
	endpoints, err := db.ListEndpointsByModelID(modelID)
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		runProbeRound(t, ts, ep.ID)
		if d := fetchDetail(t, ts.URL, ep.ID); d.Status != "failing" {
			t.Fatalf("endpoint %d status = %q, want failing before eval", ep.ID, d.Status)
		}
	}
	stub.markBroken("recover-model", false)
	stub.resetCalls()

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)

	results := resultsByModel(run, "recover-model")
	if len(results) != 1 {
		t.Fatalf("recover-model results = %d, want 1", len(results))
	}
	if results[0]["score"] != 1.0 {
		t.Errorf("recover-model score = %v, want 1", results[0]["score"])
	}
	if got := stub.callTotal("recover-model"); got != 4 {
		t.Errorf("recover-model completion calls = %d, want 4 (3 probe gate rounds + 1 case call, no skip)", got)
	}
	task := waitTaskStatus(t, ts.URL, runID, "success")
	logs := taskLogs(t, getTaskDetail(t, ts.URL, int64(task["id"].(float64))))
	if got := countLogLines(logs, "", "pre-flight"); got != 0 {
		t.Errorf("expected no pre-flight lines for a failing-not-down model, got %d (logs: %v)", got, logs)
	}
}
