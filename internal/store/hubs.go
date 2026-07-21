package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrHubHasModels is returned when deleting a hub that still has models.
var ErrHubHasModels = errors.New("hub has models")

// Hub represents a hub instance
type Hub struct {
	ID        int64
	Name      string
	BaseURL   string
	Token     string
	CreatedAt time.Time
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
		ID:        id,
		Name:      name,
		BaseURL:   baseURL,
		Token:     token,
		CreatedAt: now,
	}, nil
}

// ListHubs returns all hubs
func (db *DB) ListHubs() ([]Hub, error) {
	rows, err := db.conn.Query("SELECT id, name, base_url, token, created_at FROM hubs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hubs []Hub
	for rows.Next() {
		var h Hub
		var createdAt string
		if err := rows.Scan(&h.ID, &h.Name, &h.BaseURL, &h.Token, &createdAt); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		hubs = append(hubs, h)
	}

	return hubs, rows.Err()
}

// GetHub retrieves a hub by ID
func (db *DB) GetHub(id int64) (*Hub, error) {
	var h Hub
	var createdAt string
	err := db.conn.QueryRow(
		"SELECT id, name, base_url, token, created_at FROM hubs WHERE id = ?",
		id,
	).Scan(&h.ID, &h.Name, &h.BaseURL, &h.Token, &createdAt)
	if err != nil {
		return nil, err
	}
	h.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
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
