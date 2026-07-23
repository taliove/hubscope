package server_test

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taliove2009/hubscope/internal/store"
)

// isolatedListPath is one GET list endpoint that must filter by the session
// user's hub_id for non-super_admin users. markerA/markerB are Hub-A/Hub-B
// leak signals (strings that appear in the JSON body when that hub's data is
// present); globalMarker, when non-empty, is a super_admin-only signal for
// data with no hub ownership (score_drop alerts, rollup/retention tasks).
// The isolation sweep asserts a Hub-A admin sees markerA and neither
// markerB nor globalMarker, while a super_admin sees all three. New list
// endpoints that carry hub-scoped data MUST be registered here (single-point
// maintenance) and will fail the sweep if they leak cross-hub rows.
type isolatedListPath struct {
	path         string
	markerA      string
	markerB      string
	globalMarker string
}

// isolatedListPaths is the registry of hub-scoped list endpoints.
var isolatedListPaths = []isolatedListPath{
	{"/api/models", markerA, markerB, ""},
	{"/api/overview", markerA, markerB, ""},
	{"/api/campaigns", campaignMarkerA, campaignMarkerB, ""},
	{"/api/evals", evalJudgeA, evalJudgeB, ""},
	{"/api/evals/latest", markerA, markerB, ""},
	{"/api/alerts", alertMarkerA, alertMarkerB, alertGlobalMarker},
	{"/api/tasks", taskSrcA, taskSrcB, taskSrcGlobal},
	{"/api/share-links", shareTokenA, shareTokenB, ""},
}

// Leak-signal markers. markerA/markerB are model_id strings unique to each
// hub; their presence in a response body is the leak signal. The remaining
// constants are per-resource markers carried by the seeded rows' free-text
// fields (campaign trigger, eval run judge_model, alert message, task source,
// share token). All values are fake test-only strings (W6: no real tokens).
const (
	markerA = "iso-model-a"
	markerB = "iso-model-b"

	campaignMarkerA = "iso-campaign-a"
	campaignMarkerB = "iso-campaign-b"

	evalJudgeA = "iso-judge-a"
	evalJudgeB = "iso-judge-b"

	alertMarkerA      = "iso-alert-a"
	alertMarkerB      = "iso-alert-b"
	alertGlobalMarker = "iso-alert-global"

	taskSrcA      = "iso-task-src-a"
	taskSrcB      = "iso-task-src-b"
	taskSrcGlobal = "iso-task-src-global"

	shareTokenA = "iso-share-token-a"
	shareTokenB = "iso-share-token-b"
)

// TestPerHubIsolationSweep is the runtime guardrail for the per-hub query
// isolation invariant (spec 0005, new implicit wall). It seeds two hubs each
// with a distinct model and derived data (campaign / eval run / alert / task
// share link) carrying hub-unique markers, plus hub-less global rows
// (score_drop alert, rollup task) that only super_admin may see. It then
// walks isolatedListPaths as a Hub-A admin and as a super_admin, asserting
// no cross-hub leakage and correct global-data visibility. It also pins the
// public detail routes as cross-hub accessible (spec 0005 decision 5) so a
// future mistake of hub-filtering them is caught.
func TestPerHubIsolationSweep(t *testing.T) {
	db := openTempDB(t) // seeds super_admin "admin"
	ts := newTestAPIServer(t, db)

	hubA, err := db.CreateHub("iso-hub-a", "http://a.test", "tok-iso-a-0000")
	if err != nil {
		t.Fatalf("create hub a: %v", err)
	}
	hubB, err := db.CreateHub("iso-hub-b", "http://b.test", "tok-iso-b-0000")
	if err != nil {
		t.Fatalf("create hub b: %v", err)
	}

	// Each hub gets one model with one openai endpoint. The model_id strings
	// double as presence markers in the JSON response bodies.
	modelA, err := db.CreateModel(hubA.ID, markerA, []string{"openai"})
	if err != nil {
		t.Fatalf("create model a: %v", err)
	}
	modelB, err := db.CreateModel(hubB.ID, markerB, []string{"openai"})
	if err != nil {
		t.Fatalf("create model b: %v", err)
	}
	endpointA, err := db.ListEndpointsByModelID(modelA.ID)
	if err != nil || len(endpointA) == 0 {
		t.Fatalf("list endpoints for model a: %v (len=%d)", err, len(endpointA))
	}
	endpointB, err := db.ListEndpointsByModelID(modelB.ID)
	if err != nil || len(endpointB) == 0 {
		t.Fatalf("list endpoints for model b: %v (len=%d)", err, len(endpointB))
	}

	// One suite + case for eval-result seeding (openTempDB seeds the suite
	// bank; pick the first suite and its first case).
	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	suite := suites[0]
	cases, err := db.ListCases(suite.ID)
	if err != nil || len(cases) == 0 {
		t.Fatalf("list cases for suite %d: %v (n=%d)", suite.ID, err, len(cases))
	}
	caseRow := cases[0]

	// Per-hub derived data. Each hub's campaign only contains that hub's
	// model (single-hub campaign seed). Cross-hub campaigns are a known
	// write-path gap left to ticket 65b.
	seedHubDerived(t, db, hubA.ID, modelA.ID, markerA, suite.ID, caseRow.ID,
		campaignMarkerA, evalJudgeA, alertMarkerA, &endpointA[0].ID, taskSrcA, shareTokenA)
	seedHubDerived(t, db, hubB.ID, modelB.ID, markerB, suite.ID, caseRow.ID,
		campaignMarkerB, evalJudgeB, alertMarkerB, &endpointB[0].ID, taskSrcB, shareTokenB)

	// Hub-less global rows: a score_drop alert (endpoint_id NULL) and a
	// rollup task (entity_type empty). These belong to no hub, so only
	// super_admin sees them via the *All store variants.
	if _, err := db.CreateAlertEvent(store.AlertEvent{
		Kind:    store.AlertKindScoreDrop,
		Message: alertGlobalMarker,
		SentOK:  false,
	}); err != nil {
		t.Fatalf("seed global alert: %v", err)
	}
	if _, err := db.CreateTask(store.Task{
		Type:       store.TaskTypeRollup,
		Source:     taskSrcGlobal,
		EntityType: "",
		EntityID:   0,
	}); err != nil {
		t.Fatalf("seed global task: %v", err)
	}

	// Hub-A admin (hub-scoped) + a super_admin seeded with loginAsClient's
	// password scheme (the default "admin" uses a different password helper).
	seedUserWithRole(t, db, "iso-a-admin", store.RoleAdmin, &hubA.ID)
	seedUserWithRole(t, db, "iso-sa", store.RoleSuperAdmin, nil)
	aClient := loginAsClient(t, ts.URL, "iso-a-admin")
	saClient := loginAsClient(t, ts.URL, "iso-sa")
	anonClient := &http.Client{} // no cookie jar

	// Hub-A admin: every isolated list path must include Hub-A's marker and
	// exclude Hub-B's marker and the global marker.
	for _, p := range isolatedListPaths {
		body := getBody(t, aClient, ts.URL+p.path)
		if !strings.Contains(body, p.markerA) {
			t.Errorf("Hub-A admin GET %s: expected Hub-A data (%q) to be present", p.path, p.markerA)
		}
		if strings.Contains(body, p.markerB) {
			t.Errorf("Hub-A admin GET %s: response leaks Hub-B data (%q present)", p.path, p.markerB)
		}
		if p.globalMarker != "" && strings.Contains(body, p.globalMarker) {
			t.Errorf("Hub-A admin GET %s: response leaks global-only data (%q present)", p.path, p.globalMarker)
		}
	}

	// Super_admin: every isolated list path must show both hubs' markers, and
	// the global marker where applicable.
	for _, p := range isolatedListPaths {
		body := getBody(t, saClient, ts.URL+p.path)
		if !strings.Contains(body, p.markerA) {
			t.Errorf("super_admin GET %s: expected Hub-A data (%q) to be present", p.path, p.markerA)
		}
		if !strings.Contains(body, p.markerB) {
			t.Errorf("super_admin GET %s: expected Hub-B data (%q) to be present", p.path, p.markerB)
		}
		if p.globalMarker != "" && !strings.Contains(body, p.globalMarker) {
			t.Errorf("super_admin GET %s: expected global data (%q) to be present", p.path, p.globalMarker)
		}
	}

	// Anonymous overview stays global (public status board semantics); the
	// /api/models path is NOT in publicReadPattern, so anonymous must hit 401.
	anonOverview := getBody(t, anonClient, ts.URL+"/api/overview")
	if !strings.Contains(anonOverview, markerA) || !strings.Contains(anonOverview, markerB) {
		t.Errorf("anonymous overview: expected global aggregation with both markers, got %q", anonOverview)
	}
	anonModels := getResp(t, anonClient, ts.URL+"/api/models")
	anonModels.Body.Close()
	if anonModels.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous /api/models: expected 401, got %d", anonModels.StatusCode)
	}

	// Public detail routes stay cross-hub accessible (spec 0005 decision 5):
	// a Hub-A admin can fetch Hub-B's endpoint detail and eval summary without
	// a 404, because status board detail is public. Pinning this prevents a
	// future mistake of hub-filtering these routes.
	for _, path := range []string{
		"/api/endpoints/" + strconv.FormatInt(endpointB[0].ID, 10),
		"/api/models/" + strconv.FormatInt(modelB.ID, 10) + "/eval-summary",
	} {
		resp := getResp(t, aClient, ts.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Hub-A admin GET %s (cross-hub public detail): expected 200, got %d", path, resp.StatusCode)
		}
	}
}

// seedHubDerived plants one hub's worth of derived data carrying the given
// markers: a single-hub campaign, a done eval run with one scored result, a
// down alert on the hub's endpoint, an eval_run task pointing at the run,
// and a share link on the campaign. All markers are chosen by the caller so
// the sweep can assert on them per-resource.
func seedHubDerived(t *testing.T, db *store.DB, hubID, modelDBID int64, modelID string,
	suiteID, caseID int64, campaignTrigger, judgeModel, alertMsg string,
	endpointID *int64, taskSource, shareToken string) {
	t.Helper()

	campaign, err := db.CreateCampaign(campaignTrigger, []int64{modelDBID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("seed campaign (hub=%d): %v", hubID, err)
	}

	run, err := db.CreateEvalRun(campaign.ID, suiteID, "manual", judgeModel)
	if err != nil {
		t.Fatalf("seed eval run (hub=%d): %v", hubID, err)
	}
	score := 0.8
	if _, err := db.CreateEvalResult(store.EvalResult{
		EvalRunID: run.ID,
		ModelDBID: modelDBID,
		ModelID:   modelID,
		CaseID:    caseID,
		Score:     &score,
	}); err != nil {
		t.Fatalf("seed eval result (hub=%d): %v", hubID, err)
	}
	if err := db.FinishEvalRun(run.ID, "done", time.Now().UTC()); err != nil {
		t.Fatalf("finish eval run (hub=%d): %v", hubID, err)
	}

	if _, err := db.CreateAlertEvent(store.AlertEvent{
		EndpointID: endpointID,
		Kind:       store.AlertKindDown,
		Message:    alertMsg,
		SentOK:     true,
	}); err != nil {
		t.Fatalf("seed alert (hub=%d): %v", hubID, err)
	}

	if _, err := db.CreateTask(store.Task{
		Type:       store.TaskTypeEvalRun,
		Source:     taskSource,
		EntityType: store.TaskEntityEvalRun,
		EntityID:   run.ID,
	}); err != nil {
		t.Fatalf("seed task (hub=%d): %v", hubID, err)
	}

	if _, err := db.CreateShareLink(shareToken, campaign.ID, "iso-seeder", time.Now().UTC()); err != nil {
		t.Fatalf("seed share link (hub=%d): %v", hubID, err)
	}
}

// getResp issues a GET and returns the response without reading the body.
func getResp(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// getBody issues a GET, asserts 200, and returns the response body as a string.
func getBody(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp := getResp(t, client, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", url, err)
	}
	return string(data)
}
