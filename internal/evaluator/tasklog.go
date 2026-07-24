package evaluator

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// runTask mirrors one eval run's execution into the task center: the run is
// registered as a task when execution starts, per-case progress lands in
// task_logs, and the terminal outcome flips the task to success/failed.
//
// A nil *runTask is a valid no-op logger: a task-registration failure is
// logged and the run proceeds untracked rather than aborting.
type runTask struct {
	db *store.DB
	id int64
}

// beginRunTask registers the run as a task (pending, then immediately
// running) and writes the start line. It returns nil when registration
// fails, in which case all subsequent logging is a no-op.
func (e *Evaluator) beginRunTask(run *store.EvalRun, modelCount int) *runTask {
	task, err := e.db.CreateTask(store.Task{
		Type:       store.TaskTypeEvalRun,
		Source:     run.Trigger,
		EntityType: store.TaskEntityEvalRun,
		EntityID:   run.ID,
	})
	if err != nil {
		slog.Error("evaluator: register task for eval run", "run_id", run.ID, "error", err)
		return nil
	}
	if err := e.db.StartTask(task.ID, time.Now().UTC()); err != nil {
		slog.Error("evaluator: mark task running", "task_id", task.ID, "error", err)
	}
	t := &runTask{db: e.db, id: task.ID}
	t.log(store.TaskLogInfo, fmt.Sprintf(
		"eval run started: suite_id=%d models=%d judge=%q", run.SuiteID, modelCount, run.JudgeModel))
	return t
}

// log appends one line to the task's execution log. Persistence failures are
// logged but never abort the run.
func (t *runTask) log(level, message string) {
	if t == nil {
		return
	}
	if err := t.db.AppendTaskLog(t.id, level, message, time.Now().UTC()); err != nil {
		slog.Error("evaluator: append task log", "task_id", t.id, "error", err)
	}
}

// succeed writes the terminal line and flips the task to success.
func (t *runTask) succeed() {
	if t == nil {
		return
	}
	t.log(store.TaskLogInfo, "eval run finished: status=success")
	if err := t.db.FinishTask(t.id, store.TaskStatusSuccess, time.Now().UTC()); err != nil {
		slog.Error("evaluator: mark task success", "task_id", t.id, "error", err)
	}
}

// fail writes the failure reason as an error line and flips the task to
// failed.
func (t *runTask) fail(reason string) {
	if t == nil {
		return
	}
	t.log(store.TaskLogError, "eval run failed: "+reason)
	if err := t.db.FinishTask(t.id, store.TaskStatusFailed, time.Now().UTC()); err != nil {
		slog.Error("evaluator: mark task failed", "task_id", t.id, "error", err)
	}
}
