package server_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// This file holds the shared fixtures of the post-cutover (ticket 99, spec
// 0014 decision C) test bank plus the black-box cutover coverage itself:
// fresh databases seed the five authoritative-benchmark suites enabled, an
// upgrade from the pre-cutover seed state retires and purges the v3
// capability suites while flipping the benchmark suites on, an admin
// disabling a benchmark suite hands it to the disabled-suite purge (ADR 0012
// Addendum), and the zero-suite window degrades a batch to an empty done
// campaign instead of an error.

// createJudgeCaseForTest posts one judge case with a minimal 1/0 rubric into
// the suite; the stub judge scores it 0.75 by default (scriptable via
// setJudgeSeq on the marker embedded in the prompt).
func createJudgeCaseForTest(t *testing.T, base string, suiteID int64, prompt string) {
	t.Helper()
	resp := doPost(t, base+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       prompt,
		"verdict_type": "judge",
		"rubric":       "评分标准：回答得体得 1 分，否则 0 分。",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create judge case %q: expected 201, got %d", prompt, resp.StatusCode)
	}
}

// benchmarkRotation lists the five authoritative-benchmark suite keys in
// seed-bank order — the post-cutover evaluation rotation.
var benchmarkRotation = []string{"mmlu", "agieval_zh", "gsm8k", "cruxeval", "ifeval"}

// retireSuiteCases disables every enabled case of the suite through the
// store. It is fixture setup only (the same seam precedent as
// presetScoredRun and SetSuiteEnabled); assertions stay on the HTTP API.
// The seeded benchmark suites carry 100 cases each, so retiring via the
// case PATCH API one request at a time would dominate test runtime.
func retireSuiteCases(t *testing.T, db *store.DB, suiteID int64) {
	t.Helper()
	cases, err := db.ListCases(suiteID)
	if err != nil {
		t.Fatalf("list cases of suite %d: %v", suiteID, err)
	}
	for _, c := range cases {
		if !c.Enabled {
			continue
		}
		if _, err := db.SetCaseEnabled(c.ID, false); err != nil {
			t.Fatalf("retire case %d of suite %d: %v", c.ID, suiteID, err)
		}
	}
}

// suiteIDByKeyFromStore resolves a seeded suite's id via the store (fixture
// setup; the API twin suiteIDByKey exists for HTTP-level tests).
func suiteIDByKeyFromStore(t *testing.T, db *store.DB, key string) int64 {
	t.Helper()
	suites, err := db.ListSuites()
	if err != nil {
		t.Fatalf("list suites: %v", err)
	}
	for _, s := range suites {
		if s.Key == key {
			return s.ID
		}
	}
	t.Fatalf("suite %q not found", key)
	return 0
}

// installCustomBank replaces the enabled case set of every given suite with
// custom exact-match rule cases (expected "好的"): the stub's default smart
// answer scores 1, a bad/dumb model ("随便说点什么") scores 0 yet stays
// judged, and a broken model stays unjudged — the deterministic scoring
// triangle sweep tests need. casesPerSuite maps suite key to the number of
// custom cases to install (typically one; freeze tests give the last suite
// more so a model can freeze mid-suite).
func installCustomBank(t *testing.T, base string, db *store.DB, casesPerSuite map[string]int) {
	t.Helper()
	for key, n := range casesPerSuite {
		suiteID := suiteIDByKeyFromStore(t, db, key)
		retireSuiteCases(t, db, suiteID)
		for i := 0; i < n; i++ {
			createRuleCase(t, base, suiteID, customBankPrompt(key, i), "好的", nil)
		}
	}
}

// customBankPrompt is the prompt of the i-th custom case installed by
// installCustomBank; the marker keeps it unique across suites and tests.
func customBankPrompt(suiteKey string, i int) string {
	return "CUSTOMBANK-" + suiteKey + "-" + string(rune('A'+i)) + ":请作答"
}

// oneCasePerSuite returns the canonical custom bank: every rotation suite
// with exactly one exact-rule case.
func oneCasePerSuite() map[string]int {
	out := map[string]int{}
	for _, key := range benchmarkRotation {
		out[key] = 1
	}
	return out
}

// TestCutoverFreshBankSeedsFiveEnabledBenchmarkSuites pins the fresh-database
// path: the five authoritative-benchmark suites land enabled with their
// 100-case frozen subsets, and no v3 capability suite (or pre-v3 legacy
// suite) exists — the v3 bank is seeded, retired and purged inside the same
// first Open.
func TestCutoverFreshBankSeedsFiveEnabledBenchmarkSuites(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != len(benchmarkRotation) {
		t.Fatalf("fresh bank suites = %d, want exactly the %d benchmark suites: %v",
			len(suites), len(benchmarkRotation), suites)
	}
	wantCapability := map[string]string{
		"mmlu": "knowledge", "agieval_zh": "language", "gsm8k": "reasoning",
		"cruxeval": "coding", "ifeval": "instruction",
	}
	for _, s := range suites {
		key := s["key"].(string)
		capability, ok := wantCapability[key]
		if !ok {
			t.Errorf("unexpected suite %q in the post-cutover bank", key)
			continue
		}
		if s["enabled"] != true {
			t.Errorf("benchmark suite %q enabled = %v, want true (post-cutover rotation)", key, s["enabled"])
		}
		if s["capability"] != capability {
			t.Errorf("suite %q capability = %v, want %q", key, s["capability"], capability)
		}
		if got := len(s["cases"].([]interface{})); got != 100 {
			t.Errorf("suite %q cases = %d, want 100 (frozen subset)", key, got)
		}
	}
}

// TestCutoverEmptyRotationDegradesToEmptyBatch pins ticket 99 risk 3: with
// zero suites in the evaluation rotation, a manual full sweep settles as an
// empty done campaign (zero runs) instead of an error, and its report
// renders an empty board with HTTP 200.
func TestCutoverEmptyRotationDegradesToEmptyBatch(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")

	for _, key := range benchmarkRotation {
		if err := db.SetSuiteEnabled(suiteIDByKeyFromStore(t, db, key), false); err != nil {
			t.Fatalf("disable suite %q: %v", key, err)
		}
	}

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	final := waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")
	if final["status"] != "done" {
		t.Errorf("empty-rotation campaign status = %v, want done (safe degradation, not an error)", final["status"])
	}
	if got := len(campaignRuns(t, final)); got != 0 {
		t.Errorf("empty-rotation campaign runs = %d, want 0", got)
	}

	report := getCampaignReport(t, ts.URL, campaignID, "")
	if got := len(report["suites"].([]interface{})); got != 0 {
		t.Errorf("empty-rotation report suites = %d, want 0", got)
	}
	if got := len(reportRows(t, report)); got != 0 {
		t.Errorf("empty-rotation report rows = %d, want 0", got)
	}
}

// TestCutoverEmptyRotationWeeklyBatchDegrades pins the scheduled side of the
// empty-rotation window (ticket 99 risk 3): the weekly batch with zero
// enabled suites produces one empty done campaign, not a failed one.
func TestCutoverEmptyRotationWeeklyBatchDegrades(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "cutover-weekly.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, server.WithRateLimits(server.RateLimits{}))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	for _, key := range benchmarkRotation {
		if err := db.SetSuiteEnabled(suiteIDByKeyFromStore(t, db, key), false); err != nil {
			t.Fatalf("disable suite %q: %v", key, err)
		}
	}

	clock := scheduler.NewFakeClock(time.Date(2026, 7, 18, 23, 30, 0, 0, time.UTC)) // Saturday
	startEvalWorker(t, db, srv, clock)
	clock.Advance(2 * time.Hour) // Sunday 01:30: the weekly window

	waitFor(t, "one scheduled campaign", func() bool {
		campaigns := listCampaigns(t, ts.URL)
		return len(campaigns) == 1 && campaigns[0]["trigger"] == "scheduled"
	})
	campaigns := listCampaigns(t, ts.URL)
	campaignID := int64(campaigns[0]["id"].(float64))
	final := waitCampaignStatus(t, ts.URL, campaignID, "done", "failed")
	if final["status"] != "done" {
		t.Errorf("empty-rotation weekly campaign status = %v, want done (safe degradation)", final["status"])
	}
	if got := len(campaignRuns(t, final)); got != 0 {
		t.Errorf("empty-rotation weekly campaign runs = %d, want 0", got)
	}
}

// stagePreCutoverDatabase writes a tickets-94..98-era database: the v3
// capability suites enabled at seed generation 3, the five benchmark suites
// seeded disabled at generation 1 (one hand case each standing in for the
// frozen subsets), and one settled v3 campaign carrying a done run with a
// scored result — the shape the ticket-99 cutover migrates.
func stagePreCutoverDatabase(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer conn.Close()

	ddl := `
		CREATE TABLE hubs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			token TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hub_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			origin TEXT NOT NULL DEFAULT 'manual',
			status TEXT NOT NULL DEFAULT 'active',
			capability TEXT NOT NULL DEFAULT 'chat',
			family TEXT NOT NULL DEFAULT 'other',
			created_at TEXT NOT NULL,
			UNIQUE(hub_id, model_id)
		);
		CREATE TABLE suites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			capability TEXT NOT NULL DEFAULT '',
			nadir REAL NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE cases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suite_id INTEGER NOT NULL,
			prompt TEXT NOT NULL,
			verdict_type TEXT NOT NULL,
			rule_mode TEXT,
			rule_expected TEXT,
			rubric TEXT,
			difficulty TEXT NOT NULL DEFAULT 'basic',
			sample_count INTEGER,
			check_params TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		);
		CREATE TABLE campaigns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			"trigger" TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT,
			created_at TEXT NOT NULL
		);
		CREATE TABLE campaign_models (
			campaign_id INTEGER NOT NULL,
			model_id INTEGER NOT NULL,
			PRIMARY KEY (campaign_id, model_id)
		);
		CREATE TABLE eval_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			campaign_id INTEGER NOT NULL,
			suite_id INTEGER NOT NULL,
			suite_version INTEGER NOT NULL DEFAULT 1,
			"trigger" TEXT NOT NULL,
			judge_model TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			nadir REAL NOT NULL DEFAULT 0
		);
		CREATE TABLE eval_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			eval_run_id INTEGER NOT NULL,
			model_db_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			case_id INTEGER NOT NULL,
			answer_text TEXT,
			score REAL,
			verdict_detail TEXT,
			verdict_profile TEXT NOT NULL DEFAULT 'v2',
			latency_ms INTEGER NOT NULL,
			input_tokens INTEGER,
			output_tokens INTEGER,
			created_at TEXT NOT NULL
		);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`
	if _, err := conn.Exec(ddl); err != nil {
		t.Fatalf("create pre-cutover schema: %v", err)
	}

	stmts := []string{
		`INSERT INTO hubs (name, base_url, token, created_at) VALUES ('hub', 'http://hub', 'tok', '2026-07-20T00:00:00Z')`,
		`INSERT INTO models (hub_id, model_id, created_at) VALUES (1, 'legacy-model', '2026-07-20T00:00:00Z')`,
		// v3 capability suites: enabled, generation 3 received.
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('cap_instruction', '指令遵循', 'instruction', 0, 1)`,
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('cap_reasoning', '推理', 'reasoning', 0, 1)`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_cap_instruction', '3')`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_cap_reasoning', '3')`,
		// Benchmark suites: seeded disabled at generation 1 (tickets 94-98).
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('mmlu', '知识（MMLU）', 'knowledge', 0.25, 0)`,
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('agieval_zh', '中文（AGIEval）', 'language', 0.25, 0)`,
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('gsm8k', '推理（GSM8K）', 'reasoning', 0, 0)`,
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('cruxeval', '代码（CRUXEval）', 'coding', 0, 0)`,
		`INSERT INTO suites (key, name, capability, nadir, enabled) VALUES ('ifeval', '指令遵循（IFEval）', 'instruction', 0, 0)`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_mmlu', '1')`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_agieval_zh', '1')`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_gsm8k', '1')`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_cruxeval', '1')`,
		`INSERT INTO settings (key, value) VALUES ('seed_gen_ifeval', '1')`,
		// One hand case per benchmark suite, standing in for the frozen
		// subsets — the cutover must flip the suite on without touching them.
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, created_at) VALUES (3, 'staged mmlu case', 'rule', 'mcq', 'A', '2026-07-20T00:00:00Z')`,
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, created_at) VALUES (4, 'staged agieval case', 'rule', 'mcq', 'B', '2026-07-20T00:00:00Z')`,
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, created_at) VALUES (5, 'staged gsm8k case', 'rule', 'numeric', '1', '2026-07-20T00:00:00Z')`,
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, created_at) VALUES (6, 'staged cruxeval case', 'rule', 'output_match', '1', '2026-07-20T00:00:00Z')`,
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, created_at) VALUES (7, 'staged ifeval case', 'rule', 'ifeval', '2026-07-20T00:00:00Z')`,
		// A settled v3 campaign: one done run on cap_instruction with a scored
		// result — the historical report that must survive the purge.
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, created_at) VALUES (1, 'v3 case', 'rule', 'exact', '好的', '2026-07-20T00:00:00Z')`,
		`INSERT INTO campaigns ("trigger", status, started_at, finished_at, created_at) VALUES ('scheduled', 'done', '2026-07-21T01:00:00Z', '2026-07-21T01:05:00Z', '2026-07-21T01:00:00Z')`,
		`INSERT INTO campaign_models (campaign_id, model_id) VALUES (1, 1)`,
		`INSERT INTO eval_runs (campaign_id, suite_id, suite_version, "trigger", judge_model, status, started_at, finished_at, nadir) VALUES (1, 1, 1, 'scheduled', 'fake-judge', 'done', '2026-07-21T01:00:00Z', '2026-07-21T01:05:00Z', 0)`,
		`INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, answer_text, score, latency_ms, created_at) VALUES (1, 1, 'legacy-model', 6, '好的', 1.0, 10, '2026-07-21T01:01:00Z')`,
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("stage pre-cutover row: %v\n%s", err, stmt)
		}
	}
}

// TestCutoverUpgradePurgesV3AndEnablesBenchmark covers the upgrade path of
// the cutover (ticket 99 acceptance): opening a tickets-94..98-era database
// retires and purges the v3 capability suites with their cases/runs/results,
// flips the pre-seeded disabled benchmark suites into the rotation without
// re-seeding them, keeps the historical v3 campaign report rendering (empty
// board, never a 500), and is idempotent across restarts.
func TestCutoverUpgradePurgesV3AndEnablesBenchmark(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pre-cutover.db")
	stagePreCutoverDatabase(t, dbPath)

	serve := func() (*httptest.Server, *store.DB) {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open migrated db: %v", err)
		}
		seedTestUser(t, db)
		ts := httptest.NewServer(server.New(db, server.WithRateLimits(server.RateLimits{})))
		t.Cleanup(ts.Close)
		return ts, db
	}

	ts, db := serve()
	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != len(benchmarkRotation) {
		t.Fatalf("suites after cutover upgrade = %d, want exactly the %d benchmark suites: %v",
			len(suites), len(benchmarkRotation), suites)
	}
	for _, s := range suites {
		key := s["key"].(string)
		if key == "cap_instruction" || key == "cap_reasoning" {
			t.Errorf("v3 suite %q survived the cutover purge", key)
		}
		if s["enabled"] != true {
			t.Errorf("benchmark suite %q enabled = %v, want true after the cutover", key, s["enabled"])
		}
		// The staged hand cases prove the flip did not re-seed or wipe the
		// pre-seeded case set (generation 1 already received).
		if got := len(s["cases"].([]interface{})); got != 1 {
			t.Errorf("suite %q cases = %d, want the 1 staged case (no re-seed at cutover)", key, got)
		}
	}

	// The historical v3 campaign keeps rendering: its only run belonged to
	// the purged cap_instruction, so the report is an empty board, not a 500.
	report := getCampaignReport(t, ts.URL, 1, "")
	if got := len(report["suites"].([]interface{})); got != 0 {
		t.Errorf("v3 campaign report suites = %d, want 0 (v3 dimension purged)", got)
	}
	if got := len(reportRows(t, report)); got != 0 {
		t.Errorf("v3 campaign report rows = %d, want 0", got)
	}
	db.Close()

	// Re-opening is a no-op: nothing re-seeded, nothing re-flipped, the
	// benchmark suites still exactly five and enabled.
	ts2, db2 := serve()
	defer db2.Close()
	again := fetchSuites(t, ts2.URL, "")
	if len(again) != len(benchmarkRotation) {
		t.Fatalf("suites after reopen = %d, want still %d", len(again), len(benchmarkRotation))
	}
	for _, s := range again {
		if s["enabled"] != true || len(s["cases"].([]interface{})) != 1 {
			t.Errorf("suite %v drifted on reopen: enabled=%v cases=%d",
				s["key"], s["enabled"], len(s["cases"].([]interface{})))
		}
	}
}
