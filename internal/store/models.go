package store

import "time"

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
	ID        int64
	ModelID   int64
	Protocol  string
	Enabled   bool
	CreatedAt time.Time
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
		SELECT id, model_id, protocol, enabled, created_at
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
		var e Endpoint
		var enabled int
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ModelID, &e.Protocol, &enabled, &createdAt); err != nil {
			return nil, err
		}
		e.Enabled = enabled == 1
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		endpoints = append(endpoints, e)
	}

	return endpoints, rows.Err()
}

// GetEndpoint retrieves an endpoint by ID
func (db *DB) GetEndpoint(id int64) (*Endpoint, error) {
	var e Endpoint
	var enabled int
	var createdAt string
	err := db.conn.QueryRow(
		"SELECT id, model_id, protocol, enabled, created_at FROM endpoints WHERE id = ?",
		id,
	).Scan(&e.ID, &e.ModelID, &e.Protocol, &enabled, &createdAt)
	if err != nil {
		return nil, err
	}
	e.Enabled = enabled == 1
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &e, nil
}
