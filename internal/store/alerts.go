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
// rebuild (LatestDownRecoveryEvent filters by that whitelist). "batch"
// (spec 0017, ADR 0014) records one aggregated window flush: the actual
// text sent and the real delivery result; it carries a NULL endpoint_id.
// "group_down" / "group_recovered" (spec 0017 ticket 3, GH #66) record a
// vendor (family) group alert opening/closing: endpoint_id is NULL and the
// family name rides group_key; they join only the group state rebuild
// (LatestGroupEvent), never the per-endpoint one.
const (
	AlertKindDown             = "down"
	AlertKindRecovered        = "recovered"
	AlertKindScoreDrop        = "score_drop"
	AlertKindScoreDropSkipped = "score_drop_skipped"
	AlertKindTest             = "test"
	AlertKindBatch            = "batch"
	AlertKindGroupDown        = "group_down"
	AlertKindGroupRecovered   = "group_recovered"
)

// maxAlertLimit caps how many alert events a single query may return.
const maxAlertLimit = 200

// defaultAlertLimit applies when the caller passes a non-positive limit.
const defaultAlertLimit = 50

// AlertEvent is one recorded alert: an attempted (or completed) notification.
// EndpointID is nil for events not tied to a single endpoint; GroupKey is
// non-nil only on vendor group alerts (group_down / group_recovered), where
// it carries the family name.
type AlertEvent struct {
	ID         int64
	EndpointID *int64
	Kind       string
	Message    string
	SentOK     bool
	CreatedAt  time.Time
	GroupKey   *string
}

// scanAlertEvent scans one alert_events row. EndpointID and GroupKey may be
// NULL.
func scanAlertEvent(s rowScanner) (AlertEvent, error) {
	var e AlertEvent
	var endpointID sql.NullInt64
	var groupKey sql.NullString
	var sentOK int
	var createdAt string
	if err := s.Scan(&e.ID, &endpointID, &e.Kind, &e.Message, &sentOK, &createdAt, &groupKey); err != nil {
		return AlertEvent{}, err
	}
	if endpointID.Valid {
		id := endpointID.Int64
		e.EndpointID = &id
	}
	if groupKey.Valid {
		key := groupKey.String
		e.GroupKey = &key
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
		INSERT INTO alert_events (endpoint_id, kind, message, sent_ok, created_at, group_key)
		VALUES (?, ?, ?, ?, ?, ?)
	`, e.EndpointID, e.Kind, e.Message, sentOK, createdAt.Format(time.RFC3339), e.GroupKey)
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
		SELECT id, endpoint_id, kind, message, sent_ok, created_at, group_key
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

// UpdateAlertEventsSentOK writes the delivery result back to events that
// were recorded at transition time with sent_ok=false ("delivery
// unconfirmed", spec 0017 / ADR 0014): the window flush confirms them after
// the aggregated send. A failed send leaves them false — no retry, and the
// false stays honest (the outage may never have reached Lark).
func (db *DB) UpdateAlertEventsSentOK(ids []int64, sentOK bool) error {
	v := 0
	if sentOK {
		v = 1
	}
	for _, id := range ids {
		if _, err := db.conn.Exec(`UPDATE alert_events SET sent_ok = ? WHERE id = ?`, v, id); err != nil {
			return err
		}
	}
	return nil
}

// LatestDownRecoveryEvent returns the newest down/recovered event recorded
// for an endpoint, or nil when none exists. The alert evaluator uses it to
// rebuild its in-memory "alerted" state after a restart: a trailing "down"
// event means the outage was already reported. Group events never match:
// their endpoint_id is NULL and their kinds sit outside the whitelist.
func (db *DB) LatestDownRecoveryEvent(endpointID int64) (*AlertEvent, error) {
	e, err := scanAlertEvent(db.conn.QueryRow(`
		SELECT id, endpoint_id, kind, message, sent_ok, created_at, group_key
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

// LatestGroupEvent returns the newest group_down/group_recovered event
// recorded for a vendor group (family), or nil when none exists. The alert
// evaluator uses it to rebuild the in-memory group-open state after a
// restart — the group counterpart of LatestDownRecoveryEvent, deliberately
// separate so the two state machines never pollute each other.
func (db *DB) LatestGroupEvent(groupKey string) (*AlertEvent, error) {
	e, err := scanAlertEvent(db.conn.QueryRow(`
		SELECT id, endpoint_id, kind, message, sent_ok, created_at, group_key
		FROM alert_events
		WHERE group_key = ? AND kind IN (?, ?)
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, groupKey, AlertKindGroupDown, AlertKindGroupRecovered))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// OpenGroupAlert is one vendor group whose alert is currently open: the
// latest group event for the family is a group_down.
type OpenGroupAlert struct {
	GroupKey string
	Since    time.Time // created_at of the opening group_down event
}

// ListOpenGroupAlerts returns every vendor group with an open group alert.
// It is the store seam for the quiet-hours summary (spec 0017 ticket 4):
// the summary lists still-open groups without re-deriving state in memory.
func (db *DB) ListOpenGroupAlerts() ([]OpenGroupAlert, error) {
	rows, err := db.conn.Query(`
		SELECT group_key, kind, created_at
		FROM alert_events
		WHERE kind IN (?, ?)
		ORDER BY created_at DESC, id DESC
	`, AlertKindGroupDown, AlertKindGroupRecovered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Newest-first: the first row seen per group_key is that group's latest
	// event; an open group is one whose latest event is a group_down.
	seen := map[string]bool{}
	open := []OpenGroupAlert{}
	for rows.Next() {
		var key, kind, createdAt string
		if err := rows.Scan(&key, &kind, &createdAt); err != nil {
			return nil, err
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if kind != AlertKindGroupDown {
			continue
		}
		since, _ := time.Parse(time.RFC3339, createdAt)
		open = append(open, OpenGroupAlert{GroupKey: key, Since: since})
	}
	return open, rows.Err()
}
