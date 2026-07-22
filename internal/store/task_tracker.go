package store

import (
	"log/slog"
	"time"
)

// TaskTracker mirrors one background job's execution into the task center:
// the job is registered as a task when it starts, progress lines land in
// task_logs, and the terminal outcome flips the task to success/failed.
//
// A nil *TaskTracker is a valid no-op logger: a task-registration failure is
// logged and the job proceeds untracked rather than aborting. Every method
// is therefore safe to call without a nil check.
type TaskTracker struct {
	db *DB
	id int64
}

// BeginTask registers a task (pending, then immediately running) and writes
// the start line. It returns nil when registration fails, in which case all
// subsequent tracking is a no-op. Jobs without a linked domain object pass
// an empty entityType and entityID 0.
func (db *DB) BeginTask(taskType, source, entityType string, entityID int64, startMessage string) *TaskTracker {
	task, err := db.CreateTask(Task{
		Type:       taskType,
		Source:     source,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		slog.Error("task center: register task", "type", taskType, "error", err)
		return nil
	}
	if err := db.StartTask(task.ID, time.Now().UTC()); err != nil {
		slog.Error("task center: mark task running", "task_id", task.ID, "error", err)
	}
	t := &TaskTracker{db: db, id: task.ID}
	t.Log(TaskLogInfo, startMessage)
	return t
}

// Log appends one line to the task's execution log. Persistence failures are
// logged but never abort the job.
func (t *TaskTracker) Log(level, message string) {
	if t == nil {
		return
	}
	if err := t.db.AppendTaskLog(t.id, level, message, time.Now().UTC()); err != nil {
		slog.Error("task center: append task log", "task_id", t.id, "error", err)
	}
}

// Succeed writes the terminal line and flips the task to success.
func (t *TaskTracker) Succeed(message string) {
	if t == nil {
		return
	}
	t.Log(TaskLogInfo, message)
	if err := t.db.FinishTask(t.id, TaskStatusSuccess, time.Now().UTC()); err != nil {
		slog.Error("task center: mark task success", "task_id", t.id, "error", err)
	}
}

// Fail writes the failure reason as an error line and flips the task to
// failed.
func (t *TaskTracker) Fail(reason string) {
	if t == nil {
		return
	}
	t.Log(TaskLogError, reason)
	if err := t.db.FinishTask(t.id, TaskStatusFailed, time.Now().UTC()); err != nil {
		slog.Error("task center: mark task failed", "task_id", t.id, "error", err)
	}
}
