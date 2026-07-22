package server_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// openSuitesServer opens a fresh server on the given database path. Used by
// gen-3 seed tests that reopen the same database to prove idempotency.
func openSuitesServer(t *testing.T, dbPath string) (*httptest.Server, *store.DB) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ts := httptest.NewServer(server.New(db, testAdminPassword, server.WithRateLimits(server.RateLimits{})))
	t.Cleanup(ts.Close)
	return ts, db
}

// TestSeedGen3Idempotent reopens a seeded database and asserts the gen-3
// seed never duplicates cases, never reverts admin case edits, and never
// re-applies the legacy-suite retirement once it has been recorded.
func TestSeedGen3Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gen3.db")

	// First boot: seed lands. An admin then curates the bank — a content edit
	// (retire + mint) and one extra case on cap_reasoning — and deliberately
	// re-enables the retired legacy "basic" suite.
	ts, db := openSuitesServer(t, dbPath)
	before := suiteByKey(t, ts.URL, "cap_reasoning")
	if got := len(before["cases"].([]interface{})); got != 10 {
		t.Fatalf("cap_reasoning first-issue cases = %v, want 10", got)
	}
	seed := before["cases"].([]interface{})[0].(map[string]interface{})
	seedID := int64(seed["id"].(float64))
	patchCase(t, ts.URL, seedID, map[string]interface{}{"prompt": "1 加 1 等于几？只回复数字"})
	createResp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     int64(before["id"].(float64)),
		"prompt":       "管理员补充题：2 加 2 等于几？只回复数字",
		"verdict_type": "rule",
		"rule_config":  map[string]string{"mode": "exact", "expected": "4"},
	})
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create admin case: expected 201, got %d", createResp.StatusCode)
	}
	legacy := suiteByKey(t, ts.URL, "basic")
	if err := db.SetSuiteEnabled(int64(legacy["id"].(float64)), true); err != nil {
		t.Fatalf("re-enable legacy suite: %v", err)
	}
	db.Close()

	// Second boot: nothing is re-seeded, edited or re-retired.
	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()

	after := suiteByKey(t, ts2.URL, "cap_reasoning")
	cases := after["cases"].([]interface{})
	if got := len(cases); got != 12 {
		t.Errorf("cap_reasoning cases after reopen = %d, want 12 (no seed duplicates)", got)
	}
	if v := after["version"]; v != 3.0 {
		t.Errorf("cap_reasoning version after reopen = %v, want 3 (seed must not rebump)", v)
	}
	retired := caseByID(t, after, seedID)
	if retired["enabled"] != false || retired["prompt"] != seed["prompt"] {
		t.Errorf("admin edit reverted by reseed: %v", retired)
	}

	legacyAfter := suiteByKey(t, ts2.URL, "basic")
	if legacyAfter["enabled"] != true {
		t.Errorf("legacy suite re-retired on reopen; admin re-enable must stick: %v", legacyAfter["enabled"])
	}

	// Third boot for good measure: generation records keep it a no-op.
	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	again := suiteByKey(t, ts3.URL, "cap_reasoning")
	if got := len(again["cases"].([]interface{})); got != 12 {
		t.Errorf("cap_reasoning cases after third boot = %d, want still 12", got)
	}
}

// TestSeedGen3SweepExcludesRetired asserts a full sweep runs exactly the five
// capability suites and never the retired legacy ones.
func TestSeedGen3SweepExcludesRetired(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")

	campaign := triggerFullSweep(t, ts.URL)
	final := waitCampaignStatus(t, ts.URL, int64(campaign["id"].(float64)), "done", "failed")
	if final["status"] != "done" {
		t.Fatalf("sweep campaign status = %v, want done", final["status"])
	}

	want := map[string]bool{
		"cap_instruction": false, "cap_reasoning": false, "cap_coding": false,
		"cap_knowledge": false, "cap_language": false,
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
			t.Errorf("sweep did not run capability suite %q", key)
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

// TestSeedGen3Rotation covers the rotation contract on a capability suite
// (ADR 0010): retiring a case and minting its replacement bumps the suite
// version, the new run executes the replacement, and the historical run
// keeps rendering the retired case's original prompt.
func TestSeedGen3Rotation(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "cap_reasoning")

	before := suiteByKey(t, ts.URL, "cap_reasoning")
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
	if v := suiteByKey(t, ts.URL, "cap_reasoning")["version"]; v != 2.0 {
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
	oldRow := caseByID(t, suiteByKey(t, ts.URL, "cap_reasoning"), targetID)
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

// TestSeedGen3JudgeSampling runs the seeded language suite and asserts its
// judge cases are each answered three times with the score averaged over the
// samples, while rule cases are answered exactly once (ADR 0010).
func TestSeedGen3JudgeSampling(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "sample-model")

	suite := suiteByKey(t, ts.URL, "cap_language")
	suiteID := int64(suite["id"].(float64))
	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	byCase := map[int64]map[string]interface{}{}
	for _, r := range resultsByModel(run, "sample-model") {
		byCase[int64(r["case_id"].(float64))] = r
	}

	judgeCases, ruleCases := 0, 0
	for _, c := range suite["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		prompt := cm["prompt"].(string)
		res := byCase[int64(cm["id"].(float64))]
		if res == nil {
			t.Fatalf("seed case %v missing from results", cm["id"])
		}
		calls := stub.callCount("sample-model", prompt)
		if cm["verdict_type"] == "judge" {
			judgeCases++
			if calls != 3 {
				t.Errorf("judge case answered %d times, want 3 (sample_count=3)", calls)
			}
			// Three samples at the stub's default 0.75 verdict average to 0.75.
			if res["score"] != 0.75 {
				t.Errorf("judge case score = %v, want 0.75 (mean of sampled verdicts)", res["score"])
			}
		} else {
			ruleCases++
			if calls != 1 {
				t.Errorf("rule case answered %d times, want 1 (sample_count=1)", calls)
			}
		}
	}
	if judgeCases == 0 || ruleCases == 0 {
		t.Fatalf("cap_language should mix judge (%d) and rule (%d) cases", judgeCases, ruleCases)
	}
}

// TestSeedGen3NadirSnapshot asserts a run snapshots its suite's nadir at
// creation, read back through the eval API: the multiple-choice knowledge
// suite records 0.25, a legacy suite records 0.
func TestSeedGen3NadirSnapshot(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")

	knowledgeID := suiteIDByKey(t, ts.URL, "cap_knowledge")
	runID := triggerEval(t, ts.URL, knowledgeID, modelID)
	waitEvalDone(t, ts.URL, runID)
	if nadir := getEvalRun(t, ts.URL, runID)["nadir"]; nadir != 0.25 {
		t.Errorf("cap_knowledge run nadir = %v, want 0.25", nadir)
	}

	// A manual trigger on a retired legacy suite stays allowed and snapshots
	// the legacy nadir 0.
	legacyID := suiteIDByKey(t, ts.URL, "basic")
	legacyRunID := triggerEval(t, ts.URL, legacyID, modelID)
	waitEvalDone(t, ts.URL, legacyRunID)
	if nadir := getEvalRun(t, ts.URL, legacyRunID)["nadir"]; nadir != 0.0 {
		t.Errorf("legacy run nadir = %v, want 0", nadir)
	}
}
