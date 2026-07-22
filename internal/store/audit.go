package store

import (
	"time"
)

// AuditLog is one administrative action record: who (actor/IP) did what
// (action on object_type/object_id), with free-form detail and the outcome.
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
}

// auditColumns is the canonical column list for scanning an AuditLog.
const auditColumns = "id, at, actor, ip, action, object_type, object_id, detail, result"

// scanAudit scans a row containing auditColumns into an AuditLog.
func scanAudit(s rowScanner) (AuditLog, error) {
	var l AuditLog
	var at string
	if err := s.Scan(&l.ID, &at, &l.Actor, &l.IP, &l.Action, &l.ObjectType, &l.ObjectID, &l.Detail, &l.Result); err != nil {
		return AuditLog{}, err
	}
	l.At, _ = time.Parse(time.RFC3339Nano, at)
	return l, nil
}

// InsertAudit appends one audit entry, stamping it with the current time.
func (db *DB) InsertAudit(l AuditLog) error {
	_, err := db.conn.Exec(
		"INSERT INTO audit_logs (at, actor, ip, action, object_type, object_id, detail, result) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339Nano), l.Actor, l.IP, l.Action, l.ObjectType, l.ObjectID, l.Detail, l.Result,
	)
	return err
}

// ListAuditLogs returns one page of entries, newest first, plus the total
// count. action filters on an exact action when non-empty. Page is 1-based;
// pageSize is clamped by the caller.
func (db *DB) ListAuditLogs(page, pageSize int, action string) ([]AuditLog, int, error) {
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
// for building filter dropdowns.
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
