package server_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// TestCampaignDetailHubIsolation pins the per-hub isolation invariant (spec
// 0005) on the by-ID campaign/eval detail endpoints, closing the GH #149
// gaps: campaign detail, report, trends, eval-run detail and share-link
// creation must follow the campaigns list reachability rule — a hub-scoped
// session only reaches campaigns whose membership includes one of its hub's
// models; anything else answers the same 404 as an unknown id (no
// enumeration oracle). Super_admin sees everything; anonymous callers
// answer 401 (session-gated, never in publicReadPattern). This is the
// dedicated-test counterpart of the isolation sweep (isolation_test.go) for
// path-parameterized endpoints, alongside TestCampaignLiveFeedHubIsolation.
func TestCampaignDetailHubIsolation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	suite := suites[0]
	cases, err := db.ListCases(suite.ID)
	if err != nil || len(cases) == 0 {
		t.Fatalf("list cases: %v (n=%d)", err, len(cases))
	}
	caseRow := cases[0]

	type seeded struct {
		hubID      int64
		modelDBID  int64
		campaignID int64
		runID      int64
	}
	seed := func(hubName, modelID string) seeded {
		t.Helper()
		h, err := db.CreateHub(hubName, "http://"+hubName+".test", "tok-"+hubName+"-0000")
		if err != nil {
			t.Fatalf("create hub %s: %v", hubName, err)
		}
		m, err := db.CreateModel(h.ID, modelID, []string{"openai"})
		if err != nil {
			t.Fatalf("create model %s: %v", modelID, err)
		}
		c, err := db.CreateCampaign("manual", []int64{m.ID}, time.Now().UTC())
		if err != nil {
			t.Fatalf("create campaign (%s): %v", hubName, err)
		}
		run, err := db.CreateEvalRun(c.ID, suite.ID, "manual", "judge-x")
		if err != nil {
			t.Fatalf("create run (%s): %v", hubName, err)
		}
		score := 0.8
		if _, err := db.CreateEvalResult(store.EvalResult{
			EvalRunID: run.ID, ModelDBID: m.ID, ModelID: modelID, CaseID: caseRow.ID, Score: &score,
		}); err != nil {
			t.Fatalf("create result (%s): %v", hubName, err)
		}
		if err := db.FinishEvalRun(run.ID, "done", time.Now().UTC()); err != nil {
			t.Fatalf("finish run (%s): %v", hubName, err)
		}
		return seeded{hubID: h.ID, modelDBID: m.ID, campaignID: c.ID, runID: run.ID}
	}
	a := seed("det-iso-a", "det-model-a")
	b := seed("det-iso-b", "det-model-b")

	seedUserWithRole(t, db, "det-a-admin", store.RoleAdmin, &a.hubID)
	// A hub-scoped role without hub_id is a data inconsistency; it must see
	// nothing (same fallback as the campaigns list's empty result).
	seedUserWithRole(t, db, "det-nohub", store.RoleAdmin, nil)
	aClient := loginAsClient(t, ts.URL, "det-a-admin")
	nohubClient := loginAsClient(t, ts.URL, "det-nohub")
	saClient := authedClient(t, ts.URL)
	anonClient := &http.Client{} // no cookie jar

	getPaths := func(s seeded) map[string]string {
		return map[string]string{
			"campaign detail": fmt.Sprintf("/api/campaigns/%d", s.campaignID),
			"campaign report": fmt.Sprintf("/api/campaigns/%d/report", s.campaignID),
			"campaign trends": fmt.Sprintf("/api/campaigns/%d/trends?model=%d", s.campaignID, s.modelDBID),
			"eval run detail": fmt.Sprintf("/api/evals/%d", s.runID),
		}
	}

	// Own-hub: every detail endpoint serves the Hub-A admin.
	for name, path := range getPaths(a) {
		resp := getResp(t, aClient, ts.URL+path)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("hub-A admin GET %s (own hub): expected 200, got %d: %s", name, resp.StatusCode, body)
		}
	}
	// Own-hub share-link creation succeeds.
	resp := postAs(t, aClient, fmt.Sprintf("%s/api/campaigns/%d/share-links", ts.URL, a.campaignID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("hub-A admin POST share-links (own hub): expected 201, got %d", resp.StatusCode)
	}

	// Cross-hub and unknown ids answer identically: 404, no oracle. Unknown
	// eval-run id shares the campaign-id space poorly, so use a large id.
	for name, path := range getPaths(b) {
		resp := getResp(t, aClient, ts.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("hub-A admin GET %s (cross hub): expected 404, got %d", name, resp.StatusCode)
		}
	}
	for _, path := range []string{
		"/api/campaigns/99999",
		"/api/campaigns/99999/report",
		"/api/campaigns/99999/trends?model=1",
		"/api/evals/99999",
	} {
		resp := getResp(t, aClient, ts.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("hub-A admin GET %s (unknown id): expected 404, got %d", path, resp.StatusCode)
		}
	}
	resp = postAs(t, aClient, fmt.Sprintf("%s/api/campaigns/%d/share-links", ts.URL, b.campaignID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("hub-A admin POST share-links (cross hub): expected 404, got %d", resp.StatusCode)
	}

	// Hub-less non-super_admin: nothing is visible.
	for name, path := range getPaths(a) {
		resp := getResp(t, nohubClient, ts.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("hub-less admin GET %s: expected 404, got %d", name, resp.StatusCode)
		}
	}

	// Super_admin reaches both hubs' data on every endpoint.
	for name, path := range getPaths(b) {
		resp := getResp(t, saClient, ts.URL+path)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("super_admin GET %s (hub B): expected 200, got %d: %s", name, resp.StatusCode, body)
		}
	}
	resp = postAs(t, saClient, fmt.Sprintf("%s/api/campaigns/%d/share-links", ts.URL, b.campaignID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("super_admin POST share-links (hub B): expected 201, got %d", resp.StatusCode)
	}

	// Anonymous: session-gated, uniform 401.
	for name, path := range getPaths(a) {
		resp := getResp(t, anonClient, ts.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous GET %s: expected 401, got %d", name, resp.StatusCode)
		}
	}
}
