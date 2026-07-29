package server_test

import (
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// presetScoredRun records one done run carrying a single scored result for
// the model, the smallest fixture that makes a suite show up on a campaign
// report with a known score. Returns the run ID.
func presetScoredRun(t *testing.T, db *store.DB, campaignID, suiteID, modelDBID int64, modelID string, caseID int64, score float64) int64 {
	t.Helper()
	run, err := db.CreateEvalRun(campaignID, suiteID, "manual", store.DefaultJudgeModel)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := db.CreateEvalResult(store.EvalResult{
		EvalRunID: run.ID, ModelDBID: modelDBID, ModelID: modelID,
		CaseID: caseID, Score: &score, LatencyMs: 10,
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := db.FinishEvalRun(run.ID, "done", time.Now()); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return run.ID
}

// firstCaseID returns the id of a suite's first case from its API payload.
func firstCaseID(t *testing.T, suite map[string]interface{}) int64 {
	t.Helper()
	cases, ok := suite["cases"].([]interface{})
	if !ok || len(cases) == 0 {
		t.Fatalf("suite %v has no cases", suite["key"])
	}
	return int64(cases[0].(map[string]interface{})["id"].(float64))
}

// TestSuitePurgeCascade covers spec 0014 decision B (ADR 0012) in its
// post-cutover form (ticket 99, ADR 0012 Addendum): an admin disabling a
// benchmark suite hands it to the purge — reopening the database
// hard-deletes it together with its cases, eval runs and eval results, leaf
// to root in one transaction, and the one-shot cutover migration never
// flips it back. Historical campaign reports keep rendering: they lose the
// purged dimension and the totals recompute over the remaining suites. The
// migration is idempotent, enabled suites and their data are untouched, and
// a campaign whose runs all belonged to purged suites renders an empty
// board instead of a 500.
func TestSuitePurgeCascade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "purge.db")

	// First boot: the post-cutover bank is exactly the five benchmark suites,
	// all enabled — the purge's seeds-disabled-by-design exemption no longer
	// covers anything (no bank entry carries retireAtGen 1).
	ts, db := openSuitesServer(t, dbPath)
	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != 5 {
		t.Fatalf("fresh bank suites = %d, want the 5 benchmark suites: %v", len(suites), suites)
	}
	for _, s := range suites {
		if s["enabled"] != true {
			t.Errorf("suite %v enabled = %v on a fresh post-cutover bank, want true", s["key"], s["enabled"])
		}
	}

	// Register a model so report rows can join against the models table.
	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	modelDBID := createEvalModel(t, ts.URL, stub.URL, "purge-model")

	doomed := suiteByKey(t, ts.URL, "mmlu")
	survivor := suiteByKey(t, ts.URL, "agieval_zh")
	doomedID := int64(doomed["id"].(float64))
	survivorID := int64(survivor["id"].(float64))
	doomedCaseID := firstCaseID(t, doomed)
	survivorCaseID := firstCaseID(t, survivor)
	survivorCaseCount := len(survivor["cases"].([]interface{}))

	now := time.Now()
	// Campaign 1 covers both suites; campaign 2 covers only the doomed one.
	c1, err := db.CreateCampaign("manual", []int64{modelDBID}, now)
	if err != nil {
		t.Fatalf("create campaign 1: %v", err)
	}
	doomedRunID := presetScoredRun(t, db, c1.ID, doomedID, modelDBID, "purge-model", doomedCaseID, 0.5)
	presetScoredRun(t, db, c1.ID, survivorID, modelDBID, "purge-model", survivorCaseID, 0.8)
	if err := db.SettleCampaign(c1.ID, now); err != nil {
		t.Fatalf("settle campaign 1: %v", err)
	}
	c2, err := db.CreateCampaign("manual", []int64{modelDBID}, now)
	if err != nil {
		t.Fatalf("create campaign 2: %v", err)
	}
	presetScoredRun(t, db, c2.ID, doomedID, modelDBID, "purge-model", doomedCaseID, 0.5)
	if err := db.SettleCampaign(c2.ID, now); err != nil {
		t.Fatalf("settle campaign 2: %v", err)
	}

	// Sanity before the purge: campaign 1 total is the mean of 50 and 80
	// (both MCQ suites share the 0.25 nadir: (0.5-0.25)/0.75*100 = 33.3 and
	// (0.8-0.25)/0.75*100 = 73.3 — assert via the raw report numbers below).
	before := getCampaignReport(t, ts.URL, c1.ID, "")
	beforeRows := reportRows(t, before)
	if len(beforeRows) != 1 || beforeRows[0]["total_score"] == nil {
		t.Fatalf("pre-purge campaign 1 rows = %v, want one row with a numeric total", beforeRows)
	}

	// The admin retires mmlu; reopening must purge it — the ADR 0012
	// Addendum semantics: a benchmark suite disabled after the cutover is
	// gone, tombstoned, and never re-enabled by the one-shot cutover
	// migration.
	if err := db.SetSuiteEnabled(doomedID, false); err != nil {
		t.Fatalf("disable mmlu: %v", err)
	}
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()

	after := fetchSuites(t, ts2.URL, "")
	if len(after) != 4 {
		t.Fatalf("suites after purge = %d, want 4 (mmlu purged): %v", len(after), after)
	}
	for _, s := range after {
		if s["key"] == "mmlu" {
			t.Errorf("admin-disabled benchmark suite mmlu survived the purge")
		}
		if s["enabled"] != true {
			t.Errorf("surviving suite %v enabled = %v, want true", s["key"], s["enabled"])
		}
	}
	// The enabled suite keeps every case (regression: no collateral damage).
	if got := len(suiteByKey(t, ts2.URL, "agieval_zh")["cases"].([]interface{})); got != survivorCaseCount {
		t.Errorf("agieval_zh cases after purge = %d, want %d (untouched)", got, survivorCaseCount)
	}

	// The purged run is unreachable — no ghost records on the ops history API.
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", ts2.URL, doomedRunID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET purged run: expected 404, got %d", resp.StatusCode)
	}

	// Campaign 1 report loses the purged dimension and recomputes the total
	// over the remaining suite, never 500s.
	report := getCampaignReport(t, ts2.URL, c1.ID, "")
	reportSuites := report["suites"].([]interface{})
	if len(reportSuites) != 1 || reportSuites[0].(map[string]interface{})["key"] != "agieval_zh" {
		t.Errorf("campaign 1 report suites = %v, want only agieval_zh", reportSuites)
	}
	rows := reportRows(t, report)
	if len(rows) != 1 {
		t.Fatalf("campaign 1 report rows = %v, want one row", rows)
	}
	scores := rows[0]["suite_scores"].(map[string]interface{})
	if _, ok := scores["mmlu"]; ok {
		t.Errorf("purged dimension still in suite_scores: %v", scores)
	}
	// (0.8 - 0.25) / 0.75 * 100 = 73.33 on the remaining dimension alone.
	if got := rows[0]["total_score"]; got == nil {
		t.Errorf("campaign 1 total after purge = %v, want the agieval-only normalized score", got)
	} else if want := (0.8 - 0.25) / (1 - 0.25) * 100; math.Abs(got.(float64)-want) > 1e-9 {
		t.Errorf("campaign 1 total after purge = %v, want %v (remaining dimension only)", got, want)
	}

	// Campaign 2 lost every run: an empty board, not a 500.
	report2 := getCampaignReport(t, ts2.URL, c2.ID, "")
	if got := len(report2["suites"].([]interface{})); got != 0 {
		t.Errorf("campaign 2 report suites = %d, want 0", got)
	}
	if got := len(reportRows(t, report2)); got != 0 {
		t.Errorf("campaign 2 report rows = %d, want 0", got)
	}

	// Idempotency: a third boot changes nothing — no error, no resurrection
	// (the tombstone beats both the seed bank and the one-shot cutover
	// migration), no second-delete side effects.
	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	again := fetchSuites(t, ts3.URL, "")
	if len(again) != 4 {
		t.Errorf("suites after third boot = %d, want still 4", len(again))
	}
	for _, s := range again {
		if s["key"] == "mmlu" {
			t.Errorf("suite %v resurrected on third boot", s["key"])
		}
	}
	report3 := getCampaignReport(t, ts3.URL, c1.ID, "")
	rows3 := reportRows(t, report3)
	if len(rows3) != 1 || rows3[0]["total_score"] == nil {
		t.Errorf("campaign 1 report after third boot = %v, want one row with a numeric total", rows3)
	}
}
