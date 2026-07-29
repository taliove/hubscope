package server_test

import (
	"net/http/httptest"
	"testing"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// openSuitesServer opens a fresh server on the given database path. Used by
// gen-3 seed tests that reopen the same database to prove idempotency.
func openSuitesServer(t *testing.T, dbPath string) (*httptest.Server, *store.DB) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	seedTestUser(t, db)
	ts := httptest.NewServer(server.New(db, server.WithRateLimits(server.RateLimits{})))
	t.Cleanup(ts.Close)
	return ts, db
}

// TestBenchmarkSweepCoversRotation asserts a full sweep runs exactly the
// five authoritative-benchmark suites of the post-cutover rotation (ticket
// 99) — never a retired v3 or legacy suite.
func TestBenchmarkSweepCoversRotation(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")

	campaign := triggerFullSweep(t, ts.URL)
	final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), "done", "failed")
	if final["status"] != "done" {
		t.Fatalf("sweep campaign status = %v, want done", final["status"])
	}

	want := map[string]bool{}
	for _, key := range benchmarkRotation {
		want[key] = false
	}
	for _, run := range campaignRuns(t, final) {
		suite := suiteByID(t, ts.URL, int64(run["suite_id"].(float64)))
		key := suite["key"].(string)
		if _, ok := want[key]; !ok {
			t.Errorf("sweep ran retired/unexpected suite %q", key)
			continue
		}
		want[key] = true
	}
	for key, ran := range want {
		if !ran {
			t.Errorf("sweep did not run benchmark suite %q", key)
		}
	}
}

// suiteByID fetches GET /api/suites and returns the suite with the given id.
func suiteByID(t *testing.T, base string, id int64) map[string]interface{} {
	t.Helper()
	for _, s := range fetchSuites(t, base, "") {
		if int64(s["id"].(float64)) == id {
			return s
		}
	}
	t.Fatalf("suite id %d not found", id)
	return nil
}

// TestBenchmarkSuiteRotation covers the rotation contract on a benchmark
// suite (W7): retiring a case and minting its replacement bumps the suite
// version, the new run executes the replacement, and the historical run
// keeps rendering the retired case's original prompt.
func TestBenchmarkSuiteRotation(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	before := suiteByKey(t, ts.URL, "gsm8k")
	target := before["cases"].([]interface{})[0].(map[string]interface{})
	targetID := int64(target["id"].(float64))
	oldPrompt := target["prompt"].(string)

	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	if v := getEvalRun(t, ts.URL, run1)["suite_version"]; v != 1.0 {
		t.Fatalf("run1 suite_version = %v, want 1", v)
	}

	// Rotate: retire the case, mint the replacement.
	replacement := patchCase(t, ts.URL, targetID, map[string]interface{}{
		"prompt": "10 加 5 等于几？只回复数字",
	})
	newID := int64(replacement["id"].(float64))
	if v := suiteByKey(t, ts.URL, "gsm8k")["version"]; v != 2.0 {
		t.Fatalf("after rotation version = %v, want 2", v)
	}

	// History: run1 still references the retired case, whose row keeps the
	// original prompt, so the old report renders the old question.
	run1Detail := getEvalRun(t, ts.URL, run1)
	var sawOld bool
	for _, r := range resultsByModel(run1Detail, "smart-model") {
		if int64(r["case_id"].(float64)) == targetID {
			sawOld = true
		}
	}
	if !sawOld {
		t.Error("run1 should still reference the retired case id")
	}
	oldRow := caseByID(t, suiteByKey(t, ts.URL, "gsm8k"), targetID)
	if oldRow["prompt"] != oldPrompt || oldRow["enabled"] != false {
		t.Errorf("retired case lost its original prompt: %v", oldRow)
	}

	// The next run records version 2 and scores the replacement.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	if v := getEvalRun(t, ts.URL, run2)["suite_version"]; v != 2.0 {
		t.Errorf("run2 suite_version = %v, want 2", v)
	}
	var scoredNew, sawRetired bool
	for _, r := range resultsByModel(getEvalRun(t, ts.URL, run2), "smart-model") {
		switch int64(r["case_id"].(float64)) {
		case newID:
			scoredNew = r["score"] != nil
		case targetID:
			sawRetired = true
		}
	}
	if sawRetired {
		t.Error("run2 should not execute the retired case")
	}
	if !scoredNew {
		t.Error("run2 should score the replacement case")
	}
}

// TestBenchmarkNadirSnapshot asserts a run snapshots its suite's nadir at
// creation, read back through the eval API: the four-option MCQ suites
// record the 0.25 random-guess floor, an open-ended suite records 0 (ADR
// 0009, ticket 99 calibration).
func TestBenchmarkNadirSnapshot(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")

	knowledgeID := suiteIDByKey(t, ts.URL, "mmlu")
	runID := triggerEval(t, ts.URL, knowledgeID, modelID)
	waitEvalDone(t, ts.URL, runID)
	if nadir := getEvalRun(t, ts.URL, runID)["nadir"]; nadir != 0.25 {
		t.Errorf("mmlu run nadir = %v, want 0.25", nadir)
	}

	// An open-ended suite snapshots nadir 0 (the raw-mean caliber).
	reasoningID := suiteIDByKey(t, ts.URL, "gsm8k")
	reasoningRunID := triggerEval(t, ts.URL, reasoningID, modelID)
	waitEvalDone(t, ts.URL, reasoningRunID)
	if nadir := getEvalRun(t, ts.URL, reasoningRunID)["nadir"]; nadir != 0.0 {
		t.Errorf("gsm8k run nadir = %v, want 0", nadir)
	}
}
