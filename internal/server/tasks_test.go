package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// listTasks fetches GET /api/tasks with the given raw query string and
// returns the decoded page object.
func listTasks(t *testing.T, base, query string) map[string]interface{} {
	t.Helper()
	url := base + "/api/tasks"
	if query != "" {
		url += "?" + query
	}
	resp := doGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/tasks?%s: expected 200, got %d: %s", query, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode task page: %v", err)
	}
	var page map[string]interface{}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatalf("unmarshal task page: %v", err)
	}
	return page
}

// taskItems extracts the items array of a task page as generic maps.
func taskItems(t *testing.T, page map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := page["items"].([]interface{})
	if !ok {
		t.Fatalf("task page items missing or wrong type: %v", page)
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		items = append(items, r.(map[string]interface{}))
	}
	return items
}

// taskTotal extracts the total count of a task page.
func taskTotal(t *testing.T, page map[string]interface{}) int {
	t.Helper()
	total, ok := page["total"].(float64)
	if !ok {
		t.Fatalf("task page total missing or wrong type: %v", page)
	}
	return int(total)
}

// getTaskDetail fetches GET /api/tasks/{id} and returns the decoded detail.
func getTaskDetail(t *testing.T, base string, id int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/tasks/%d", base, id))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/tasks/%d: expected 200, got %d: %s", id, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("unmarshal task detail: %v", err)
	}
	return detail
}

// taskLogs extracts the logs array of a task detail as generic maps.
func taskLogs(t *testing.T, detail map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := detail["logs"].([]interface{})
	if !ok {
		t.Fatalf("task detail logs missing or wrong type: %v", detail)
	}
	logs := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		logs = append(logs, r.(map[string]interface{}))
	}
	return logs
}

// countLogLines returns how many log lines carry the given level (empty =
// any) and contain substr in their message.
func countLogLines(logs []map[string]interface{}, level, substr string) int {
	n := 0
	for _, l := range logs {
		if level != "" && l["level"] != level {
			continue
		}
		if msg, _ := l["message"].(string); strings.Contains(msg, substr) {
			n++
		}
	}
	return n
}

// enabledCaseCount returns the number of enabled cases in a suite, read
// through the API so expectations track seed-bank growth instead of
// hardcoding a case count.
func enabledCaseCount(t *testing.T, base string, suiteID int64) int {
	t.Helper()
	resp := doGet(t, base+"/api/suites")
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	for _, s := range suites {
		if int64(s["id"].(float64)) != suiteID {
			continue
		}
		n := 0
		for _, c := range s["cases"].([]interface{}) {
			if enabled, _ := c.(map[string]interface{})["enabled"].(bool); enabled {
				n++
			}
		}
		return n
	}
	t.Fatalf("suite %d not found", suiteID)
	return 0
}

// waitTaskStatus polls the task list until the task linked to the given eval
// run reaches one of the wanted statuses, then returns it.
func waitTaskStatus(t *testing.T, base string, runID int64, want ...string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, item := range taskItems(t, listTasks(t, base, "type=eval_run")) {
			if int64(item["entity_id"].(float64)) != runID {
				continue
			}
			for _, w := range want {
				if item["status"] == w {
					return item
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no task for eval run %d reached %v in time", runID, want)
	return nil
}

// TestEvalRunRegistersTask verifies that a manual eval run registers a task
// which flows to success, carrying per-case progress logs.
func TestEvalRunRegistersTask(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	dumbID := createEvalModel(t, ts.URL, stub.URL, "dumb-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	runID := triggerEval(t, ts.URL, suiteID, smartID, dumbID)
	waitEvalDone(t, ts.URL, runID)
	task := waitTaskStatus(t, ts.URL, runID, "success")

	if task["type"] != "eval_run" {
		t.Errorf("task type = %v, want eval_run", task["type"])
	}
	if task["source"] != "manual" {
		t.Errorf("task source = %v, want manual", task["source"])
	}
	if task["entity_type"] != "eval_run" {
		t.Errorf("task entity_type = %v, want eval_run", task["entity_type"])
	}
	if int64(task["entity_id"].(float64)) != runID {
		t.Errorf("task entity_id = %v, want %d", task["entity_id"], runID)
	}
	if task["started_at"] == nil || task["finished_at"] == nil {
		t.Errorf("successful task must carry started_at and finished_at: %v", task)
	}
	if d, ok := task["duration_ms"].(float64); !ok || d < 0 {
		t.Errorf("finished task must carry a non-negative duration_ms, got %v", task["duration_ms"])
	}
	if task["created_at"] == nil || task["created_at"] == "" {
		t.Errorf("task must carry created_at: %v", task)
	}

	// Detail carries the line-by-line execution log: one completion line per
	// (model, case) pair — 2 models x N basic cases — bracketed by start and
	// terminal lines. N is read back through the API so the expectation
	// tracks seed-bank growth.
	caseCount := enabledCaseCount(t, ts.URL, suiteID)
	detail := getTaskDetail(t, ts.URL, int64(task["id"].(float64)))
	logs := taskLogs(t, detail)
	if got := countLogLines(logs, "info", "eval run started"); got != 1 {
		t.Errorf("expected 1 start line, got %d (logs: %v)", got, logs)
	}
	if got := countLogLines(logs, "info", "status=success"); got != 1 {
		t.Errorf("expected 1 terminal success line, got %d (logs: %v)", got, logs)
	}
	if got := countLogLines(logs, "info", "done: model="); got != 2*caseCount {
		t.Errorf("expected %d case completion lines, got %d (logs: %v)", 2*caseCount, got, logs)
	}
	if got := countLogLines(logs, "info", "model=smart-model score=1.00"); got != caseCount {
		t.Errorf("expected %d smart-model score=1.00 lines, got %d (logs: %v)", caseCount, got, logs)
	}
	if got := countLogLines(logs, "info", "model=dumb-model score=0.00"); got != caseCount {
		t.Errorf("expected %d dumb-model score=0.00 lines, got %d (logs: %v)", caseCount, got, logs)
	}
	for _, l := range logs {
		if l["at"] == nil || l["at"] == "" {
			t.Errorf("every log line must carry a timestamp: %v", l)
		}
	}
}

// TestEvalRunTaskLogsJudgeFailure verifies judge failures land in the task
// log as warn lines while judged cases log their score.
func TestEvalRunTaskLogsJudgeFailure(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "chinese")

	runID := triggerEval(t, ts.URL, suiteID, smartID)
	waitEvalDone(t, ts.URL, runID)
	task := waitTaskStatus(t, ts.URL, runID, "success")

	detail := getTaskDetail(t, ts.URL, int64(task["id"].(float64)))
	logs := taskLogs(t, detail)

	// The chinese suite's judge cases are all scored 0.75 by the stub except
	// the formal-rewrite case, which gets a garbage judge reply and must
	// surface as a warn-level judge failure line, never as score 0.
	caseCount := enabledCaseCount(t, ts.URL, suiteID)
	if got := countLogLines(logs, "info", "score=0.75"); got != caseCount-1 {
		t.Errorf("expected %d judged score=0.75 lines, got %d (logs: %v)", caseCount-1, got, logs)
	}
	if got := countLogLines(logs, "warn", "judge failed"); got != 1 {
		t.Errorf("expected 1 judge failure warn line, got %d (logs: %v)", got, logs)
	}
}

// TestFailedEvalRunTaskMarkedFailed drives the weekly worker with a hub that
// hangs mid-run, cancels the worker context, and asserts the interrupted run
// ends with its task marked failed (and an error-level log line).
func TestFailedEvalRunTaskMarkedFailed(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "tasks-failed.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedTestUser(t, db)

	stub := newEvalStubHub()
	t.Cleanup(stub.Close)
	srv := server.New(db, server.WithRateLimits(server.RateLimits{}))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Two chat models: the run blocks inside the first model's first case,
	// so cancelling before release makes the second model's iteration observe
	// the canceled context and fail the run deterministically.
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "dumb-model")
	// Model creation trial-probes pollute the stub's call log; reset it so
	// the wait below only observes genuine eval calls.
	stub.resetCalls()
	stub.blockCalls()

	clock := scheduler.NewFakeClock(time.Date(2026, 7, 19, 1, 30, 0, 0, time.UTC)) // a Sunday
	worker := scheduler.NewEvalWorker(db, srv.Evaluator(), clock,
		scheduler.WithEvalPollInterval(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		stub.release()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("eval worker did not stop within 10s of cancellation")
		}
	})

	waitFor(t, "first eval call reaching the stub", func() bool {
		return stub.sawModel("smart-model") || stub.sawModel("dumb-model")
	})
	cancel()
	stub.release()

	var failedTask map[string]interface{}
	waitFor(t, "failed task to appear", func() bool {
		// The hub auto-sync triggered by the model setup may have failed
		// against the eval stub and registered its own failed task; this test
		// cares only about the eval run's task.
		items := taskItems(t, listTasks(t, ts.URL, "status=failed&type=eval_run"))
		if len(items) != 1 {
			return false
		}
		failedTask = items[0]
		return true
	})

	if failedTask["type"] != "eval_run" {
		t.Errorf("failed task type = %v, want eval_run", failedTask["type"])
	}
	if failedTask["source"] != "scheduled" {
		t.Errorf("failed task source = %v, want scheduled", failedTask["source"])
	}
	if failedTask["finished_at"] == nil {
		t.Errorf("failed task must carry finished_at: %v", failedTask)
	}

	// The linked eval run is failed as well.
	runID := int64(failedTask["entity_id"].(float64))
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", ts.URL, runID))
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	if run["status"] != "failed" {
		t.Errorf("linked eval run status = %v, want failed", run["status"])
	}

	detail := getTaskDetail(t, ts.URL, int64(failedTask["id"].(float64)))
	logs := taskLogs(t, detail)
	if got := countLogLines(logs, "error", "cancel"); got != 1 {
		t.Errorf("expected 1 error-level cancellation line, got %d (logs: %v)", got, logs)
	}
}

// TestTaskListPaginationAndFilters exercises the type/status filters and the
// page/page_size envelope of GET /api/tasks.
func TestTaskListPaginationAndFilters(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	smartID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "basic")

	run1 := triggerEval(t, ts.URL, suiteID, smartID)
	waitTaskStatus(t, ts.URL, run1, "success")
	run2 := triggerEval(t, ts.URL, suiteID, smartID)
	waitTaskStatus(t, ts.URL, run2, "success")

	page1 := listTasks(t, ts.URL, "type=eval_run&page=1&page_size=1")
	if got := taskTotal(t, page1); got != 2 {
		t.Fatalf("total = %d, want 2", got)
	}
	items1 := taskItems(t, page1)
	if len(items1) != 1 {
		t.Fatalf("page 1 with page_size=1 returned %d items, want 1", len(items1))
	}
	// Newest first: page 1 carries the second run's task.
	if int64(items1[0]["entity_id"].(float64)) != run2 {
		t.Errorf("page 1 entity_id = %v, want run %d (newest first)", items1[0]["entity_id"], run2)
	}

	page2 := listTasks(t, ts.URL, "type=eval_run&page=2&page_size=1")
	items2 := taskItems(t, page2)
	if len(items2) != 1 || int64(items2[0]["entity_id"].(float64)) != run1 {
		t.Errorf("page 2 = %v, want run %d", items2, run1)
	}

	if got := taskTotal(t, listTasks(t, ts.URL, "status=success")); got != 2 {
		t.Errorf("status=success total = %d, want 2", got)
	}
	if got := taskTotal(t, listTasks(t, ts.URL, "status=pending")); got != 0 {
		t.Errorf("status=pending total = %d, want 0", got)
	}
	if got := taskTotal(t, listTasks(t, ts.URL, "type=no_such_type")); got != 0 {
		t.Errorf("unknown type total = %d, want 0", got)
	}
}

// TestProbeRoundsAreNotTasks pins the boundary that probe rounds never
// register tasks: probing an endpoint adds nothing to the task list (the
// hub/model setup may legitimately have registered discovery sync tasks).
func TestProbeRoundsAreNotTasks(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")

	resp := doGet(t, ts.URL+"/api/models")
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var models []map[string]interface{}
	_ = json.Unmarshal(env.Data, &models)
	endpoints := models[0]["endpoints"].([]interface{})
	endpointID := int64(endpoints[0].(map[string]interface{})["id"].(float64))

	before := taskTotal(t, listTasks(t, ts.URL, ""))

	probeResp := doPost(t, fmt.Sprintf("%s/api/endpoints/%d/probe", ts.URL, endpointID), nil)
	probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusOK {
		t.Fatalf("probe: expected 200, got %d", probeResp.StatusCode)
	}

	if got := taskTotal(t, listTasks(t, ts.URL, "")); got != before {
		t.Errorf("probe round changed task total from %d to %d (probe rounds are not tasks)", before, got)
	}
}

// TestTaskDetailErrors covers the not-found and malformed-id paths of
// GET /api/tasks/{id}.
func TestTaskDetailErrors(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	resp := doGet(t, ts.URL+"/api/tasks/999999")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown task: expected 404, got %d", resp.StatusCode)
	}

	resp = doGet(t, ts.URL+"/api/tasks/abc")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed task id: expected 400, got %d", resp.StatusCode)
	}
}
