package store

import (
	"strings"
	"time"
)

// CreateDiscoveredModel registers a model found via hub discovery. When the
// model already exists (any origin) it is not duplicated: the stored row is
// returned with created=false, a retired row is reactivated to 'active'
// because it reappeared in the hub listing, and its classification is
// refreshed so rule changes reach existing models on every sync. Manual
// models keep their origin.
func (db *DB) CreateDiscoveredModel(hubID int64, modelID, capability, family string) (*Model, bool, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(
		"INSERT INTO models (hub_id, model_id, origin, status, capability, family, created_at) VALUES (?, ?, 'discovered', 'active', ?, ?, ?)",
		hubID, modelID, capability, family, now.Format(time.RFC3339),
	)
	if err == nil {
		id, err := result.LastInsertId()
		if err != nil {
			return nil, false, err
		}
		return &Model{
			ID:         id,
			HubID:      hubID,
			ModelID:    modelID,
			Origin:     "discovered",
			Status:     "active",
			Capability: capability,
			Family:     family,
			CreatedAt:  now,
		}, true, nil
	}
	if !isUniqueViolation(err) {
		return nil, false, err
	}

	// Already registered. Reactivate when retired, refresh the
	// classification, then return the stored row.
	if _, err := db.conn.Exec(
		"UPDATE models SET status = 'active', capability = ?, family = ? WHERE hub_id = ? AND model_id = ?",
		capability, family, hubID, modelID,
	); err != nil {
		return nil, false, err
	}
	existing, err := db.getModelByModelID(hubID, modelID)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

// MarkRetiredMissing marks every discovered, still-active model of the hub
// whose model_id is not in seenIDs as 'retired' and returns the number of
// rows affected. Manual models are never touched. An empty seenIDs is
// treated as "list fetch anomaly" and retires nothing — a hub returning
// 200 with an empty model list must not wipe out every discovered model.
func (db *DB) MarkRetiredMissing(hubID int64, seenIDs []string) (int, error) {
	if len(seenIDs) == 0 {
		return 0, nil
	}
	query := "UPDATE models SET status = 'retired' WHERE hub_id = ? AND origin = 'discovered' AND status = 'active'"
	args := []interface{}{hubID}
	if len(seenIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(seenIDs)), ",")
		query += " AND model_id NOT IN (" + placeholders + ")"
		for _, id := range seenIDs {
			args = append(args, id)
		}
	}

	result, err := db.conn.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// CreateEndpoint inserts one endpoint for a model with the given enabled
// state. Discovery uses it to record per-protocol probe outcomes: reachable
// protocols start enabled, unreachable ones start disabled but stay visible.
func (db *DB) CreateEndpoint(modelID int64, protocol string, enabled bool) (*Endpoint, error) {
	now := time.Now().UTC()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	result, err := db.conn.Exec(
		"INSERT INTO endpoints (model_id, protocol, enabled, created_at) VALUES (?, ?, ?, ?)",
		modelID, protocol, enabledInt, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		ID:        id,
		ModelID:   modelID,
		Protocol:  protocol,
		Enabled:   enabled,
		CreatedAt: now,
	}, nil
}

// getModelByModelID fetches a model by its (hub_id, model_id) unique key.
func (db *DB) getModelByModelID(hubID int64, modelID string) (*Model, error) {
	m, err := scanModel(db.conn.QueryRow(
		"SELECT "+modelColumns+" FROM models WHERE hub_id = ? AND model_id = ?",
		hubID, modelID,
	))
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
