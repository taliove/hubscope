package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/store"
)

// seedTaskAt inserts a finished task (with one log line) backdated to the
// given creation time, bypassing CreateTask which always stamps time.Now.
func seedTaskAt(t *testing.T, db *store.DB, taskType string, createdAt time.Time) int64 {
	t.Helper()
	task, err := db.CreateTask(store.Task{Type: taskType, Source: store.TaskSourceScheduled, EntityType: "", EntityID: 0})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := db.SetTaskCreatedAt(task.ID, createdAt.UTC()); err != nil {
		t.Fatalf("backdate task: %v", err)
	}
	if err := db.AppendTaskLog(task.ID, store.TaskLogInfo, "seeded", createdAt.UTC()); err != nil {
		t.Fatalf("append task log: %v", err)
	}
	return task.ID
}

// TestTaskRetentionCleanup drives the rollup worker's daily cleanup across
// the task-retention boundary and asserts tasks older than the retention
// (with their logs) are pruned while recent ones stay.
func TestTaskRetentionCleanup(t *testing.T) {
	db := openTempDB(t)

	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	clock := scheduler.NewFakeClock(start)
	ts := newTestAPIServer(t, db)

	// An old task past the 90-day retention and a recent one inside it.
	oldID := seedTaskAt(t, db, store.TaskTypeRollup, start.Add(-91*24*time.Hour))
	recentID := seedTaskAt(t, db, store.TaskTypeRollup, start.Add(-24*time.Hour))

	worker := scheduler.NewRollupWorker(db, clock,
		scheduler.WithRollupInterval(time.Hour),
		scheduler.WithCleanupInterval(24*time.Hour),
		scheduler.WithRetention(48*time.Hour),
		scheduler.WithRollupLag(time.Hour),
		scheduler.WithRollupPollInterval(time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("rollup worker did not stop within 10s of cancellation")
		}
	})

	// The first tick's cleanup prunes the old task. The task list confirms
	// it; the old task's detail (and its log) 404s.
	waitFor(t, "old task pruned by retention cleanup", func() bool {
		resp := doGet(t, fmt.Sprintf("%s/api/tasks?page=1&page_size=50", ts.URL))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return false
		}
		var payload struct {
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			return false
		}
		oldPresent, recentPresent := false, false
		for _, item := range payload.Items {
			if item.ID == oldID {
				oldPresent = true
			}
			if item.ID == recentID {
				recentPresent = true
			}
		}
		return !oldPresent && recentPresent
	})

	resp := doGet(t, fmt.Sprintf("%s/api/tasks/%d", ts.URL, oldID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("pruned task detail: expected 404, got %d", resp.StatusCode)
	}
}
