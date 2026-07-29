package server_test

import (
	"fmt"
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

// TestSuitePurgeCascade covers spec 0014 decision B (ADR 0012): reopening
// the database hard-deletes every disabled suite together with its cases,
// eval runs and eval results — leaf to root in one transaction. Historical
// campaign reports keep rendering: they lose the purged dimension and the
// totals recompute over the remaining suites. The migration is idempotent,
// enabled suites and their data are untouched, and a campaign whose runs all
// belonged to purged suites renders an empty board instead of a 500.
func TestSuitePurgeCascade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "purge.db")

	// First boot: the seeded bank holds the five capability suites plus the
	// mmlu/gsm8k/agieval_zh benchmark suites (ADR 0013), which seed disabled
	// by design until the ticket-99 cutover and are exempt from the purge as
	// seeded-disabled suites — no pre-v3 legacy suite (capability "") may
	// ever be listed.
	ts, db := openSuitesServer(t, dbPath)
	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != 10 {
		t.Fatalf("fresh bank suites = %d, want 5 capability suites + 5 benchmark disabled: %v", len(suites), suites)
	}
	for _, s := range suites {
		if s["capability"] == "" {
			t.Errorf("legacy suite %v listed on a fresh bank, want purged/never seeded", s["key"])
		}
		if s["key"] == "mmlu" || s["key"] == "gsm8k" || s["key"] == "agieval_zh" || s["key"] == "cruxeval" || s["key"] == "ifeval" {
			if s["enabled"] != false {
				t.Errorf("benchmark suite %v enabled = %v, want false until cutover", s["key"], s["enabled"])
			}
			continue
		}
		if s["enabled"] != true {
			t.Errorf("capability suite %v enabled = %v, want true", s["key"], s["enabled"])
		}
	}

	// Register a model so report rows can join against the models table.
	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	modelDBID := createEvalModel(t, ts.URL, stub.URL, "purge-model")

	instruction := suiteByKey(t, ts.URL, "cap_instruction")
	reasoning := suiteByKey(t, ts.URL, "cap_reasoning")
	instructionID := int64(instruction["id"].(float64))
	reasoningID := int64(reasoning["id"].(float64))
	instructionCaseID := firstCaseID(t, instruction)
	reasoningCaseID := firstCaseID(t, reasoning)
	reasoningCaseCount := len(reasoning["cases"].([]interface{}))

	now := time.Now()
	// Campaign 1 covers both suites; campaign 2 covers only the doomed one.
	c1, err := db.CreateCampaign("manual", []int64{modelDBID}, now)
	if err != nil {
		t.Fatalf("create campaign 1: %v", err)
	}
	doomedRunID := presetScoredRun(t, db, c1.ID, instructionID, modelDBID, "purge-model", instructionCaseID, 0.5)
	presetScoredRun(t, db, c1.ID, reasoningID, modelDBID, "purge-model", reasoningCaseID, 0.8)
	if err := db.SettleCampaign(c1.ID, now); err != nil {
		t.Fatalf("settle campaign 1: %v", err)
	}
	c2, err := db.CreateCampaign("manual", []int64{modelDBID}, now)
	if err != nil {
		t.Fatalf("create campaign 2: %v", err)
	}
	presetScoredRun(t, db, c2.ID, instructionID, modelDBID, "purge-model", instructionCaseID, 0.5)
	if err := db.SettleCampaign(c2.ID, now); err != nil {
		t.Fatalf("settle campaign 2: %v", err)
	}

	// Sanity before the purge: campaign 1 total is the mean of 50 and 80.
	before := getCampaignReport(t, ts.URL, c1.ID, "")
	beforeRows := reportRows(t, before)
	if len(beforeRows) != 1 || beforeRows[0]["total_score"] != 65.0 {
		t.Fatalf("pre-purge campaign 1 rows = %v, want one row totaling 65", beforeRows)
	}

	// Retire cap_instruction, then reopen: the migration must purge it.
	if err := db.SetSuiteEnabled(instructionID, false); err != nil {
		t.Fatalf("disable cap_instruction: %v", err)
	}
	db.Close()

	ts2, db2 := openSuitesServer(t, dbPath)
	defer db2.Close()

	after := fetchSuites(t, ts2.URL, "")
	if len(after) != 9 {
		t.Fatalf("suites after purge = %d, want 9 (4 capability + 5 benchmark exempt): %v", len(after), after)
	}
	for _, s := range after {
		if s["key"] == "cap_instruction" {
			t.Errorf("disabled suite cap_instruction survived the purge")
		}
		if s["capability"] == "" {
			t.Errorf("legacy suite %v listed after purge", s["key"])
		}
	}
	// The enabled suite keeps every case (regression: no collateral damage).
	if got := len(suiteByKey(t, ts2.URL, "cap_reasoning")["cases"].([]interface{})); got != reasoningCaseCount {
		t.Errorf("cap_reasoning cases after purge = %d, want %d (untouched)", got, reasoningCaseCount)
	}

	// The purged run is unreachable — no ghost records on the ops history API.
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", ts2.URL, doomedRunID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET purged run: expected 404, got %d", resp.StatusCode)
	}

	// Campaign 1 report loses the purged dimension and recomputes the total
	// over the remaining suite (80), never 500s.
	report := getCampaignReport(t, ts2.URL, c1.ID, "")
	reportSuites := report["suites"].([]interface{})
	if len(reportSuites) != 1 || reportSuites[0].(map[string]interface{})["key"] != "cap_reasoning" {
		t.Errorf("campaign 1 report suites = %v, want only cap_reasoning", reportSuites)
	}
	rows := reportRows(t, report)
	if len(rows) != 1 {
		t.Fatalf("campaign 1 report rows = %v, want one row", rows)
	}
	scores := rows[0]["suite_scores"].(map[string]interface{})
	if _, ok := scores["cap_instruction"]; ok {
		t.Errorf("purged dimension still in suite_scores: %v", scores)
	}
	if rows[0]["total_score"] != 80.0 {
		t.Errorf("campaign 1 total after purge = %v, want 80 (remaining dimension only)", rows[0]["total_score"])
	}

	// Campaign 2 lost every run: an empty board, not a 500.
	report2 := getCampaignReport(t, ts2.URL, c2.ID, "")
	if got := len(report2["suites"].([]interface{})); got != 0 {
		t.Errorf("campaign 2 report suites = %d, want 0", got)
	}
	if got := len(reportRows(t, report2)); got != 0 {
		t.Errorf("campaign 2 report rows = %d, want 0", got)
	}

	// Idempotency: a third boot changes nothing — no error, no resurrection,
	// no second-delete side effects.
	db2.Close()
	ts3, db3 := openSuitesServer(t, dbPath)
	defer db3.Close()
	again := fetchSuites(t, ts3.URL, "")
	if len(again) != 9 {
		t.Errorf("suites after third boot = %d, want still 9 (4 capability + 5 benchmark exempt)", len(again))
	}
	for _, s := range again {
		if s["key"] == "cap_instruction" || s["capability"] == "" {
			t.Errorf("suite %v resurrected on third boot", s["key"])
		}
	}
	report3 := getCampaignReport(t, ts3.URL, c1.ID, "")
	rows3 := reportRows(t, report3)
	if len(rows3) != 1 || rows3[0]["total_score"] != 80.0 {
		t.Errorf("campaign 1 report after third boot = %v, want one row totaling 80", rows3)
	}
}
