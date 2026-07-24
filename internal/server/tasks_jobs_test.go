package server_test

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
)

// waitTasksByType polls GET /api/tasks?type=... until at least want tasks in
// the wanted status exist (empty status = any), then returns them newest
// first.
func waitTasksByType(t *testing.T, base, taskType, status string, want int) []map[string]interface{} {
	t.Helper()
	var matched []map[string]interface{}
	waitFor(t, fmt.Sprintf("%d %s task(s) with status %q", want, taskType, status), func() bool {
		matched = matched[:0]
		for _, item := range taskItems(t, listTasks(t, base, "type="+taskType)) {
			if status == "" || item["status"] == status {
				matched = append(matched, item)
			}
		}
		return len(matched) >= want
	})
	return matched
}

// TestDiscoverySyncTasksCoverAutoAndManualTriggers verifies that both the
// automatic sync on hub creation and a manual per-hub re-sync register
// discovery_sync tasks whose logs carry the sync outcome stats, and that the
// task type filter isolates them from other task types.
func TestDiscoverySyncTasksCoverAutoAndManualTriggers(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"model-a", "model-b"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	// The automatic sync on hub add registered a task linked to the hub.
	tasks := waitTasksByType(t, ts.URL, "discovery_sync", "success", 1)
	auto := tasks[0]
	if auto["source"] != "manual" {
		t.Errorf("auto-sync task source = %v, want manual", auto["source"])
	}
	if auto["entity_type"] != "hub" {
		t.Errorf("auto-sync task entity_type = %v, want hub", auto["entity_type"])
	}
	if int(auto["entity_id"].(float64)) != hubID {
		t.Errorf("auto-sync task entity_id = %v, want %d", auto["entity_id"], hubID)
	}
	if auto["started_at"] == nil || auto["finished_at"] == nil {
		t.Errorf("successful sync task must carry started_at and finished_at: %v", auto)
	}

	logs := taskLogs(t, getTaskDetail(t, ts.URL, int64(auto["id"].(float64))))
	if got := countLogLines(logs, "info", "discovery sync started"); got != 1 {
		t.Errorf("expected 1 sync start line, got %d (logs: %v)", got, logs)
	}
	if got := countLogLines(logs, "info", "added=2 updated=0 retired=0"); got != 1 {
		t.Errorf("expected 1 stats line with added=2 updated=0 retired=0, got %d (logs: %v)", got, logs)
	}

	// A manual re-sync registers its own task with fresh stats: model-a is
	// refreshed (updated), model-c is new (added), model-b vanished (retired).
	stub.setModels([]string{"model-a", "model-c"})
	resp := syncHubViaAPI(t, ts.URL, hubID)
	resp.Body.Close()
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")

	tasks = waitTasksByType(t, ts.URL, "discovery_sync", "success", 2)
	manual := tasks[0] // newest first
	if manual["source"] != "manual" {
		t.Errorf("manual re-sync task source = %v, want manual", manual["source"])
	}
	if int(manual["entity_id"].(float64)) != hubID {
		t.Errorf("manual re-sync task entity_id = %v, want %d", manual["entity_id"], hubID)
	}
	logs = taskLogs(t, getTaskDetail(t, ts.URL, int64(manual["id"].(float64))))
	if got := countLogLines(logs, "info", "added=1 updated=1 retired=1"); got != 1 {
		t.Errorf("expected 1 stats line with added=1 updated=1 retired=1, got %d (logs: %v)", got, logs)
	}

	// Type filtering: discovery tasks never leak into the eval_run filter,
	// and the discovery filter returns only discovery tasks.
	if got := taskTotal(t, listTasks(t, ts.URL, "type=eval_run")); got != 0 {
		t.Errorf("type=eval_run total = %d, want 0 (no eval runs triggered)", got)
	}
	for _, item := range taskItems(t, listTasks(t, ts.URL, "type=discovery_sync")) {
		if item["type"] != "discovery_sync" {
			t.Errorf("type=discovery_sync page carried type %v", item["type"])
		}
	}
}

// TestDiscoveryRunEndpointRegistersTasks verifies the synchronous full-sync
// endpoint (POST /api/discovery/run) registers a task for the hub it syncs.
func TestDiscoveryRunEndpointRegistersTasks(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, []string{"model-a"})
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "succeeded")
	waitTasksByType(t, ts.URL, "discovery_sync", "success", 1)

	runDiscovery(t, ts.URL)

	tasks := waitTasksByType(t, ts.URL, "discovery_sync", "success", 2)
	latest := tasks[0] // newest first
	if latest["source"] != "manual" {
		t.Errorf("full-sync task source = %v, want manual", latest["source"])
	}
	if int(latest["entity_id"].(float64)) != hubID {
		t.Errorf("full-sync task entity_id = %v, want %d", latest["entity_id"], hubID)
	}
	logs := taskLogs(t, getTaskDetail(t, ts.URL, int64(latest["id"].(float64))))
	if got := countLogLines(logs, "info", "added=0 updated=1 retired=0"); got != 1 {
		t.Errorf("expected 1 stats line with added=0 updated=1 retired=0, got %d (logs: %v)", got, logs)
	}
}

// TestDiscoverySyncFailureMarksTaskFailed verifies a sync whose model
// listing fails ends with a failed task carrying an error-level log line.
func TestDiscoverySyncFailureMarksTaskFailed(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	stub := newDiscoveryStubHub(t, nil)
	stub.setListFailing(true)
	hubID := createHubViaAPI(t, ts.URL, stub.URL)
	waitForHubSyncStatus(t, ts.URL, hubID, "failed")

	tasks := waitTasksByType(t, ts.URL, "discovery_sync", "failed", 1)
	failed := tasks[0]
	if failed["finished_at"] == nil {
		t.Errorf("failed sync task must carry finished_at: %v", failed)
	}
	logs := taskLogs(t, getTaskDetail(t, ts.URL, int64(failed["id"].(float64))))
	if got := countLogLines(logs, "error", "discovery sync failed"); got != 1 {
		t.Errorf("expected 1 error-level failure line, got %d (logs: %v)", got, logs)
	}
}

// TestRollupAndCleanupRegisterTasks drives the rollup worker with a fake
// clock and asserts both the rollup and the retention cleanup register
// scheduled tasks whose logs record the rows processed.
func TestRollupAndCleanupRegisterTasks(t *testing.T) {
	db := openTempDB(t)

	start := time.Now().UTC()
	clock := scheduler.NewFakeClock(start)
	ts := httptest.NewServer(server.New(db,
		server.WithNow(clock.Now), server.WithRateLimits(server.RateLimits{})))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()
	ids := createModelEndpoints(t, ts.URL, stub.URL, "model-rollup-tasks")
	ep := int64(ids[0])

	// Three probes old enough to be both rolled up (lag 1h) and deleted
	// (retention 48h) on the worker's first tick.
	oldBase := start.Add(-3 * 24 * time.Hour)
	seedProbeFull(t, db, ep, false, true, 100, nil, nil, oldBase)
	seedProbeFull(t, db, ep, false, true, 200, nil, nil, oldBase.Add(10*time.Minute))
	seedProbeFull(t, db, ep, true, true, 300, nil, nil, oldBase.Add(20*time.Minute))

	startRollupWorker(t, db, clock)

	rollupTasks := waitTasksByType(t, ts.URL, "rollup", "success", 1)
	rollupTask := rollupTasks[0]
	if rollupTask["source"] != "scheduled" {
		t.Errorf("rollup task source = %v, want scheduled", rollupTask["source"])
	}
	if rollupTask["started_at"] == nil || rollupTask["finished_at"] == nil {
		t.Errorf("rollup task must carry started_at and finished_at: %v", rollupTask)
	}
	logs := taskLogs(t, getTaskDetail(t, ts.URL, int64(rollupTask["id"].(float64))))
	if got := countLogLines(logs, "info", "probes_aggregated=3"); got != 1 {
		t.Errorf("expected 1 log line with probes_aggregated=3, got %d (logs: %v)", got, logs)
	}

	cleanupTasks := waitTasksByType(t, ts.URL, "retention_cleanup", "success", 1)
	cleanupTask := cleanupTasks[0]
	if cleanupTask["source"] != "scheduled" {
		t.Errorf("cleanup task source = %v, want scheduled", cleanupTask["source"])
	}
	logs = taskLogs(t, getTaskDetail(t, ts.URL, int64(cleanupTask["id"].(float64))))
	if got := countLogLines(logs, "info", "deleted raw probes: probes_deleted=3"); got != 1 {
		t.Errorf("expected 1 log line with probes_deleted=3, got %d (logs: %v)", got, logs)
	}
	if got := countLogLines(logs, "info", "pruned audit logs: audit_logs_pruned="); got != 1 {
		t.Errorf("expected 1 log line reporting pruned audit logs, got %d (logs: %v)", got, logs)
	}
	if got := countLogLines(logs, "info", "retention cleanup finished"); got != 1 {
		t.Errorf("expected 1 terminal cleanup line, got %d (logs: %v)", got, logs)
	}
}
