package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/scheduler"
	"github.com/taliove/hubscope/internal/store"
)

// fetchAuditLogs queries GET /api/audit-logs with the given raw query suffix
// and returns the decoded page object.
func fetchAuditLogs(t *testing.T, baseURL, query string) map[string]interface{} {
	t.Helper()
	resp := doGet(t, baseURL+"/api/audit-logs"+query)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list audit logs: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	var page map[string]interface{}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatalf("unmarshal audit logs: %v", err)
	}
	return page
}

// auditItems extracts the items array from a page object.
func auditItems(t *testing.T, page map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := page["items"].([]interface{})
	if !ok {
		t.Fatalf("page has no items array: %v", page)
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		items = append(items, r.(map[string]interface{}))
	}
	return items
}

// actionsOf collects the action values of the items.
func actionsOf(items []map[string]interface{}) map[string]bool {
	set := map[string]bool{}
	for _, it := range items {
		set[it["action"].(string)] = true
	}
	return set
}

// TestAuditLogsRecordWriteOperations verifies that every administrative write
// operation lands in the audit log with actor, IP, detail and result.
func TestAuditLogsRecordWriteOperations(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	hubID := createHubViaAPI(t, ts.URL, stubHub.URL)

	resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
		"hub_id":   hubID,
		"model_id": "audit-model",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create model: expected 201, got %d", resp.StatusCode)
	}

	models := listModelsViaAPI(t, ts.URL)
	model := models["audit-model"]
	modelID := int64(model["id"].(float64))
	epID := int64(model["endpoints"].([]interface{})[0].(map[string]interface{})["id"].(float64))

	// hub.update
	putHubResp := doPut(t, fmt.Sprintf("%s/api/hubs/%d", ts.URL, hubID), map[string]interface{}{"name": "audit-hub-renamed"})
	putHubResp.Body.Close()

	patchResp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epID), map[string]interface{}{"enabled": false})
	patchResp.Body.Close()
	runProbeRound(t, ts, epID)

	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{"alert_enabled": true})
	putResp.Body.Close()

	ruleResp := doPost(t, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "family", "keyword": "auditkw", "category": "acme",
	})
	var ruleEnv envelope
	if err := json.NewDecoder(ruleResp.Body).Decode(&ruleEnv); err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	ruleResp.Body.Close()
	var createdRule map[string]interface{}
	if err := json.Unmarshal(ruleEnv.Data, &createdRule); err != nil {
		t.Fatalf("unmarshal rule: %v", err)
	}
	ruleID := int(createdRule["id"].(float64))

	// rule.update + rule.delete
	patchRuleResp := doPatch(t, fmt.Sprintf("%s/api/classification-rules/%d", ts.URL, ruleID), map[string]interface{}{"category": "acme2"})
	patchRuleResp.Body.Close()
	delRuleResp := doDelete(t, fmt.Sprintf("%s/api/classification-rules/%d", ts.URL, ruleID))
	delRuleResp.Body.Close()

	// case.create + case.update against the first seeded suite
	suitesResp := doGet(t, ts.URL+"/api/suites")
	var suitesEnv envelope
	if err := json.NewDecoder(suitesResp.Body).Decode(&suitesEnv); err != nil {
		t.Fatalf("decode suites: %v", err)
	}
	suitesResp.Body.Close()
	var suites []map[string]interface{}
	if err := json.Unmarshal(suitesEnv.Data, &suites); err != nil {
		t.Fatalf("unmarshal suites: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("expected seeded suites")
	}
	suiteID := int64(suites[0]["id"].(float64))
	caseResp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "2+2=?",
		"verdict_type": "rule",
		"rule_config":  map[string]interface{}{"mode": "exact", "expected": "4"},
	})
	var caseEnv envelope
	if err := json.NewDecoder(caseResp.Body).Decode(&caseEnv); err != nil {
		t.Fatalf("decode case: %v", err)
	}
	caseResp.Body.Close()
	if caseResp.StatusCode != http.StatusCreated {
		t.Fatalf("create case: expected 201, got %d", caseResp.StatusCode)
	}
	var createdCase map[string]interface{}
	if err := json.Unmarshal(caseEnv.Data, &createdCase); err != nil {
		t.Fatalf("unmarshal case: %v", err)
	}
	patchCaseResp := doPatch(t, fmt.Sprintf("%s/api/cases/%d", ts.URL, int64(createdCase["id"].(float64))),
		map[string]interface{}{"prompt": "3+3=?"})
	patchCaseResp.Body.Close()

	// eval.create (async run against the stub hub; only creation is audited)
	evalResp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id":  suiteID,
		"model_ids": []int64{modelID},
	})
	evalResp.Body.Close()
	if evalResp.StatusCode != http.StatusAccepted {
		t.Fatalf("create eval: expected 202, got %d", evalResp.StatusCode)
	}

	// discovery.run (full sync over all hubs)
	discResp := doPost(t, ts.URL+"/api/discovery/run", nil)
	discResp.Body.Close()

	syncResp := syncHubViaAPI(t, ts.URL, hubID)
	syncResp.Body.Close()

	delEpResp := doDelete(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epID))
	delEpResp.Body.Close()
	delModelResp := doDelete(t, fmt.Sprintf("%s/api/models/%d", ts.URL, modelID))
	delModelResp.Body.Close()
	delHubResp := doDelete(t, fmt.Sprintf("%s/api/hubs/%d", ts.URL, hubID))
	delHubResp.Body.Close()

	page := fetchAuditLogs(t, ts.URL, "?page_size=100")
	total := int(page["total"].(float64))
	if total < 16 {
		t.Fatalf("expected at least 16 audit entries, got %d", total)
	}
	items := auditItems(t, page)

	wantActions := []string{
		"hub.create", "hub.update", "model.create", "endpoint.patch", "endpoint.probe",
		"settings.update", "rule.create", "rule.update", "rule.delete",
		"case.create", "case.update", "eval.create", "discovery.run", "hub.sync",
		"endpoint.delete", "model.delete", "hub.delete",
	}
	got := actionsOf(items)
	for _, want := range wantActions {
		if !got[want] {
			t.Errorf("missing audit action %q; got %v", want, got)
		}
	}

	// Entries carry actor, IP and a result; newest first. The actor "admin"
	// here is the logged-in username of the seeded super_admin (seedTestUser
	// creates a user named "admin"), not a hardcoded constant — see
	// TestAuditActorIsLoginUsername for the non-"admin" proof.
	for _, it := range items {
		if it["actor"] != "admin" {
			t.Errorf("actor: expected admin, got %v", it["actor"])
		}
		if it["ip"] == "" || it["ip"] == nil {
			t.Errorf("ip should be recorded, got %v", it["ip"])
		}
		if it["result"] == "" || it["result"] == nil {
			t.Errorf("result should be recorded, got %v", it["result"])
		}
	}
	first := items[0]["at"].(string)
	last := items[len(items)-1]["at"].(string)
	if first < last {
		t.Errorf("expected newest first, got first=%s last=%s", first, last)
	}

	// The create entry details what was added.
	for _, it := range items {
		if it["action"] == "hub.create" {
			detail, _ := it["detail"].(string)
			if detail == "" {
				t.Error("hub.create should record detail (name/base_url)")
			}
			if it["result"] != "success" {
				t.Errorf("hub.create result: expected success, got %v", it["result"])
			}
		}
	}

	// Action filter narrows the listing.
	filtered := fetchAuditLogs(t, ts.URL, "?action=hub.create")
	for _, it := range auditItems(t, filtered) {
		if it["action"] != "hub.create" {
			t.Errorf("action filter leaked %v", it["action"])
		}
	}
	if got := len(auditItems(t, filtered)); got != 1 {
		t.Errorf("expected exactly 1 hub.create entry, got %d", got)
	}

	// Pagination: page sizes and distinct pages.
	p1 := fetchAuditLogs(t, ts.URL, "?page=1&page_size=3")
	p2 := fetchAuditLogs(t, ts.URL, "?page=2&page_size=3")
	if got := len(auditItems(t, p1)); got != 3 {
		t.Fatalf("page 1: expected 3 items, got %d", got)
	}
	if int(p1["total"].(float64)) != total {
		t.Errorf("total should be stable across pages: %v vs %d", p1["total"], total)
	}
	id1 := auditItems(t, p1)[0]["id"]
	for _, it := range auditItems(t, p2) {
		if it["id"] == id1 {
			t.Error("page 2 should not overlap page 1")
		}
	}
}

// TestAuditLogsFailedLogin verifies failed and successful logins are audited.
func TestAuditLogsFailedLogin(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json",
		bytes.NewBufferString(`{"username":"x","password":"definitely-wrong-fake"}`))
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login: expected 401, got %d", resp.StatusCode)
	}

	page := fetchAuditLogs(t, ts.URL, "?action=auth.login")
	items := auditItems(t, page)
	// Two entries: the failed attempt above plus the successful login the
	// authed test client performed when fetching the audit list.
	var failed, succeeded bool
	for _, it := range items {
		switch it["result"] {
		case "failed":
			failed = true
		case "success":
			succeeded = true
		}
	}
	if !failed {
		t.Error("expected a failed auth.login audit entry")
	}
	if !succeeded {
		t.Error("expected a success auth.login audit entry")
	}
}

// TestAuditPrune verifies old audit entries are pruned by the retention job.
func TestAuditPrune(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	createHubViaAPI(t, ts.URL, stubHub.URL)
	if total := int(fetchAuditLogs(t, ts.URL, "")["total"].(float64)); total == 0 {
		t.Fatal("expected audit entries before prune")
	}

	pruned, err := db.PruneAuditLogsBefore(time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("prune audit logs: %v", err)
	}
	if pruned == 0 {
		t.Error("expected prune to delete rows")
	}
	if total := int(fetchAuditLogs(t, ts.URL, "")["total"].(float64)); total != 0 {
		t.Errorf("expected 0 audit entries after prune, got %d", total)
	}
}

// TestAuditPrunedByRollupWorker verifies the worker wiring: on the daily
// cleanup tick, audit entries older than the audit retention are pruned.
func TestAuditPrunedByRollupWorker(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()

	createHubViaAPI(t, ts.URL, stubHub.URL)
	if total := int(fetchAuditLogs(t, ts.URL, "")["total"].(float64)); total == 0 {
		t.Fatal("expected audit entries before prune")
	}

	// The fake clock starts at real now (audit rows are stamped on the wall
	// clock) so advancing four days pushes the 48h-retention cutoff past them.
	clock := scheduler.NewFakeClock(time.Now())
	worker := scheduler.NewRollupWorker(db, clock,
		scheduler.WithCleanupInterval(24*time.Hour),
		scheduler.WithAuditRetention(48*time.Hour),
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

	// Four days in one-hour steps so the cleanup tick fires repeatedly.
	for i := 0; i < 96; i++ {
		clock.Advance(time.Hour)
	}

	waitFor(t, "audit entries pruned by worker", func() bool {
		return int(fetchAuditLogs(t, ts.URL, "")["total"].(float64)) == 0
	})
}

// fetchAuditLogsAs queries GET /api/audit-logs with a specific client (so a
// Hub-scoped admin and a super_admin see different views) and returns the
// decoded page object.
func fetchAuditLogsAs(t *testing.T, client *http.Client, baseURL, query string) map[string]interface{} {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/audit-logs" + query)
	if err != nil {
		t.Fatalf("GET audit logs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("list audit logs: expected 200, got %d (%s)", resp.StatusCode, body)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	var page map[string]interface{}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatalf("unmarshal audit logs: %v", err)
	}
	return page
}

// auditItemsField is the items array extracted from a page returned by
// fetchAuditLogsAs (a thin wrapper around auditItems for the client variant).
func auditItemsField(t *testing.T, page map[string]interface{}) []map[string]interface{} {
	return auditItems(t, page)
}

// hasAuditActor reports whether any item in the page was authored by actor.
func hasAuditActor(items []map[string]interface{}, actor string) bool {
	for _, it := range items {
		if a, ok := it["actor"].(string); ok && a == actor {
			return true
		}
	}
	return false
}

// hasAuditAction reports whether any item in the page carries the action.
func hasAuditAction(items []map[string]interface{}, action string) bool {
	for _, it := range items {
		if a, ok := it["action"].(string); ok && a == action {
			return true
		}
	}
	return false
}

// TestAuditLogsHubIsolation verifies the three isolation classes of the
// audit log (spec 0005, ticket 66):
//  1. A Hub-scoped admin sees only rows stamped with their own hub_id
//     (their own writes + their own login).
//  2. They do NOT see another hub's rows (Hub-B admin's writes/login).
//  3. They do NOT see super_admin rows whose hub_id is NULL
//     (settings.update, the super_admin's own login).
//
// A super_admin sees all three classes.
func TestAuditLogsHubIsolation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	// Two hubs against the same working stub so model.create trials succeed
	// and each hub gets a real endpoint the hub admin can patch (no network
	// flakiness, no probe timeouts).
	hubA := int64(createHubViaAPI(t, ts.URL, stubHub.URL))
	hubB := int64(createHubViaAPI(t, ts.URL, stubHub.URL))

	// Create one model per hub via the super_admin API client; the response
	// carries the endpoint id the hub admin will patch.
	createEndpointIn := func(t *testing.T, hubID int64, modelID string) int64 {
		t.Helper()
		resp := doPost(t, ts.URL+"/api/models", map[string]interface{}{
			"hub_id":   hubID,
			"model_id": modelID,
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create model in hub %d: expected 201, got %d (%s)", hubID, resp.StatusCode, body)
		}
		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode model: %v", err)
		}
		var model map[string]interface{}
		if err := json.Unmarshal(env.Data, &model); err != nil {
			t.Fatalf("unmarshal model: %v", err)
		}
		eps := model["endpoints"].([]interface{})
		if len(eps) == 0 {
			t.Fatalf("model %s: expected at least one endpoint", modelID)
		}
		return int64(eps[0].(map[string]interface{})["id"].(float64))
	}
	epA := createEndpointIn(t, hubA, "iso-aud-model-a")
	epB := createEndpointIn(t, hubB, "iso-aud-model-b")

	const (
		aAdmin  = "iso-aud-a-admin"
		bAdmin  = "iso-aud-b-admin"
		saAdmin = "iso-aud-sa"
	)
	seedUserWithRole(t, db, aAdmin, store.RoleAdmin, &hubA)
	seedUserWithRole(t, db, bAdmin, store.RoleAdmin, &hubB)
	seedUserWithRole(t, db, saAdmin, store.RoleSuperAdmin, nil)
	aClient := loginAsClient(t, ts.URL, aAdmin)
	bClient := loginAsClient(t, ts.URL, bAdmin)
	saClient := loginAsClient(t, ts.URL, saAdmin)

	// Hub-A admin patches Hub-A's endpoint -> audit row, hub_id=hubA, actor=aAdmin.
	patchResp := doPatch(t, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epA), map[string]interface{}{"enabled": false})
	patchResp.Body.Close()
	// Hub-B admin patches Hub-B's endpoint -> audit row, hub_id=hubB, actor=bAdmin.
	patchRespB := postPatchAs(t, bClient, fmt.Sprintf("%s/api/endpoints/%d", ts.URL, epB), map[string]interface{}{"enabled": false})
	patchRespB.Body.Close()
	// super_admin updates global settings -> audit row, hub_id=NULL, action=settings.update.
	putResp := putAs(t, saClient, ts.URL+"/api/settings", map[string]interface{}{"alert_enabled": true})
	putResp.Body.Close()

	// Hub-A admin view: own hub only — sees their endpoint.patch (actor aAdmin),
	// does NOT see Hub-B's actor, and does NOT see the super_admin's
	// settings.update row (hub_id NULL is super_admin-only).
	aPage := fetchAuditLogsAs(t, aClient, ts.URL, "?page_size=100")
	aItems := auditItemsField(t, aPage)
	if !hasAuditActor(aItems, aAdmin) {
		t.Errorf("Hub-A admin audit view: expected own row (actor %s) to be visible", aAdmin)
	}
	if hasAuditActor(aItems, bAdmin) {
		t.Errorf("Hub-A admin audit view: Hub-B row (actor %s) must not be visible", bAdmin)
	}
	if hasAuditAction(aItems, "settings.update") {
		t.Errorf("Hub-A admin audit view: super_admin settings.update (hub_id NULL) must not be visible")
	}

	// super_admin view: all three classes — Hub-A row, Hub-B row, and the
	// NULL-hub settings.update row.
	saPage := fetchAuditLogsAs(t, saClient, ts.URL, "?page_size=100")
	saItems := auditItemsField(t, saPage)
	if !hasAuditActor(saItems, aAdmin) {
		t.Errorf("super_admin audit view: expected Hub-A row (actor %s)", aAdmin)
	}
	if !hasAuditActor(saItems, bAdmin) {
		t.Errorf("super_admin audit view: expected Hub-B row (actor %s)", bAdmin)
	}
	if !hasAuditAction(saItems, "settings.update") {
		t.Errorf("super_admin audit view: expected settings.update (hub_id NULL) row")
	}
}

// postPatchAs issues a JSON PATCH with the given client and returns the
// response, mirroring postAs/putAs for the hub-scoped admin clients.
func postPatchAs(t *testing.T, client *http.Client, url string, body interface{}) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	return resp
}
