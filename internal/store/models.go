package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/classifier"
)

// Model represents a model registered on a hub
type Model struct {
	ID         int64
	HubID      int64
	ModelID    string
	Origin     string
	Status     string
	Capability string
	Family     string
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

// modelColumns is the canonical column list for scanning a Model.
const modelColumns = "id, hub_id, model_id, origin, status, capability, family, created_at"

// scanModel scans a row containing modelColumns into a Model.
func scanModel(s rowScanner) (Model, error) {
	var m Model
	var createdAt string
	if err := s.Scan(&m.ID, &m.HubID, &m.ModelID, &m.Origin, &m.Status, &m.Capability, &m.Family, &createdAt); err != nil {
		return Model{}, err
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return m, nil
}

// CreateModel creates a model with one endpoint per given protocol.
// Classification (capability + family) is derived from the current rule set.
// The caller picks the protocols: since ticket 17 only protocols that
// answered a trial probe get an endpoint.
func (db *DB) CreateModel(hubID int64, modelID string, protocols []string) (*Model, error) {
	now := time.Now().UTC()

	rules, err := db.ListClassificationRules()
	if err != nil {
		return nil, err
	}
	capability, family := classifier.Classify(modelID, rules)

	// Start transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert model
	result, err := tx.Exec(
		"INSERT INTO models (hub_id, model_id, origin, status, capability, family, created_at) VALUES (?, ?, 'manual', 'active', ?, ?, ?)",
		hubID, modelID, capability, family, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}

	modelDBID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Create one endpoint per working protocol
	for _, protocol := range protocols {
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
		Capability: capability,
		Family:     family,
		CreatedAt:  now,
	}, nil
}

// GetModel retrieves a model by ID
func (db *DB) GetModel(id int64) (*Model, error) {
	m, err := scanModel(db.conn.QueryRow(
		"SELECT "+modelColumns+" FROM models WHERE id = ?",
		id,
	))
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// SetModelCapability updates a model's capability tag ("chat" / "non_chat").
// Discovery (ticket 05) sets this from the model list; eval eligibility
// checks read it.
func (db *DB) SetModelCapability(id int64, capability string) error {
	_, err := db.conn.Exec("UPDATE models SET capability = ? WHERE id = ?", capability, id)
	return err
}

// SetModelClassification updates both classification dimensions of a model.
func (db *DB) SetModelClassification(id int64, capability, family string) error {
	_, err := db.conn.Exec("UPDATE models SET capability = ?, family = ? WHERE id = ?", capability, family, id)
	return err
}

// ListModelsAll returns every model with its endpoints. It is the only
// no-argument list form: per the per-hub query isolation invariant (spec
// 0005), list calls must take an explicit hub filter so a missing filter is a
// compile error. The All variant is reserved for super_admin paths and for
// store-internal maintenance that is genuinely global (e.g. ReclassifyAll);
// HTTP handlers must branch on the session user before reaching it.
func (db *DB) ListModelsAll() ([]Model, error) {
	rows, err := db.conn.Query("SELECT " + modelColumns + " FROM models ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}

	return models, rows.Err()
}

// ListModelsByHub returns the models that belong to a single hub, ordered by
// id. It is the hub-scoped counterpart of ListModelsAll and the form HTTP
// handlers must use for non-super_admin sessions.
func (db *DB) ListModelsByHub(hubID int64) ([]Model, error) {
	rows, err := db.conn.Query("SELECT "+modelColumns+" FROM models WHERE hub_id = ? ORDER BY id", hubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}

	return models, rows.Err()
}

// ListActiveChatModelIDs returns the database IDs of all active,
// chat-capable models that have at least one enabled endpoint — the
// population a full evaluation sweep covers. Models without an enabled
// endpoint are excluded: they cannot be called at all, so sweeping them
// would only record "no enabled endpoint" failures for every case.
func (db *DB) ListActiveChatModelIDs() ([]int64, error) {
	rows, err := db.conn.Query(`
		SELECT id FROM models
		WHERE status = 'active' AND capability = 'chat'
			AND EXISTS (SELECT 1 FROM endpoints e WHERE e.model_id = models.id AND e.enabled = 1)
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
		SELECT ` + endpointColumnsAliased + `
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
