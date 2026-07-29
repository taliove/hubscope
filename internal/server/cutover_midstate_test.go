package server_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// countRowsForTest counts rows matching a raw query — the assertion twin of
// ExecRawForTest for mid-state fixtures whose rows no public method lists.
func countRowsForTest(t *testing.T, conn *sql.DB, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

// rawSQLite opens a second connection to the database file for fixture
// staging and low-level assertions (the stagePreCutoverDatabase pattern).
func rawSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestBenchmarkCutoverMidState pins the mid-state fallback (GH #15): the dev
// database reached the cutover through a half-applied path — the v3 suite
// rows are alive AND still enabled=1, their purge tombstones
// (purged_suite_cap_*) were already written so seedSuites skips the v3 bank
// and the generation-tracked retirement (retireAtGen 4) never fires, and the
// one-shot benchmark_cutover key is already recorded so the enable
// migration no-ops. Without an unconditional retirement the v3 suites would
// survive forever (the purge deletes only enabled=0). Reopening must retire
// them in the same Open and let the purge cascade them away — suites,
// cases, runs and results — while the five benchmark suites stay enabled.
func TestBenchmarkCutoverMidState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "midstate.db")

	// First Open lands the post-cutover bank (and writes benchmark_cutover=1
	// itself, exactly the one-shot key state of the mid-state).
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	// Sabotage into the dev mid-state: a v3 suite alive and enabled with a
	// case, a run and a result hanging off it, plus the five purge
	// tombstones that keep seedSuites from ever touching the v3 bank again.
	conn := rawSQLite(t, dbPath)
	stmts := []string{
		`INSERT INTO suites (key, name, version, capability, nadir, enabled)
			VALUES ('cap_instruction', '指令遵循', 1, 'instruction', 0, 1)`,
		`INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, enabled, created_at)
			VALUES ((SELECT id FROM suites WHERE key = 'cap_instruction'),
				'只回复数字:1+1=?', 'rule', 'exact', '2', 1, '2026-07-01T00:00:00Z')`,
		`INSERT INTO campaigns ("trigger", status, started_at, finished_at, created_at)
			VALUES ('scheduled', 'done', '2026-07-20T01:00:00Z', '2026-07-20T01:05:00Z', '2026-07-20T01:00:00Z')`,
		`INSERT INTO eval_runs (campaign_id, suite_id, suite_version, "trigger", judge_model, status, started_at, finished_at)
			VALUES (1, (SELECT id FROM suites WHERE key = 'cap_instruction'), 1,
				'scheduled', 'judge-x', 'done', '2026-07-20T01:00:00Z', '2026-07-20T01:05:00Z')`,
		`INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, answer_text, score, latency_ms, created_at)
			VALUES (1, 1, 'smart-model',
				(SELECT id FROM cases WHERE prompt = '只回复数字:1+1=?'),
				'2', 1.0, 120, '2026-07-20T01:01:00Z')`,
		`INSERT OR REPLACE INTO settings (key, value) VALUES
			('purged_suite_cap_instruction', '1'),
			('purged_suite_cap_reasoning', '1'),
			('purged_suite_cap_coding', '1'),
			('purged_suite_cap_knowledge', '1'),
			('purged_suite_cap_language', '1')`,
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("stage mid-state: %v\n%s", err, stmt)
		}
	}
	if got := countRowsForTest(t, conn, `SELECT COUNT(*) FROM settings WHERE key = 'benchmark_cutover'`); got != 1 {
		t.Fatalf("benchmark_cutover keys = %d, want 1 (written by the first Open)", got)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close staging conn: %v", err)
	}

	// Reopen: the mid-state fallback must converge the database.
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	suites, err := db2.ListSuites()
	if err != nil {
		t.Fatalf("list suites: %v", err)
	}
	if len(suites) != len(benchmarkRotation) {
		t.Fatalf("suites after mid-state reopen = %d, want exactly the %d benchmark suites: %v",
			len(suites), len(benchmarkRotation), suites)
	}
	for _, s := range suites {
		if !s.Enabled {
			t.Errorf("benchmark suite %q enabled = %v, want true (untouched by the fallback)", s.Key, s.Enabled)
		}
	}

	check := rawSQLite(t, dbPath)
	for _, q := range []struct {
		table string
		query string
	}{
		{"suites", `SELECT COUNT(*) FROM suites WHERE key LIKE 'cap_%'`},
		{"cases", `SELECT COUNT(*) FROM cases WHERE suite_id NOT IN (SELECT id FROM suites)`},
		{"eval_runs", `SELECT COUNT(*) FROM eval_runs WHERE suite_id NOT IN (SELECT id FROM suites)`},
		{"eval_results", `SELECT COUNT(*) FROM eval_results WHERE eval_run_id NOT IN (SELECT id FROM eval_runs)`},
	} {
		if got := countRowsForTest(t, check, q.query); got != 0 {
			t.Errorf("%s rows referencing the v3 suite = %d, want 0 (purged with cascade)", q.table, got)
		}
	}
}

// TestBenchmarkCutoverIdempotent pins the steady state: repeated Opens never
// drift the post-cutover bank, and an admin disabling a benchmark suite
// after the cutover hands it to the purge at the next Open — tombstoned,
// never resurrected (retireAtGen 0 + an empty seeds-disabled exemption:
// "disabled means gone", ADR 0012 Addendum).
func TestBenchmarkCutoverIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idempotent.db")

	open := func() *store.DB {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		return db
	}
	shape := func(db *store.DB) map[string]bool {
		suites, err := db.ListSuites()
		if err != nil {
			t.Fatalf("list suites: %v", err)
		}
		out := map[string]bool{}
		for _, s := range suites {
			out[s.Key] = s.Enabled
		}
		return out
	}

	db := open()
	first := shape(db)
	if len(first) != len(benchmarkRotation) {
		t.Fatalf("fresh bank = %d suites, want %d: %v", len(first), len(benchmarkRotation), first)
	}
	db.Close()

	// Two more Opens: the terminal state is byte-identical.
	for i := 0; i < 2; i++ {
		db = open()
		again := shape(db)
		db.Close()
		if len(again) != len(first) {
			t.Fatalf("reopen %d: suites = %d, want %d", i+1, len(again), len(first))
		}
		for key, enabled := range first {
			if again[key] != enabled {
				t.Fatalf("reopen %d: suite %q enabled drifted %v -> %v", i+1, key, enabled, again[key])
			}
		}
	}

	// The admin disables mmlu after the cutover: the next Open purges it.
	db = open()
	if err := db.SetSuiteEnabled(suiteIDByKeyFromStore(t, db, "mmlu"), false); err != nil {
		t.Fatalf("disable mmlu: %v", err)
	}
	db.Close()

	db = open()
	after := shape(db)
	db.Close()
	if _, present := after["mmlu"]; present {
		t.Errorf("mmlu survived the purge after an admin disabled it: %v", after)
	}
	if len(after) != len(benchmarkRotation)-1 {
		t.Fatalf("suites after admin-disable purge = %d, want %d: %v", len(after), len(benchmarkRotation)-1, after)
	}

	// And it never comes back: the tombstone outlives further Opens.
	db = open()
	final := shape(db)
	db.Close()
	if _, present := final["mmlu"]; present {
		t.Errorf("mmlu resurrected on a later open (tombstone lost): %v", final)
	}
}
