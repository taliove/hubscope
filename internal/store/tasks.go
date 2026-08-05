package store

import (
	"database/sql"
	"strings"
	"time"
)

// Task center vocabulary: the unified state machine every background job
// flows through, plus the known task types, trigger sources and log levels.
// Probe rounds are deliberately not tasks (too high-frequency).
const (
	TaskTypeEvalRun          = "eval_run"
	TaskTypeDiscoverySync    = "discovery_sync"
	TaskTypeRollup           = "rollup"
	TaskTypeRetentionCleanup = "retention_cleanup"

	TaskSourceManual    = "manual"
	TaskSourceScheduled = "scheduled"

	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusFailed  = "failed"

	TaskEntityEvalRun = "eval_run"
	TaskEntityHub     = "hub"

	TaskLogInfo  = "info"
	TaskLogWarn  = "warn"
	TaskLogError = "error"
)

// Task is one background job with a definite start and end: a row in the
// task center. EntityType/EntityID link it to the domain object it works on
// (e.g. the eval run it executes).
type Task struct {
	ID         int64
	Type       string
	Source     string
	Status     string
	EntityType string
	EntityID   int64
	StartedAt  *time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
}

// TaskLog is one timestamped line in a task's execution log.
type TaskLog struct {
	ID      int64
	TaskID  int64
	At      time.Time
	Level   string
	Message string
}

// taskColumns is the canonical tasks column list for scans.
const taskColumns = "id, type, source, status, entity_type, entity_id, started_at, finished_at, created_at"

// scanTask scans one tasks row.
func scanTask(s rowScanner) (Task, error) {
	var t Task
	var startedAt, finishedAt sql.NullString
	var createdAt string
	if err := s.Scan(&t.ID, &t.Type, &t.Source, &t.Status, &t.EntityType, &t.EntityID,
		&startedAt, &finishedAt, &createdAt); err != nil {
		return Task{}, err
	}
	if startedAt.Valid {
		v, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		t.StartedAt = &v
	}
	if finishedAt.Valid {
		v, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
		t.FinishedAt = &v
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return t, nil
}

// CreateTask registers a new task in pending status and returns the stored
// copy. The caller supplies type, source and entity linkage; status and
// timestamps are managed by the store.
func (db *DB) CreateTask(t Task) (*Task, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(`
		INSERT INTO tasks (type, source, status, entity_type, entity_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, t.Type, t.Source, TaskStatusPending, t.EntityType, t.EntityID, now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Task{
		ID:         id,
		Type:       t.Type,
		Source:     t.Source,
		Status:     TaskStatusPending,
		EntityType: t.EntityType,
		EntityID:   t.EntityID,
		CreatedAt:  now,
	}, nil
}

// GetTaskByEntity returns the newest task linked to the given domain object,
// or nil when none exists. Callers use it to annotate an entity's task after
// the fact (e.g. the score-drop alerter marking a skipped comparison on the
// run's finished task).
func (db *DB) GetTaskByEntity(entityType string, entityID int64) (*Task, error) {
	t, err := scanTask(db.conn.QueryRow(
		"SELECT "+taskColumns+" FROM tasks WHERE entity_type = ? AND entity_id = ? ORDER BY id DESC LIMIT 1",
		entityType, entityID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTask retrieves a task by ID.
func (db *DB) GetTask(id int64) (*Task, error) {
	t, err := scanTask(db.conn.QueryRow("SELECT "+taskColumns+" FROM tasks WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// StartTask flips a task to running and stamps its start time.
func (db *DB) StartTask(id int64, at time.Time) error {
	_, err := db.conn.Exec(
		"UPDATE tasks SET status = ?, started_at = ? WHERE id = ?",
		TaskStatusRunning, at.UTC().Format(time.RFC3339Nano), id)
	return err
}

// FinishTask marks a task with a terminal status (success or failed) and
// stamps its finish time.
func (db *DB) FinishTask(id int64, status string, at time.Time) error {
	_, err := db.conn.Exec(
		"UPDATE tasks SET status = ?, finished_at = ? WHERE id = ?",
		status, at.UTC().Format(time.RFC3339Nano), id)
	return err
}

// AppendTaskLog appends one line to a task's execution log.
func (db *DB) AppendTaskLog(taskID int64, level, message string, at time.Time) error {
	_, err := db.conn.Exec(
		"INSERT INTO task_logs (task_id, at, level, message) VALUES (?, ?, ?, ?)",
		taskID, at.UTC().Format(time.RFC3339Nano), level, message)
	return err
}

// ListTaskLogs returns a task's log lines in insertion order.
func (db *DB) ListTaskLogs(taskID int64) ([]TaskLog, error) {
	rows, err := db.conn.Query(
		"SELECT id, task_id, at, level, message FROM task_logs WHERE task_id = ? ORDER BY id", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []TaskLog
	for rows.Next() {
		var l TaskLog
		var at string
		if err := rows.Scan(&l.ID, &l.TaskID, &at, &l.Level, &l.Message); err != nil {
			return nil, err
		}
		l.At, _ = time.Parse(time.RFC3339Nano, at)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ListTasksAll returns one page of tasks, newest first, plus the total count.
// taskType and status filter on exact matches when non-empty. Page is
// 1-based; pageSize is clamped by the caller. It is the super_admin /
// store-internal counterpart of ListTasksByHub; HTTP handlers must pick the
// form based on the session's hub scope. Hub-less tasks (rollup / retention
// with entity_type "") belong to no hub and appear only here.
func (db *DB) ListTasksAll(page, pageSize int, taskType, status string) ([]Task, int, error) {
	return db.listTasks(0, page, pageSize, taskType, status)
}

// ListTasksByHub returns one page of tasks reachable from a single hub,
// newest first, plus the total count. The polymorphic entity dispatch is:
//   - entity_type 'hub'    -> entity_id is the hub id (discovery_sync tasks).
//   - entity_type 'eval_run' -> entity_id is an eval run id, resolved to a
//     hub through eval_runs -> campaign_models -> models.hub_id.
//
// Hub-less tasks (entity_type "" — rollup / retention) are excluded; they
// belong to the global *All view (super_admin only).
func (db *DB) ListTasksByHub(hubID int64, page, pageSize int, taskType, status string) ([]Task, int, error) {
	return db.listTasks(hubID, page, pageSize, taskType, status)
}

// listTasks is the shared implementation. hubID is 0 for the unscoped (all)
// variant — hub IDs are AUTOINCREMENT from 1, so 0 never matches — or the
// hubID parameter for the hub-scoped variant.
func (db *DB) listTasks(hubID int64, page, pageSize int, taskType, status string) ([]Task, int, error) {
	hubFilter := ""
	var hubArgs []interface{}
	if hubID != 0 {
		hubFilter = `WHERE (
			(entity_type = 'hub' AND entity_id = ?)
			OR (entity_type = 'eval_run' AND entity_id IN (
				SELECT r.id FROM eval_runs r
				WHERE EXISTS (
					SELECT 1 FROM campaign_models cm
					JOIN models m ON m.id = cm.model_id
					WHERE cm.campaign_id = r.campaign_id AND m.hub_id = ?
				)
			))
		)`
		hubArgs = append(hubArgs, hubID, hubID)
	}

	typeFilter := ""
	if taskType != "" {
		typeFilter = " AND type = ?"
		hubArgs = append(hubArgs, taskType)
	}
	if status != "" {
		typeFilter += " AND status = ?"
		hubArgs = append(hubArgs, status)
	}
	// The type/status filters are AND-anchored to the hub WHERE when scoped,
	// and need a leading WHERE when unscoped.
	if hubID == 0 && (taskType != "" || status != "") {
		hubFilter = "WHERE 1=1"
	}

	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM tasks "+hubFilter+typeFilter, hubArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	listArgs := append(append([]interface{}{}, hubArgs...), pageSize, offset)
	rows, err := db.conn.Query(
		"SELECT "+taskColumns+" FROM tasks "+hubFilter+typeFilter+" ORDER BY id DESC LIMIT ? OFFSET ?",
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, task)
	}
	return tasks, total, rows.Err()
}

// LastErrorLogByRun returns each eval run task's last error line, keyed by
// run ID — the failure reasons of a failed batch (2026-08-05 ops ruling).
func (db *DB) LastErrorLogByRun(runIDs []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(runIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(runIDs))
	args := make([]interface{}, len(runIDs))
	for i, id := range runIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.Query(`
		SELECT t.entity_id, l.message FROM task_logs l
		JOIN tasks t ON t.id = l.task_id
		WHERE t.entity_type = 'eval_run' AND t.entity_id IN (`+strings.Join(placeholders, ",")+`)
			AND l.level = 'error'
		ORDER BY t.entity_id, l.id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		var msg string
		if err := rows.Scan(&runID, &msg); err != nil {
			return nil, err
		}
		out[runID] = msg // last error wins (ordered by log id)
	}
	return out, rows.Err()
}

// SetTaskCreatedAt backdates a task's creation timestamp. It exists for
// retention tests that need tasks of a known age.
func (db *DB) SetTaskCreatedAt(id int64, at time.Time) error {
	_, err := db.conn.Exec("UPDATE tasks SET created_at = ? WHERE id = ?", at.UTC().Format(time.RFC3339Nano), id)
	return err
}

// PruneTasksBefore deletes tasks created before cutoff together with their
// logs, returning how many tasks were removed.
func (db *DB) PruneTasksBefore(cutoff time.Time) (int64, error) {
	if _, err := db.conn.Exec(
		"DELETE FROM task_logs WHERE task_id IN (SELECT id FROM tasks WHERE created_at < ?)",
		cutoff.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return 0, err
	}
	result, err := db.conn.Exec("DELETE FROM tasks WHERE created_at < ?", cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
