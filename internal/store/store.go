package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

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
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hub_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			origin TEXT NOT NULL DEFAULT 'manual',
			status TEXT NOT NULL DEFAULT 'active',
			capability TEXT NOT NULL DEFAULT 'chat',
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

		CREATE TABLE IF NOT EXISTS suites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suite_id INTEGER NOT NULL,
			prompt TEXT NOT NULL,
			verdict_type TEXT NOT NULL,
			rule_mode TEXT,
			rule_expected TEXT,
			rubric TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			FOREIGN KEY (suite_id) REFERENCES suites(id)
		);

		CREATE INDEX IF NOT EXISTS idx_cases_suite ON cases(suite_id);

		CREATE TABLE IF NOT EXISTS eval_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suite_id INTEGER NOT NULL,
			"trigger" TEXT NOT NULL,
			judge_model TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
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
			latency_ms INTEGER NOT NULL,
			input_tokens INTEGER,
			output_tokens INTEGER,
			created_at TEXT NOT NULL,
			FOREIGN KEY (eval_run_id) REFERENCES eval_runs(id)
		);

		CREATE INDEX IF NOT EXISTS idx_eval_results_run ON eval_results(eval_run_id);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Idempotent column migrations for databases created by older versions.
	if err := db.ensureColumn("endpoints", "interval_seconds", "INTEGER NULL"); err != nil {
		return err
	}

	// Seed the built-in evaluation suites on first run (no-op afterwards).
	return db.seedSuites()
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
