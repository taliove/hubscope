package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrHubHasModels is returned when deleting a hub that still has models.
var ErrHubHasModels = errors.New("hub has models")

// Hub sync statuses recorded on the hub row after each discovery sync.
const (
	HubSyncIdle      = "idle"
	HubSyncRunning   = "syncing"
	HubSyncSucceeded = "succeeded"
	HubSyncFailed    = "failed"
)

// hubColumns is the canonical column list for scanning a Hub.
const hubColumns = "id, name, base_url, token, sync_status, last_synced_at, last_sync_error, created_at"

// Hub represents a hub instance
type Hub struct {
	ID            int64
	Name          string
	BaseURL       string
	Token         string
	SyncStatus    string
	LastSyncedAt  *time.Time
	LastSyncError *string
	CreatedAt     time.Time
}

// scanHub scans a row containing hubColumns into a Hub.
func scanHub(s rowScanner) (Hub, error) {
	var h Hub
	var syncedAt, syncErr sql.NullString
	var createdAt string
	if err := s.Scan(&h.ID, &h.Name, &h.BaseURL, &h.Token, &h.SyncStatus, &syncedAt, &syncErr, &createdAt); err != nil {
		return Hub{}, err
	}
	if syncedAt.Valid {
		if t, err := time.Parse(time.RFC3339, syncedAt.String); err == nil {
			h.LastSyncedAt = &t
		}
	}
	if syncErr.Valid {
		h.LastSyncError = &syncErr.String
	}
	h.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return h, nil
}

// CreateHub inserts a new hub
func (db *DB) CreateHub(name, baseURL, token string) (*Hub, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(
		"INSERT INTO hubs (name, base_url, token, created_at) VALUES (?, ?, ?, ?)",
		name, baseURL, token, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Hub{
		ID:         id,
		Name:       name,
		BaseURL:    baseURL,
		Token:      token,
		SyncStatus: HubSyncIdle,
		CreatedAt:  now,
	}, nil
}

// ListHubs returns all hubs
func (db *DB) ListHubs() ([]Hub, error) {
	rows, err := db.conn.Query("SELECT " + hubColumns + " FROM hubs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hubs []Hub
	for rows.Next() {
		h, err := scanHub(rows)
		if err != nil {
			return nil, err
		}
		hubs = append(hubs, h)
	}

	return hubs, rows.Err()
}

// GetHub retrieves a hub by ID
func (db *DB) GetHub(id int64) (*Hub, error) {
	h, err := scanHub(db.conn.QueryRow(
		"SELECT "+hubColumns+" FROM hubs WHERE id = ?",
		id,
	))
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// UpdateHub updates a hub's fields
func (db *DB) UpdateHub(id int64, name, baseURL *string, token *string) (*Hub, error) {
	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}

	if name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *name)
	}
	if baseURL != nil {
		updates = append(updates, "base_url = ?")
		args = append(args, *baseURL)
	}
	if token != nil {
		updates = append(updates, "token = ?")
		args = append(args, *token)
	}

	if len(updates) == 0 {
		return db.GetHub(id)
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE hubs SET %s WHERE id = ?", strings.Join(updates, ", "))
	_, err := db.conn.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	return db.GetHub(id)
}

// SetHubSyncing marks a hub's sync as in flight.
func (db *DB) SetHubSyncing(id int64) error {
	_, err := db.conn.Exec("UPDATE hubs SET sync_status = ? WHERE id = ?", HubSyncRunning, id)
	return err
}

// SetHubSyncResult records the outcome of a finished sync: succeeded with the
// error cleared when syncErr is nil, failed with the message otherwise.
// last_synced_at always advances — it marks the last completed attempt.
func (db *DB) SetHubSyncResult(id int64, syncErr *string) error {
	status := HubSyncSucceeded
	var errVal interface{}
	if syncErr != nil {
		status = HubSyncFailed
		errVal = *syncErr
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.conn.Exec(
		"UPDATE hubs SET sync_status = ?, last_synced_at = ?, last_sync_error = ? WHERE id = ?",
		status, now, errVal, id,
	)
	return err
}

// DeleteHub deletes a hub if it has no models
func (db *DB) DeleteHub(id int64) error {
	// Check for models
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM models WHERE hub_id = ?", id).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %d models", ErrHubHasModels, count)
	}

	_, err = db.conn.Exec("DELETE FROM hubs WHERE id = ?", id)
	return err
}
