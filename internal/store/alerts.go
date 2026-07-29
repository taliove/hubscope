package store

import (
	"database/sql"
	"time"
)

// Alert kinds emitted by the alert evaluator. "score_drop" fires when a
// model's aggregate falls beyond the threshold between two campaigns;
// "score_drop_skipped" records that a comparison was skipped because the two
// runs ran different suite versions (nothing is sent for a skip). "test" is
// the manual channel check from POST /api/settings/test-lark (ticket 100):
// it carries a NULL endpoint_id and never joins the down/recovered state
// rebuild (LatestDownRecoveryEvent filters by that whitelist).
const (
	AlertKindDown             = "down"
	AlertKindRecovered        = "recovered"
	AlertKindScoreDrop        = "score_drop"
	AlertKindScoreDropSkipped = "score_drop_skipped"
	AlertKindTest             = "test"
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

// ListAlertEventsAll returns alert events newest first, including hub-less
// events (score_drop / score_drop_skipped have a NULL endpoint_id). It is
// the super_admin / store-internal counterpart of ListAlertEventsByHub.
func (db *DB) ListAlertEventsAll(limit int) ([]AlertEvent, error) {
	return db.listAlertEvents(limit, 0)
}

// ListAlertEventsByHub returns endpoint-bound alert events for a single hub,
// newest first. score_drop / score_drop_skipped events (endpoint_id IS NULL)
// have no hub ownership and are excluded — they belong to the global *All
// view. Correct endpoint->hub attribution via a model_db_id column is
// tracked by a follow-up ticket; the endpoint_id JOIN through endpoints ->
// models is the correct path today.
func (db *DB) ListAlertEventsByHub(hubID int64, limit int) ([]AlertEvent, error) {
	return db.listAlertEvents(limit, hubID)
}

// listAlertEvents is the shared implementation. hubID is 0 for the unscoped
// (all) variant — hub IDs are AUTOINCREMENT from 1, so 0 never matches — or
// the hubID parameter for the hub-scoped variant.
func (db *DB) listAlertEvents(limit int, hubID int64) ([]AlertEvent, error) {
	if limit <= 0 {
		limit = defaultAlertLimit
	}
	if limit > maxAlertLimit {
		limit = maxAlertLimit
	}

	hubFilter := ""
	var args []interface{}
	if hubID != 0 {
		// Hub-scoped: only endpoint-bound alerts whose endpoint belongs to
		// hubID. score_drop / score_drop_skipped events (endpoint_id IS NULL)
		// have no hub ownership and are excluded — they belong to the
		// global *All view (super_admin only).
		hubFilter = `WHERE endpoint_id IN (
			SELECT e.id FROM endpoints e
			JOIN models m ON m.id = e.model_id
			WHERE m.hub_id = ?
		)`
		args = append(args, hubID)
	}
	args = append(args, limit)

	rows, err := db.conn.Query(`
		SELECT id, endpoint_id, kind, message, sent_ok, created_at
		FROM alert_events
		`+hubFilter+`
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, args...)
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
