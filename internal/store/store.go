package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the database connection and provides data access methods
type DB struct {
	conn *sql.DB
}

// Open opens a SQLite database at the given path and runs migrations
func Open(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite permits only one writer at a time; a single connection
	// serializes access and avoids SQLITE_BUSY under concurrent use
	// (e.g. the scheduler writing probes while the API serves reads).
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate creates all required tables
func (db *DB) migrate() error {
	schema := `
		CREATE TABLE IF NOT EXISTS hubs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			token TEXT NOT NULL,
			sync_status TEXT NOT NULL DEFAULT 'idle',
			last_synced_at TEXT NULL,
			last_sync_error TEXT NULL,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hub_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			origin TEXT NOT NULL DEFAULT 'manual',
			status TEXT NOT NULL DEFAULT 'active',
			capability TEXT NOT NULL DEFAULT 'chat',
			family TEXT NOT NULL DEFAULT 'other',
			created_at TEXT NOT NULL,
			FOREIGN KEY (hub_id) REFERENCES hubs(id),
			UNIQUE(hub_id, model_id)
		);

		CREATE TABLE IF NOT EXISTS endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id INTEGER NOT NULL,
			protocol TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			FOREIGN KEY (model_id) REFERENCES models(id)
		);

		CREATE TABLE IF NOT EXISTS probes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint_id INTEGER NOT NULL,
			streaming INTEGER NOT NULL,
			ok INTEGER NOT NULL,
			http_status INTEGER NOT NULL,
			error_summary TEXT,
			latency_ms INTEGER NOT NULL,
			ttft_ms INTEGER,
			input_tokens INTEGER,
			output_tokens INTEGER,
			created_at TEXT NOT NULL,
			FOREIGN KEY (endpoint_id) REFERENCES endpoints(id)
		);

		CREATE INDEX IF NOT EXISTS idx_probes_endpoint_time ON probes(endpoint_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS probe_rollups (
			endpoint_id INTEGER NOT NULL,
			streaming INTEGER NOT NULL,
			bucket_start TEXT NOT NULL,
			total INTEGER NOT NULL,
			failures INTEGER NOT NULL,
			p50_ms REAL,
			p95_ms REAL,
			avg_ttft_ms REAL,
			UNIQUE(endpoint_id, streaming, bucket_start)
		);

		CREATE TABLE IF NOT EXISTS rollup_watermarks (
			endpoint_id INTEGER PRIMARY KEY,
			rolled_up_to TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS suites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1
		);

		CREATE TABLE IF NOT EXISTS cases (
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
			created_at TEXT NOT NULL,
			FOREIGN KEY (suite_id) REFERENCES suites(id)
		);

		CREATE INDEX IF NOT EXISTS idx_cases_suite ON cases(suite_id);

		CREATE TABLE IF NOT EXISTS campaigns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			"trigger" TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS campaign_models (
			campaign_id INTEGER NOT NULL,
			model_id INTEGER NOT NULL,
			PRIMARY KEY (campaign_id, model_id),
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id),
			FOREIGN KEY (model_id) REFERENCES models(id)
		);

		CREATE TABLE IF NOT EXISTS eval_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			campaign_id INTEGER NOT NULL,
			suite_id INTEGER NOT NULL,
			suite_version INTEGER NOT NULL DEFAULT 1,
			"trigger" TEXT NOT NULL,
			judge_model TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id),
			FOREIGN KEY (suite_id) REFERENCES suites(id)
		);

		CREATE TABLE IF NOT EXISTS eval_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			eval_run_id INTEGER NOT NULL,
			model_db_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			case_id INTEGER NOT NULL,
			answer_text TEXT,
			score REAL,
			verdict_detail TEXT,
			verdict_profile TEXT NOT NULL DEFAULT 'v1',
			latency_ms INTEGER NOT NULL,
			input_tokens INTEGER,
			output_tokens INTEGER,
			created_at TEXT NOT NULL,
			FOREIGN KEY (eval_run_id) REFERENCES eval_runs(id)
		);

		CREATE INDEX IF NOT EXISTS idx_eval_results_run ON eval_results(eval_run_id);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS classification_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dimension TEXT NOT NULL,
			keyword TEXT NOT NULL,
			category TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 100,
			created_at TEXT NOT NULL,
			UNIQUE(dimension, keyword)
		);

		CREATE TABLE IF NOT EXISTS image_param_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword TEXT NOT NULL,
			params TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 100,
			created_at TEXT NOT NULL,
			UNIQUE(keyword)
		);

		CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			at TEXT NOT NULL,
			actor TEXT NOT NULL,
			ip TEXT NOT NULL,
			action TEXT NOT NULL,
			object_type TEXT NOT NULL,
			object_id TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_audit_logs_at ON audit_logs(at DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action, at DESC);

		CREATE TABLE IF NOT EXISTS alert_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint_id INTEGER,
			kind TEXT NOT NULL,
			message TEXT NOT NULL,
			sent_ok INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			group_key TEXT NULL,
			FOREIGN KEY (endpoint_id) REFERENCES endpoints(id)
		);

		CREATE INDEX IF NOT EXISTS idx_alert_events_endpoint ON alert_events(endpoint_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			source TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			started_at TEXT NULL,
			finished_at TEXT NULL,
			created_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_tasks_type_status ON tasks(type, status, id DESC);

		CREATE TABLE IF NOT EXISTS task_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			at TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id)
		);

		CREATE INDEX IF NOT EXISTS idx_task_logs_task ON task_logs(task_id, id);

		CREATE TABLE IF NOT EXISTS share_links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token TEXT NOT NULL UNIQUE,
			campaign_id INTEGER NOT NULL,
			created_by TEXT NOT NULL,
			created_at TEXT NOT NULL,
			revoked_at TEXT,
			FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
		);

		CREATE INDEX IF NOT EXISTS idx_share_links_campaign ON share_links(campaign_id);

		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			hub_id INTEGER NULL,
			role TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			FOREIGN KEY (hub_id) REFERENCES hubs(id)
		);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Idempotent column migrations for databases created by older versions.
	if err := db.ensureColumn("endpoints", "interval_seconds", "INTEGER NULL"); err != nil {
		return err
	}
	if err := db.ensureColumn("hubs", "sync_status", "TEXT NOT NULL DEFAULT 'idle'"); err != nil {
		return err
	}
	if err := db.ensureColumn("hubs", "last_synced_at", "TEXT NULL"); err != nil {
		return err
	}
	if err := db.ensureColumn("hubs", "last_sync_error", "TEXT NULL"); err != nil {
		return err
	}
	if err := db.ensureColumn("models", "family", "TEXT NOT NULL DEFAULT 'other'"); err != nil {
		return err
	}
	// Ticket 21: suite versioning + per-case sampling.
	if err := db.ensureColumn("suites", "version", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := db.ensureColumn("cases", "difficulty", "TEXT NOT NULL DEFAULT 'basic'"); err != nil {
		return err
	}
	if err := db.ensureColumn("cases", "sample_count", "INTEGER NULL"); err != nil {
		return err
	}
	// Ticket 97 (spec 0014 decision C): IFEval cases carry structured check
	// parameters (instruction id + kwargs) as a JSON array; NULL for every
	// other verdict shape, so pre-existing rows need no backfill.
	if err := db.ensureColumn("cases", "check_params", "TEXT NULL"); err != nil {
		return err
	}
	if err := db.ensureColumn("eval_runs", "suite_version", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	// Ticket 49 (ADR 0008): every result records the verdict profile it was
	// scored with; the DEFAULT backfills pre-existing rows to the legacy v1
	// caliber without rewriting them.
	if err := db.ensureColumn("eval_results", "verdict_profile", "TEXT NOT NULL DEFAULT 'v1'"); err != nil {
		return err
	}
	// Token usage columns (added to the create-table DDL alongside latency):
	// nullable by design — a hub that does not report usage leaves them NULL,
	// and cost sums count those NULLs as 0 (GH #42). Backfilling old
	// databases with the columns is what lets the cost aggregates read every
	// historical result without a special case.
	if err := db.ensureColumn("eval_results", "input_tokens", "INTEGER NULL"); err != nil {
		return err
	}
	if err := db.ensureColumn("eval_results", "output_tokens", "INTEGER NULL"); err != nil {
		return err
	}
	// Ticket 50 (ADR 0010): question-bank v3 organizes suites by capability,
	// stores a per-suite nadir constant for normalized scoring (backfilled 0,
	// which degenerates to the legacy raw-mean caliber), and adds an enabled
	// flag so retired suites stay readable but leave the evaluation rotation.
	// eval_runs snapshots the suite's nadir next to suite_version, keeping
	// historical runs on the nadir they were actually scored with.
	if err := db.ensureColumn("suites", "capability", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := db.ensureColumn("suites", "nadir", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.ensureColumn("suites", "enabled", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := db.ensureColumn("eval_runs", "nadir", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Ticket 29: every run belongs to a campaign. The column arrives as
	// NOT NULL DEFAULT 0 (SQLite cannot add a bare NOT NULL column); the
	// backfill below wraps each pre-existing run in its own migration
	// campaign, so 0 never survives as a dangling reference.
	if err := db.ensureColumn("eval_runs", "campaign_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.backfillRunCampaigns(); err != nil {
		return err
	}
	// Ticket 53: campaigns created before the membership snapshot existed
	// get their members backfilled from recorded results. Runs after the
	// run-campaign backfill so no member points at the sentinel campaign 0.
	if err := db.backfillCampaignMembers(); err != nil {
		return err
	}
	// The campaign index lives outside the schema block: on pre-campaign
	// databases the column only exists after the ensureColumn above.
	if _, err := db.conn.Exec(
		"CREATE INDEX IF NOT EXISTS idx_eval_runs_campaign ON eval_runs(campaign_id)",
	); err != nil {
		return err
	}

	// GH #66 (spec 0017 ticket 3): vendor group alerts carry the family name
	// in group_key (endpoint_id NULL on group_down/group_recovered events).
	// NULL on every pre-existing row, so the upgrade is lossless.
	if err := db.ensureColumn("alert_events", "group_key", "TEXT NULL"); err != nil {
		return err
	}

	// A hub left 'syncing' means the process died mid-sync (the in-flight
	// guard is in-memory only); mark it failed so the UI does not show a
	// phantom running sync.
	if _, err := db.conn.Exec(
		"UPDATE hubs SET sync_status = 'failed', last_sync_error = 'sync interrupted by restart' WHERE sync_status = 'syncing'",
	); err != nil {
		return err
	}

	// Ticket 66: every audit entry is stamped with the actor's hub_id so the
	// listing can be hub-isolated (spec 0005 per-hub isolation extended to the
	// audit log). super_admin actions that target no single hub (hub.create,
	// settings.update, classification rules, case edits, auth.login
	// user-not-found) write NULL, which only super_admin can read. Historical
	// rows backfill NULL (equivalent to super_admin-visible), so old data is
	// neither lost nor wrongly hidden from the only readers it ever had.
	if err := db.ensureColumn("audit_logs", "hub_id", "INTEGER NULL"); err != nil {
		return err
	}

	// Tasks left pending/running mean the process died mid-execution; close
	// them out as failed so the task center shows no phantom running jobs.
	if _, err := db.conn.Exec(
		"UPDATE tasks SET status = 'failed', finished_at = ? WHERE status IN ('pending', 'running')",
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}

	// Eval runs left running mean the process died mid-run; close them out as
	// failed so campaign progress never shows phantom running members. This
	// precedes the campaigns cleanup so both views of the batch agree.
	if _, err := db.conn.Exec(
		"UPDATE eval_runs SET status = 'failed', finished_at = ? WHERE status = 'running'",
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}

	// Campaigns left pending/running mean the process died mid-batch; close
	// them out as failed, mirroring the tasks cleanup above.
	if _, err := db.conn.Exec(
		"UPDATE campaigns SET status = 'failed', finished_at = ? WHERE status IN ('pending', 'running')",
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}

	// Seed the built-in evaluation suites and default classification rules on
	// first run (no-op afterwards), then re-run classification so upgraded
	// databases pick up the current rule set.
	if err := db.seedSuites(); err != nil {
		return err
	}
	// Ticket 99 (spec 0014 decision C): the benchmark cutover flips the five
	// authoritative-benchmark suites into the rotation. Runs after seedSuites
	// (the suites must exist) and BEFORE the purge: databases that seeded
	// them disabled under tickets 94-98 would otherwise lose them to the
	// purge in the same boot that retires the v3 suites.
	if err := db.enableBenchmarkSuitesAtCutover(); err != nil {
		return err
	}
	// Mid-state fallback of the same cutover (GH #15): retire the v3 suites
	// unconditionally, so databases whose generation-tracked retirement was
	// short-circuited by pre-written purge tombstones still hand the v3
	// suites to the purge below. Idempotent — steady state matches zero rows.
	if err := db.retireV3SuitesAtOpen(); err != nil {
		return err
	}
	// Ticket 93 (spec 0014 decision B, ADR 0012): disabled suites are
	// hard-deleted with their cases, runs and results. Runs after seedSuites
	// so a first-time retirement (retireAtGen — including the v3 retirement
	// at the ticket-99 cutover) is purged in the same boot; idempotent, so
	// every later Open is a no-op.
	if err := db.purgeDisabledSuites(); err != nil {
		return err
	}
	if err := db.seedClassificationRules(); err != nil {
		return err
	}
	// Image-probe cost-saving parameter rules (spec 0014 / GH #33): an
	// independent one-shot flag, so the two rule tables seed separately.
	if err := db.seedImageParamRules(); err != nil {
		return err
	}
	return db.ReclassifyAll()
}

// ensureColumn adds a column to an existing table when it is missing. It
// inspects PRAGMA table_info first so the migration is safe to run on every
// startup. All arguments are internal constants, never user input.
func (db *DB) ensureColumn(table, column, decl string) error {
	rows, err := db.conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// backfillRunCampaigns wraps every pre-campaign-era eval run (campaign_id 0)
// in its own single-run migration campaign, so the NOT NULL campaign_id
// invariant holds for historical data. The migration campaign inherits the
// run's trigger, status and timestamps: a done run yields a done campaign.
// Idempotent — every backfilled run leaves the campaign_id = 0 bucket.
func (db *DB) backfillRunCampaigns() error {
	rows, err := db.conn.Query(`
		SELECT id, "trigger", status, started_at, finished_at
		FROM eval_runs WHERE campaign_id = 0 ORDER BY id
	`)
	if err != nil {
		return err
	}
	type orphanRun struct {
		id         int64
		trigger    string
		status     string
		startedAt  string
		finishedAt sql.NullString
	}
	var orphans []orphanRun
	for rows.Next() {
		var o orphanRun
		if err := rows.Scan(&o.id, &o.trigger, &o.status, &o.startedAt, &o.finishedAt); err != nil {
			rows.Close()
			return err
		}
		orphans = append(orphans, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(orphans) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, o := range orphans {
		result, err := tx.Exec(`
			INSERT INTO campaigns ("trigger", status, started_at, finished_at, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, o.trigger, o.status, o.startedAt, o.finishedAt, o.startedAt)
		if err != nil {
			return err
		}
		campaignID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE eval_runs SET campaign_id = ? WHERE id = ?", campaignID, o.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
