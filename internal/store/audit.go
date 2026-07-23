package store

import (
	"database/sql"
	"time"
)

// AuditLog is one administrative action record: who (actor/IP) did what
// (action on object_type/object_id), with free-form detail and the outcome.
// HubID is the hub the actor belonged to when performing the action, or nil
// for super_admin actions that target no single hub (hub.create,
// settings.update, classification rules, case edits, auth.login
// user-not-found). It drives per-hub isolation of GET /api/audit-logs.
type AuditLog struct {
	ID         int64
	At         time.Time
	Actor      string
	IP         string
	Action     string
	ObjectType string
	ObjectID   string
	Detail     string
	Result     string
	HubID      *int64
}

// auditColumns is the canonical column list for scanning an AuditLog.
const auditColumns = "id, at, actor, ip, action, object_type, object_id, detail, result, hub_id"

// scanAudit scans a row containing auditColumns into an AuditLog.
func scanAudit(s rowScanner) (AuditLog, error) {
	var l AuditLog
	var at string
	var hubID sql.NullInt64
	if err := s.Scan(&l.ID, &at, &l.Actor, &l.IP, &l.Action, &l.ObjectType, &l.ObjectID, &l.Detail, &l.Result, &hubID); err != nil {
		return AuditLog{}, err
	}
	l.At, _ = time.Parse(time.RFC3339Nano, at)
	if hubID.Valid {
		v := hubID.Int64
		l.HubID = &v
	}
	return l, nil
}

// InsertAudit appends one audit entry, stamping it with the current time and
// the caller-supplied hub_id (nil for super_admin / hub-less actions).
func (db *DB) InsertAudit(l AuditLog) error {
	_, err := db.conn.Exec(
		"INSERT INTO audit_logs (at, actor, ip, action, object_type, object_id, detail, result, hub_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339Nano), l.Actor, l.IP, l.Action, l.ObjectType, l.ObjectID, l.Detail, l.Result, l.HubID,
	)
	return err
}

// ListAuditLogsByHub returns one page of entries stamped with hubID, newest
// first, plus their total count. action filters on an exact action when
// non-empty. Page is 1-based; pageSize is clamped by the caller. Rows with a
// NULL hub_id (super_admin / hub-less actions) are excluded — only
// ListAuditLogsAll surfaces those.
func (db *DB) ListAuditLogsByHub(hubID int64, page, pageSize int, action string) ([]AuditLog, int, error) {
	where := " WHERE hub_id = ?"
	args := []interface{}{hubID}
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
	}

	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM audit_logs"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := db.conn.Query(
		"SELECT "+auditColumns+" FROM audit_logs"+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		l, err := scanAudit(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// ListAuditLogsAll returns one page of every entry (NULL-hub rows included),
// newest first, plus the total count. Only the super_admin path reaches this
// function; hub-scoped admins go through ListAuditLogsByHub.
func (db *DB) ListAuditLogsAll(page, pageSize int, action string) ([]AuditLog, int, error) {
	where := ""
	args := []interface{}{}
	if action != "" {
		where = " WHERE action = ?"
		args = append(args, action)
	}

	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM audit_logs"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := db.conn.Query(
		"SELECT "+auditColumns+" FROM audit_logs"+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		l, err := scanAudit(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// ListAuditActions returns the distinct action values present in the log,
// for building filter dropdowns. It is intentionally not hub-scoped: the
// action vocabulary is a UI helper, not business data, so super_admin and
// hub-scoped admins see the same set of filter options.
func (db *DB) ListAuditActions() ([]string, error) {
	rows, err := db.conn.Query("SELECT DISTINCT action FROM audit_logs ORDER BY action")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// PruneAuditLogsBefore deletes entries older than the cutoff and returns the
// number removed.
func (db *DB) PruneAuditLogsBefore(cutoff time.Time) (int64, error) {
	result, err := db.conn.Exec("DELETE FROM audit_logs WHERE at < ?", cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
