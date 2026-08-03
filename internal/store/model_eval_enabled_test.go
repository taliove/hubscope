package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestEvalEnabledMigrationDefaultsOn pins the GH #170 additive migration:
// a database created before the eval_enabled column existed opens cleanly,
// every pre-existing model reads back eval-enabled (the upgrade is
// opt-out), the model joins the sweep candidate set, and a later flip
// persists.
func TestEvalEnabledMigrationDefaultsOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Stage a pre-GH #170 database: the models table has no eval_enabled
	// column (same fixture discipline as the server's pre-campaign_models
	// staging — the DDL only has to look like a database that predates the
	// column).
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
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
		CREATE TABLE endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id INTEGER NOT NULL,
			protocol TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		);
	`
	if _, err := conn.Exec(ddl); err != nil {
		t.Fatalf("stage legacy schema: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := conn.Exec("INSERT INTO hubs (name, base_url, token, created_at) VALUES ('h', 'http://h', 'tok', ?)", now); err != nil {
		t.Fatalf("stage hub: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO models (hub_id, model_id, origin, status, capability, family, created_at) VALUES (1, 'legacy-model', 'manual', 'active', 'chat', 'gpt', ?)", now); err != nil {
		t.Fatalf("stage model: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO endpoints (model_id, protocol, enabled, created_at) VALUES (1, 'openai', 1, ?)", now); err != nil {
		t.Fatalf("stage endpoint: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	// Opening through the store migrates the schema in place.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	model, err := db.GetModel(1)
	if err != nil {
		t.Fatalf("get legacy model: %v", err)
	}
	if !model.EvalEnabled {
		t.Error("pre-migration model eval_enabled = false, want true (opt-out upgrade: existing rows default on)")
	}
	ids, err := db.ListActiveChatModelIDs()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(ids) != 1 || ids[0] != model.ID {
		t.Errorf("sweep candidates = %v, want [%d] (migrated model stays in the population)", ids, model.ID)
	}

	// The flip persists and shrinks the candidate set.
	if err := db.SetModelEvalEnabled(model.ID, false); err != nil {
		t.Fatalf("set eval_enabled: %v", err)
	}
	model, err = db.GetModel(1)
	if err != nil {
		t.Fatalf("re-get model: %v", err)
	}
	if model.EvalEnabled {
		t.Error("after SetModelEvalEnabled(false): eval_enabled still true")
	}
	ids, err = db.ListActiveChatModelIDs()
	if err != nil {
		t.Fatalf("re-list candidates: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("sweep candidates after opt-out = %v, want empty", ids)
	}
}
