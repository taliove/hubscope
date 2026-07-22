package store

import (
	"database/sql"
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

// ListTasks returns one page of tasks, newest first, plus the total count.
// taskType and status filter on exact matches when non-empty. Page is
// 1-based; pageSize is clamped by the caller.
func (db *DB) ListTasks(page, pageSize int, taskType, status string) ([]Task, int, error) {
	where := ""
	args := []interface{}{}
	if taskType != "" {
		where += " AND type = ?"
		args = append(args, taskType)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if where != "" {
		where = " WHERE " + where[len(" AND "):]
	}

	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM tasks"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := db.conn.Query(
		"SELECT "+taskColumns+" FROM tasks"+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
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
