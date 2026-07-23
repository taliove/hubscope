package server_test

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/taliove2009/hubscope/internal/store"
)

// isolatedListPaths are the GET list endpoints that must filter by the session
// user's hub_id for non-super_admin users. The isolation sweep asserts that a
// Hub-A admin cannot see Hub-B's data through any of them, while a
// super_admin sees both hubs. New list endpoints that carry hub-scoped data
// MUST be registered here (single-point maintenance) and will fail the sweep
// if they leak cross-hub rows.
var isolatedListPaths = []string{
	"/api/models",
	"/api/overview",
}

// markerB is a model_id string unique to Hub-B; its presence in a response
// body is the leak signal. markerA is the Hub-A counterpart.
const (
	markerA = "iso-model-a"
	markerB = "iso-model-b"
)

// TestPerHubIsolationSweep is the runtime guardrail for the per-hub query
// isolation invariant (spec 0005, new implicit wall). It seeds two hubs each
// with a distinct model, then walks isolatedListPaths as a Hub-A admin and as
// a super_admin, asserting no cross-hub leakage. It also pins the public
// detail routes as cross-hub accessible (spec 0005 decision 5) so a future
// mistake of hub-filtering them is caught.
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
	endpointB, err := db.ListEndpointsByModelID(modelB.ID)
	if err != nil || len(endpointB) == 0 {
		t.Fatalf("list endpoints for model b: %v (len=%d)", err, len(endpointB))
	}
	_ = modelA

	// Hub-A admin (hub-scoped) + a super_admin seeded with loginAsClient's
	// password scheme (the default "admin" uses a different password helper).
	seedUserWithRole(t, db, "iso-a-admin", store.RoleAdmin, &hubA.ID)
	seedUserWithRole(t, db, "iso-sa", store.RoleSuperAdmin, nil)
	aClient := loginAsClient(t, ts.URL, "iso-a-admin")
	saClient := loginAsClient(t, ts.URL, "iso-sa")
	anonClient := &http.Client{} // no cookie jar

	// Hub-A admin: every isolated list path must exclude Hub-B's marker and
	// include Hub-A's.
	for _, path := range isolatedListPaths {
		body := getBody(t, aClient, ts.URL+path)
		if strings.Contains(body, markerB) {
			t.Errorf("Hub-A admin GET %s: response leaks Hub-B data (%q present)", path, markerB)
		}
		if !strings.Contains(body, markerA) {
			t.Errorf("Hub-A admin GET %s: expected Hub-A data (%q) to be present", path, markerA)
		}
	}

	// Super_admin: every isolated list path must show both hubs.
	for _, path := range isolatedListPaths {
		body := getBody(t, saClient, ts.URL+path)
		if !strings.Contains(body, markerA) {
			t.Errorf("super_admin GET %s: expected Hub-A data (%q) to be present", path, markerA)
		}
		if !strings.Contains(body, markerB) {
			t.Errorf("super_admin GET %s: expected Hub-B data (%q) to be present", path, markerB)
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
