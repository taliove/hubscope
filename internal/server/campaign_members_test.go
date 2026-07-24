package server_test

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// firstReportSuiteKey returns the key of the first suite the campaign's
// report covers (the suite of the first run created).
func firstReportSuiteKey(t *testing.T, report map[string]interface{}) string {
	t.Helper()
	suites, ok := report["suites"].([]interface{})
	if !ok || len(suites) == 0 {
		t.Fatalf("report suites missing: %v", report["suites"])
	}
	key, ok := suites[0].(map[string]interface{})["key"].(string)
	if !ok {
		t.Fatalf("report suite key missing: %v", suites[0])
	}
	return key
}

// rowModelIDs extracts the model IDs of report rows in order.
func rowModelIDs(rows []map[string]interface{}) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["model_id"].(string))
	}
	return ids
}

// TestFullSweepMembersPendingFromFirstRun pins the ticket 53 contract: a
// full sweep snapshots its model population at campaign creation, so the
// live report lists every member from the first run on — a model the sweep
// has not reached yet renders "pending" cells instead of staying invisible
// until its first result lands (the ticket 52 derivation gap).
//
// Scenario: three models, gamma frozen on its very first answer call. At the
// freeze point the first run is mid-flight: alpha and beta have their full
// cap_instruction case set recorded (effectively done cells), gamma has zero
// results anywhere — yet must already occupy a row with a pending cell.
func TestFullSweepMembersPendingFromFirstRun(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	createEvalModel(t, ts.URL, stub.URL, "gamma-model")
	stub.blockModel("gamma-model")
	stub.resetCalls()
	t.Cleanup(func() { stub.releaseModel("gamma-model") })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	// Gamma's first call reaching the stub proves the freeze point: the
	// first run is in flight with alpha and beta fully recorded and gamma
	// untouched.
	waitFor(t, "gamma's first answer call reaching the stub", func() bool {
		return stub.callTotal("gamma-model") >= 1
	})

	report := getCampaignReport(t, ts.URL, campaignID, "")
	if report["status"] != store.CampaignStatusRunning {
		t.Fatalf("campaign status = %v, want running", report["status"])
	}
	suiteKey := firstReportSuiteKey(t, report)

	rows := reportRows(t, report)
	want := []string{"alpha-model", "beta-model", "gamma-model"}
	if got := rowModelIDs(rows); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("live board rows = %v, want every member %v (lexicographic, gamma pending)", got, want)
	}
	assertCell(t, rows[0], suiteKey, "done", 10, 10)
	assertCell(t, rows[1], suiteKey, "done", 10, 10)
	assertCell(t, rows[2], suiteKey, "pending", 0, 10)

	// Gamma has no results yet: no scores, no total.
	gamma := rows[2]
	if gamma["total_score"] != nil {
		t.Errorf("gamma total_score = %v, want null (no results yet)", gamma["total_score"])
	}
	if scores, _ := gamma["suite_scores"].(map[string]interface{}); len(scores) != 0 {
		t.Errorf("gamma suite_scores = %v, want empty (no results yet)", scores)
	}

	// Settle: releasing gamma lets the sweep finish and every member ends
	// up on the ranked board.
	stub.releaseModel("gamma-model")
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)
	settled := reportRows(t, getCampaignReport(t, ts.URL, campaignID, ""))
	if len(settled) != 3 {
		t.Fatalf("settled rows = %v, want all three members", rowModelIDs(settled))
	}
}

// TestManualRunMembersMatchSelection pins the manual single-run path: the
// membership snapshot holds exactly the selected models — no more (an
// unselected model never appears), no less (a selected model shows pending
// before its first result).
func TestManualRunMembersMatchSelection(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	alphaID := createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	gammaID := createEvalModel(t, ts.URL, stub.URL, "gamma-model")
	suiteID := suiteIDByKey(t, ts.URL, "cap_instruction")
	stub.blockModel("gamma-model")
	stub.resetCalls()
	t.Cleanup(func() { stub.releaseModel("gamma-model") })

	triggerEval(t, ts.URL, suiteID, alphaID, gammaID)
	waitFor(t, "gamma's first answer call reaching the stub", func() bool {
		return stub.callTotal("gamma-model") >= 1
	})

	campaigns := listCampaigns(t, ts.URL)
	if len(campaigns) != 1 {
		t.Fatalf("campaigns = %v, want exactly one", campaigns)
	}
	campaignID := int64(campaigns[0]["id"].(float64))
	report := getCampaignReport(t, ts.URL, campaignID, "")
	suiteKey := firstReportSuiteKey(t, report)

	rows := reportRows(t, report)
	if got := rowModelIDs(rows); len(got) != 2 || got[0] != "alpha-model" || got[1] != "gamma-model" {
		t.Fatalf("live board rows = %v, want exactly the selected [alpha-model gamma-model]", got)
	}
	assertCell(t, rows[0], suiteKey, "done", 10, 10)
	assertCell(t, rows[1], suiteKey, "pending", 0, 10)

	stub.releaseModel("gamma-model")
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)
}

// TestDeletedModelStaysOffLiveMemberBoard pins the ticket 26 semantics on
// the member-driven board: the membership snapshot is a historical fact, but
// a model deleted mid-batch must drop out of the live board (members join
// the models table at read time) — the snapshot never resurrects it.
func TestDeletedModelStaysOffLiveMemberBoard(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	betaID := createEvalModel(t, ts.URL, stub.URL, "beta-model")
	createEvalModel(t, ts.URL, stub.URL, "gamma-model")
	stub.blockModel("gamma-model")
	stub.resetCalls()
	t.Cleanup(func() { stub.releaseModel("gamma-model") })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitFor(t, "gamma's first answer call reaching the stub", func() bool {
		return stub.callTotal("gamma-model") >= 1
	})

	// Beta is a member with recorded results in the in-flight run; deleting
	// it must remove its row from the live board.
	resp := doDelete(t, ts.URL+"/api/models/"+strconv.FormatInt(betaID, 10))
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("delete beta-model: got status %d", resp.StatusCode)
	}

	report := getCampaignReport(t, ts.URL, campaignID, "")
	rows := reportRows(t, report)
	if got := rowModelIDs(rows); len(got) != 2 || got[0] != "alpha-model" || got[1] != "gamma-model" {
		t.Fatalf("live board rows after delete = %v, want [alpha-model gamma-model] (deleted member hidden)", got)
	}

	stub.releaseModel("gamma-model")
}

// stagePreMembersDatabase writes a pre-ticket-53 database: the current
// schema minus the campaign_models table, with one done campaign whose done
// run recorded results for two models. It hand-copies the schema as of
// ticket 53 (2026-07-23); later migrations do not need to be mirrored here —
// the fixture only has to look like a database that predates campaign_models.
func stagePreMembersDatabase(t *testing.T, path string) {
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
			version INTEGER NOT NULL DEFAULT 1
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
		CREATE TABLE eval_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			campaign_id INTEGER NOT NULL,
			suite_id INTEGER NOT NULL,
			suite_version INTEGER NOT NULL DEFAULT 1,
			"trigger" TEXT NOT NULL,
			judge_model TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT
		);
		CREATE TABLE eval_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			eval_run_id INTEGER NOT NULL,
			model_db_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			case_id INTEGER NOT NULL,
			answer_text TEXT,
			score REAL,
			latency_ms INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`
	if _, err := conn.Exec(ddl); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	stmts := []string{
		`INSERT INTO hubs (name, base_url, token, created_at) VALUES ('legacy hub', 'http://hub', 'tok', '2026-07-01T00:00:00Z')`,
		`INSERT INTO models (hub_id, model_id, family, created_at) VALUES (1, 'legacy-alpha', 'gpt', '2026-07-01T00:00:00Z')`,
		`INSERT INTO models (hub_id, model_id, family, created_at) VALUES (1, 'legacy-beta', 'gpt', '2026-07-01T00:00:00Z')`,
		`INSERT INTO suites (key, name) VALUES ('legacy_basic', '遗留基础')`,
		`INSERT INTO cases (suite_id, prompt, verdict_type, created_at) VALUES (1, '1+1=?', 'rule', '2026-07-01T00:00:00Z')`,
		`INSERT INTO campaigns ("trigger", status, started_at, finished_at, created_at) VALUES ('manual', 'done', '2026-07-01T01:00:00Z', '2026-07-01T01:05:00Z', '2026-07-01T01:00:00Z')`,
		`INSERT INTO eval_runs (campaign_id, suite_id, "trigger", judge_model, status, started_at, finished_at) VALUES (1, 1, 'manual', 'fake-judge', 'done', '2026-07-01T01:00:00Z', '2026-07-01T01:05:00Z')`,
		`INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, score, latency_ms, created_at) VALUES (1, 1, 'legacy-alpha', 1, 1.0, 10, '2026-07-01T01:01:00Z')`,
		`INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, score, latency_ms, created_at) VALUES (1, 2, 'legacy-beta', 1, 0.5, 10, '2026-07-01T01:02:00Z')`,
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("stage old row: %v\n%s", err, stmt)
		}
	}
}

// TestCampaignMembersMigrationBackfills opens a pre-membership database and
// asserts the migration backfills campaign membership from the recorded
// results without disturbing the settled report: the grid keeps serving
// exactly the models with results, and re-opening is a no-op.
func TestCampaignMembersMigrationBackfills(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pre-members.db")
	stagePreMembersDatabase(t, dbPath)

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

	assertBoard := func(ts *httptest.Server) {
		t.Helper()
		report := getCampaignReport(t, ts.URL, 1, "")
		rows := reportRows(t, report)
		if got := rowModelIDs(rows); len(got) != 2 || got[0] != "legacy-alpha" || got[1] != "legacy-beta" {
			t.Fatalf("migrated report rows = %v, want [legacy-alpha legacy-beta] (backfill consistent)", got)
		}
		scores, _ := rows[0]["suite_scores"].(map[string]interface{})
		if scores["legacy_basic"] != 100.0 {
			t.Errorf("legacy-alpha legacy_basic = %v, want 100", scores["legacy_basic"])
		}
		scores, _ = rows[1]["suite_scores"].(map[string]interface{})
		if scores["legacy_basic"] != 50.0 {
			t.Errorf("legacy-beta legacy_basic = %v, want 50", scores["legacy_basic"])
		}
		assertCell(t, rows[0], "legacy_basic", "done", 1, 1)
		assertCell(t, rows[1], "legacy_basic", "done", 1, 1)
	}

	ts, db := serve()
	assertBoard(ts)
	db.Close()

	// Re-opening must be a no-op: the backfill is idempotent and the board
	// stays identical.
	ts2, db2 := serve()
	defer db2.Close()
	assertBoard(ts2)
}
