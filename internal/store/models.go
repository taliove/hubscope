package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Model represents a model registered on a hub
type Model struct {
	ID         int64
	HubID      int64
	ModelID    string
	Origin     string
	Status     string
	Capability string
	CreatedAt  time.Time
}

// Endpoint represents a model-protocol combination
type Endpoint struct {
	ID              int64
	ModelID         int64
	Protocol        string
	Enabled         bool
	IntervalSeconds *int // nil means the global default interval applies
	CreatedAt       time.Time
}

// endpointColumns is the canonical column list for scanning an Endpoint.
const endpointColumns = "id, model_id, protocol, enabled, interval_seconds, created_at"

// endpointColumnsAliased is endpointColumns prefixed with the endpoints
// table alias for JOIN queries.
const endpointColumnsAliased = "e.id, e.model_id, e.protocol, e.enabled, e.interval_seconds, e.created_at"

// IntervalPatch is a tri-state update to an endpoint's interval override:
// Set=false leaves it unchanged; Set=true with Value=nil clears it (back to
// the global default); Set=true with a Value sets the override.
type IntervalPatch struct {
	Set   bool
	Value *int
}

// rowScanner abstracts sql.Row and sql.Rows so one helper scans both.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanEndpoint scans a row containing endpointColumns into an Endpoint.
func scanEndpoint(s rowScanner) (Endpoint, error) {
	var e Endpoint
	var enabled int
	var interval sql.NullInt64
	var createdAt string
	if err := s.Scan(&e.ID, &e.ModelID, &e.Protocol, &enabled, &interval, &createdAt); err != nil {
		return Endpoint{}, err
	}
	e.Enabled = enabled == 1
	if interval.Valid {
		v := int(interval.Int64)
		e.IntervalSeconds = &v
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return e, nil
}

// CreateModel creates a model and its two endpoints (anthropic + openai)
func (db *DB) CreateModel(hubID int64, modelID string) (*Model, error) {
	now := time.Now().UTC()

	// Start transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert model
	result, err := tx.Exec(
		"INSERT INTO models (hub_id, model_id, origin, status, capability, created_at) VALUES (?, ?, 'manual', 'active', 'chat', ?)",
		hubID, modelID, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}

	modelDBID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Create two endpoints
	for _, protocol := range []string{"anthropic", "openai"} {
		_, err = tx.Exec(
			"INSERT INTO endpoints (model_id, protocol, enabled, created_at) VALUES (?, ?, 1, ?)",
			modelDBID, protocol, now.Format(time.RFC3339),
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Model{
		ID:         modelDBID,
		HubID:      hubID,
		ModelID:    modelID,
		Origin:     "manual",
		Status:     "active",
		Capability: "chat",
		CreatedAt:  now,
	}, nil
}

// GetModel retrieves a model by ID
func (db *DB) GetModel(id int64) (*Model, error) {
	var m Model
	var createdAt string
	err := db.conn.QueryRow(`
		SELECT id, hub_id, model_id, origin, status, capability, created_at
		FROM models
		WHERE id = ?
	`, id).Scan(&m.ID, &m.HubID, &m.ModelID, &m.Origin, &m.Status, &m.Capability, &createdAt)
	if err != nil {
		return nil, err
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &m, nil
}

// SetModelCapability updates a model's capability tag ("chat" / "non_chat").
// Discovery (ticket 05) sets this from the model list; eval eligibility
// checks read it.
func (db *DB) SetModelCapability(id int64, capability string) error {
	_, err := db.conn.Exec("UPDATE models SET capability = ? WHERE id = ?", capability, id)
	return err
}

// ListModels returns all models with their endpoints
func (db *DB) ListModels() ([]Model, error) {
	rows, err := db.conn.Query(`
		SELECT id, hub_id, model_id, origin, status, capability, created_at
		FROM models
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []Model
	for rows.Next() {
		var m Model
		var createdAt string
		if err := rows.Scan(&m.ID, &m.HubID, &m.ModelID, &m.Origin, &m.Status, &m.Capability, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		models = append(models, m)
	}

	return models, rows.Err()
}

// ListEndpointsByModelID returns all endpoints for a model
func (db *DB) ListEndpointsByModelID(modelID int64) ([]Endpoint, error) {
	rows, err := db.conn.Query(`
		SELECT `+endpointColumns+`
		FROM endpoints
		WHERE model_id = ?
		ORDER BY id
	`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		e, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}

	return endpoints, rows.Err()
}

// ListEnabledEndpoints returns enabled endpoints whose model is active. The
// scheduler re-reads this on every tick so PATCH changes take effect on the
// next round.
func (db *DB) ListEnabledEndpoints() ([]Endpoint, error) {
	rows, err := db.conn.Query(`
		SELECT `+endpointColumnsAliased+`
		FROM endpoints e
		JOIN models m ON m.id = e.model_id
		WHERE e.enabled = 1 AND m.status = 'active'
		ORDER BY e.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		e, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, e)
	}

	return endpoints, rows.Err()
}

// GetEndpoint retrieves an endpoint by ID
func (db *DB) GetEndpoint(id int64) (*Endpoint, error) {
	e, err := scanEndpoint(db.conn.QueryRow(
		"SELECT "+endpointColumns+" FROM endpoints WHERE id = ?",
		id,
	))
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// UpdateEndpoint applies a partial update to an endpoint and returns the
// stored copy. enabled is unchanged when nil; interval is a tri-state
// IntervalPatch (see its docs).
func (db *DB) UpdateEndpoint(id int64, enabled *bool, interval IntervalPatch) (*Endpoint, error) {
	updates := []string{}
	args := []interface{}{}

	if enabled != nil {
		v := 0
		if *enabled {
			v = 1
		}
		updates = append(updates, "enabled = ?")
		args = append(args, v)
	}
	if interval.Set {
		updates = append(updates, "interval_seconds = ?")
		if interval.Value == nil {
			args = append(args, nil)
		} else {
			args = append(args, *interval.Value)
		}
	}

	if len(updates) == 0 {
		return db.GetEndpoint(id)
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE endpoints SET %s WHERE id = ?", strings.Join(updates, ", "))
	if _, err := db.conn.Exec(query, args...); err != nil {
		return nil, err
	}

	return db.GetEndpoint(id)
}
