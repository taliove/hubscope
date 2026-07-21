package store

import (
	"database/sql"
	"time"
)

// Alert kinds emitted by the alert evaluator. "score_drop" is reserved for
// ticket 09 and never produced by probe alerting.
const (
	AlertKindDown      = "down"
	AlertKindRecovered = "recovered"
	AlertKindScoreDrop = "score_drop"
)

// maxAlertLimit caps how many alert events a single query may return.
const maxAlertLimit = 200

// defaultAlertLimit applies when the caller passes a non-positive limit.
const defaultAlertLimit = 50

// AlertEvent is one recorded alert: an attempted (or completed) notification.
// EndpointID is nil for events not tied to a single endpoint.
type AlertEvent struct {
	ID         int64
	EndpointID *int64
	Kind       string
	Message    string
	SentOK     bool
	CreatedAt  time.Time
}

// scanAlertEvent scans one alert_events row. EndpointID may be NULL.
func scanAlertEvent(s rowScanner) (AlertEvent, error) {
	var e AlertEvent
	var endpointID sql.NullInt64
	var sentOK int
	var createdAt string
	if err := s.Scan(&e.ID, &endpointID, &e.Kind, &e.Message, &sentOK, &createdAt); err != nil {
		return AlertEvent{}, err
	}
	if endpointID.Valid {
		id := endpointID.Int64
		e.EndpointID = &id
	}
	e.SentOK = sentOK == 1
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return e, nil
}

// CreateAlertEvent inserts an alert event and returns the stored copy. The
// input is not mutated. A zero CreatedAt is replaced with the current time.
func (db *DB) CreateAlertEvent(e AlertEvent) (AlertEvent, error) {
	createdAt := e.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	sentOK := 0
	if e.SentOK {
		sentOK = 1
	}

	result, err := db.conn.Exec(`
		INSERT INTO alert_events (endpoint_id, kind, message, sent_ok, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, e.EndpointID, e.Kind, e.Message, sentOK, createdAt.Format(time.RFC3339))
	if err != nil {
		return AlertEvent{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return AlertEvent{}, err
	}
	e.ID = id
	e.CreatedAt = createdAt
	return e, nil
}

// ListAlertEvents returns alert events newest first.
func (db *DB) ListAlertEvents(limit int) ([]AlertEvent, error) {
	if limit <= 0 {
		limit = defaultAlertLimit
	}
	if limit > maxAlertLimit {
		limit = maxAlertLimit
	}

	rows, err := db.conn.Query(`
		SELECT id, endpoint_id, kind, message, sent_ok, created_at
		FROM alert_events
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []AlertEvent{}
	for rows.Next() {
		e, err := scanAlertEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// LatestDownRecoveryEvent returns the newest down/recovered event recorded
// for an endpoint, or nil when none exists. The alert evaluator uses it to
// rebuild its in-memory "alerted" state after a restart: a trailing "down"
// event means the outage was already reported.
func (db *DB) LatestDownRecoveryEvent(endpointID int64) (*AlertEvent, error) {
	e, err := scanAlertEvent(db.conn.QueryRow(`
		SELECT id, endpoint_id, kind, message, sent_ok, created_at
		FROM alert_events
		WHERE endpoint_id = ? AND kind IN (?, ?)
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, endpointID, AlertKindDown, AlertKindRecovered))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
