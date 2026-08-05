package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// probeGateRunLogContains reports whether the run's task log carries a
// probe-gate skip line for the model.
func probeGateRunLogContains(t *testing.T, base string, runID int64, want string) bool {
	t.Helper()
	items := taskItems(t, listTasks(t, base, "type=eval_run"))
	for _, item := range items {
		if int64(item["entity_id"].(float64)) != runID {
			continue
		}
		detail := getTaskDetail(t, base, int64(item["id"].(float64)))
		for _, line := range taskLogs(t, detail) {
			msg, _ := line["message"].(string)
			if strings.Contains(msg, want) {
				return true
			}
		}
	}
	return false
}

// TestProbeGateSkipsUnreachableModel pins the spec-0020 gate (GH #174): a
// model that fails every probe round is excluded from the batch — exactly
// three probe calls, zero case calls, zero result rows (no dead rows, the
// GH #154 form) — while reachable models evaluate normally; the run's task
// log names the skipped model.
func TestProbeGateSkipsUnreachableModel(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smart := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	broken := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	stub.resetCalls()
	runID := triggerEval(t, ts.URL, suiteID, smart, broken)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run with one reachable model should be done, got %v", run["status"])
	}

	if got := stub.callTotal("broken-model"); got != 3 {
		t.Errorf("broken model should see exactly 3 probe calls and no case calls, got %d", got)
	}
	if res := resultsByModel(run, "broken-model"); len(res) != 0 {
		t.Errorf("gated model must produce no result rows, got %d", len(res))
	}
	if res := resultsByModel(run, "smart-model"); len(res) == 0 {
		t.Error("reachable model should have results")
	}
	if !probeGateRunLogContains(t, ts.URL, runID, "unreachable at probe gate") {
		t.Error("run task log must name the probe-gated model")
	}
}

// TestProbeGateAllUnreachableFailsCampaign pins the all-dead gate outcome
// (GH #174): when every selected model is unreachable the run settles
// failed with the gate reason — no cases burned — instead of an empty
// success.
func TestProbeGateAllUnreachableFailsCampaign(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	broken := createEvalModel(t, ts.URL, stub.URL, "broken-model")
	stub.markBroken("broken-model", true)
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	stub.resetCalls()
	// triggerEval asserts a running/done fresh status; an all-gated run
	// settles failed synchronously, so trigger by hand.
	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{broken},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/evals: expected 202, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var campaign map[string]interface{}
	_ = json.Unmarshal(env.Data, &campaign)
	runID := int64(campaign["runs"].([]interface{})[0].(map[string]interface{})["id"].(float64))

	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "failed" {
		t.Fatalf("all-unreachable run should settle failed, got %v", run["status"])
	}
	if got := stub.callTotal("broken-model"); got != 3 {
		t.Errorf("broken model should see exactly 3 probe calls, got %d", got)
	}
	if res := resultsByModel(run, "broken-model"); len(res) != 0 {
		t.Errorf("gated run must produce no result rows, got %d", len(res))
	}
	if !probeGateRunLogContains(t, ts.URL, runID, "probe gate: every selected model unreachable") {
		t.Error("run task log must carry the all-unreachable gate reason")
	}
}

// TestProbeGateDoesNotTouchProbesTable pins the W5 isolation: eval probe
// rounds are not monitoring — they never write the probes table, so the
// status machine and alerting stay blind to them.
func TestProbeGateDoesNotTouchProbesTable(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	model := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	probeRows := func() int {
		endpoints, err := db.ListEndpointsByModelID(model)
		if err != nil {
			t.Fatalf("list endpoints: %v", err)
		}
		total := 0
		for _, ep := range endpoints {
			rows, err := db.ListProbes(ep.ID, 100, nil)
			if err != nil {
				t.Fatalf("list probes: %v", err)
			}
			total += len(rows)
		}
		return total
	}

	before := probeRows()
	runID := triggerEval(t, ts.URL, suiteID, model)
	waitEvalDone(t, ts.URL, runID)
	if after := probeRows(); after != before {
		t.Errorf("eval probe gate must not write probe records: before=%d after=%d", before, after)
	}
	// And the gate did actually probe (the assertion above is vacuous
	// otherwise): three calls against the stub.
	if got := stub.callTotal("smart-model"); got < 3 {
		t.Errorf("reachable model should see its 3 probe rounds plus cases, got %d total calls", got)
	}
}
