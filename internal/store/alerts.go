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
// (LatestGroupEvent), never the per-endpoint one. "quiet_summary" (spec 0017
// ticket 4, GH #67) records one quiet-hours end summary: the actual text
// sent and the real delivery result; it carries a NULL endpoint_id.
const (
	AlertKindDown             = "down"
	AlertKindRecovered        = "recovered"
	AlertKindScoreDrop        = "score_drop"
	AlertKindScoreDropSkipped = "score_drop_skipped"
	AlertKindTest             = "test"
	AlertKindBatch            = "batch"
	AlertKindGroupDown        = "group_down"
	AlertKindGroupRecovered   = "group_recovered"
	AlertKindQuietSummary     = "quiet_summary"
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

// ConfirmedAlertEvents returns the subset of the given event IDs whose
// delivery was confirmed (sent_ok=true). The window flush uses it to drop
// transitions a quiet-hours summary already reported between decision and
// flush, so the same story is never sent twice (spec 0017 ticket 4, GH #67).
func (db *DB) ConfirmedAlertEvents(ids []int64) (map[int64]bool, error) {
	confirmed := map[int64]bool{}
	for _, id := range ids {
		var sentOK int
		if err := db.conn.QueryRow(`SELECT sent_ok FROM alert_events WHERE id = ?`, id).Scan(&sentOK); err != nil {
			return nil, err
		}
		if sentOK == 1 {
			confirmed[id] = true
		}
	}
	return confirmed, nil
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
// latest group event for the family is a group_down (the anchor). EventID
// and SentOK describe that anchor event — the quiet-hours summary confirms
// anchors whose delivery was never confirmed.
type OpenGroupAlert struct {
	GroupKey string
	EventID  int64     // id of the anchor group_down event
	SentOK   bool      // delivery state of the anchor event
	Since    time.Time // created_at of the opening group_down event
}

// ListOpenGroupAlerts returns every vendor group with an open group alert.
// It is the store seam for the quiet-hours summary (spec 0017 ticket 4):
// the summary lists still-open groups without re-deriving state in memory.
func (db *DB) ListOpenGroupAlerts() ([]OpenGroupAlert, error) {
	rows, err := db.conn.Query(`
		SELECT id, group_key, kind, sent_ok, created_at
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
		var id int64
		var key, kind, createdAt string
		var sentOK int
		if err := rows.Scan(&id, &key, &kind, &sentOK, &createdAt); err != nil {
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
		open = append(open, OpenGroupAlert{GroupKey: key, EventID: id, SentOK: sentOK == 1, Since: since})
	}
	return open, rows.Err()
}

// OpenEndpointAlert is one endpoint whose down alert is currently open: the
// latest down/recovered event for the endpoint is a down (the anchor).
// EventID and SentOK describe that anchor — the quiet-hours summary reports
// still-open endpoints whose anchor delivery was never confirmed (sent_ok
// false), which keeps the derivation purely event-based: a restart loses no
// state, and an alert already delivered before the quiet window is not
// repeated by the summary.
type OpenEndpointAlert struct {
	EndpointID int64
	EventID    int64 // id of the anchor down event
	SentOK     bool  // delivery state of the anchor event
	HubName    string
	ModelID    string
	Protocol   string
	Family     string
	Since      time.Time // created_at of the anchor down event
}

// ListOpenEndpointAlerts returns every endpoint with an open down alert,
// joined with its hub/model identity for summary rendering. It is the
// endpoint counterpart of ListOpenGroupAlerts (spec 0017 ticket 4): the
// still-open judgment is derived from events, never from in-memory alert
// state, so a restart cannot corrupt the quiet-hours summary. Endpoints
// whose join no longer resolves (deleted endpoint/model/hub) drop out —
// there is nothing left to act on.
func (db *DB) ListOpenEndpointAlerts() ([]OpenEndpointAlert, error) {
	rows, err := db.conn.Query(`
		SELECT a.id, a.endpoint_id, a.kind, a.sent_ok, a.created_at,
		       h.name, m.model_id, e.protocol, m.family
		FROM alert_events a
		JOIN endpoints e ON e.id = a.endpoint_id
		JOIN models m ON m.id = e.model_id
		JOIN hubs h ON h.id = m.hub_id
		WHERE a.kind IN (?, ?)
		ORDER BY a.created_at DESC, a.id DESC
	`, AlertKindDown, AlertKindRecovered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Newest-first: the first row seen per endpoint is its latest event; an
	// open endpoint is one whose latest event is a down.
	seen := map[int64]bool{}
	open := []OpenEndpointAlert{}
	for rows.Next() {
		var a OpenEndpointAlert
		var kind, createdAt string
		var sentOK int
		if err := rows.Scan(&a.EventID, &a.EndpointID, &kind, &sentOK, &createdAt,
			&a.HubName, &a.ModelID, &a.Protocol, &a.Family); err != nil {
			return nil, err
		}
		if seen[a.EndpointID] {
			continue
		}
		seen[a.EndpointID] = true
		if kind != AlertKindDown {
			continue
		}
		a.SentOK = sentOK == 1
		a.Since, _ = time.Parse(time.RFC3339, createdAt)
		open = append(open, a)
	}
	return open, rows.Err()
}
