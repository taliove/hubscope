package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// This file pins the public (anonymous) view of GET /api/alerts (spec 0019
// ticket 1, ui-guidelines appendix item 16): anonymous readers see exactly
// the four incident-narrative kinds (down / recovered / group_down /
// group_recovered) at global scope, while the seven operational-pipeline
// kinds (test / batch / quiet_summary / score_drop / score_drop_skipped /
// retire_pending / retired) and every hub-less event stay session-only.
// The filter lives in the store query so the limit window is never diluted
// by hidden kinds. The authenticated three-branch behavior is pinned by the
// isolation sweep (isolation_test.go) and the existing alert tests.

// publicVisibleKinds are the four kinds anonymous readers may see.
var publicVisibleKinds = []string{"down", "recovered", "group_down", "group_recovered"}

// publicHiddenKinds are the seven kinds anonymous readers must never see.
var publicHiddenKinds = []string{
	"test", "batch", "quiet_summary",
	"score_drop", "score_drop_skipped",
	"retire_pending", "retired",
}

// listAlertsAnon fetches GET /api/alerts without a session, asserts 200, and
// returns the raw body plus the decoded events.
func listAlertsAnon(t *testing.T, ts *httptest.Server, query string) (string, []map[string]interface{}) {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/alerts" + query)
	if err != nil {
		t.Fatalf("anonymous GET /api/alerts%s: %v", query, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("anonymous GET /api/alerts%s: expected 200, got %d", query, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	var events []map[string]interface{}
	if err := json.Unmarshal(env.Data, &events); err != nil {
		t.Fatalf("unmarshal alerts: %v", err)
	}
	return string(data), events
}

// seedOneEndpoint creates a hub with one model and returns its first
// endpoint ID — the anchor for endpoint-bound alert events.
func seedOneEndpoint(t *testing.T, db *store.DB, hubName, modelID string) int64 {
	t.Helper()
	hub, err := db.CreateHub(hubName, "http://"+hubName+".test", "fake-token-0000")
	if err != nil {
		t.Fatalf("create hub %s: %v", hubName, err)
	}
	model, err := db.CreateModel(hub.ID, modelID, []string{"openai"})
	if err != nil {
		t.Fatalf("create model %s: %v", modelID, err)
	}
	endpoints, err := db.ListEndpointsByModelID(model.ID)
	if err != nil || len(endpoints) == 0 {
		t.Fatalf("list endpoints for %s: %v (n=%d)", modelID, err, len(endpoints))
	}
	return endpoints[0].ID
}

// seedAlert records one alert event; failures fatal the test.
func seedAlert(t *testing.T, db *store.DB, e store.AlertEvent) {
	t.Helper()
	if _, err := db.CreateAlertEvent(e); err != nil {
		t.Fatalf("seed alert %s: %v", e.Kind, err)
	}
}

// TestAnonymousAlertsSeesOnlyIncidentNarrative seeds all eleven alert kinds
// and asserts the anonymous payload is constructionally free of the seven
// hidden kinds and of every hub-less event: the whitelist is applied by the
// server, not by the frontend.
func TestAnonymousAlertsSeesOnlyIncidentNarrative(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	endpointID := seedOneEndpoint(t, db, "pub-hub", "pub-model")

	// The four visible kinds, each carrying a unique presence marker.
	groupKey := "testfamily"
	visible := []store.AlertEvent{
		{EndpointID: &endpointID, Kind: store.AlertKindDown, Message: "pubvis-down", SentOK: true},
		{EndpointID: &endpointID, Kind: store.AlertKindRecovered, Message: "pubvis-recovered", SentOK: true},
		{GroupKey: &groupKey, Kind: store.AlertKindGroupDown, Message: "pubvis-group-down", SentOK: true},
		{GroupKey: &groupKey, Kind: store.AlertKindGroupRecovered, Message: "pubvis-group-recovered", SentOK: true},
	}
	for _, e := range visible {
		seedAlert(t, db, e)
	}
	// The seven hidden kinds — all hub-less (endpoint_id and group_key NULL),
	// as they are emitted in production. The score_drop event doubles as the
	// hub-less probe (the isolation sweep's alertGlobalMarker counterpart).
	hidden := []store.AlertEvent{
		{Kind: store.AlertKindTest, Message: "pubhid-test", SentOK: true},
		{Kind: store.AlertKindBatch, Message: "pubhid-batch", SentOK: true},
		{Kind: store.AlertKindQuietSummary, Message: "pubhid-quiet", SentOK: true},
		{Kind: store.AlertKindScoreDrop, Message: "pubhid-scoredrop", SentOK: false},
		{Kind: store.AlertKindScoreDropSkipped, Message: "pubhid-skipped", SentOK: false},
		{Kind: store.AlertKindRetirePending, Message: "pubhid-retirepending", SentOK: true},
		{Kind: store.AlertKindRetired, Message: "pubhid-retired", SentOK: true},
	}
	for _, e := range hidden {
		seedAlert(t, db, e)
	}

	body, events := listAlertsAnon(t, ts, "")

	// Exactly the four visible events come back, nothing else.
	if len(events) != len(publicVisibleKinds) {
		t.Fatalf("anonymous payload: expected %d events, got %d: %s", len(publicVisibleKinds), len(events), body)
	}
	for _, e := range events {
		kind, _ := e["kind"].(string)
		found := false
		for _, k := range publicVisibleKinds {
			if kind == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("anonymous payload carries hidden kind %q: %s", kind, body)
		}
		// Hub-less events are constructionally excluded: every visible event
		// is endpoint-bound or carries a group key.
		if e["endpoint_id"] == nil && e["group_key"] == nil {
			t.Errorf("anonymous payload carries hub-less event (kind %q): %s", kind, body)
		}
	}

	// Raw-body greps: presence markers for the visible four, absence for the
	// hidden seven — by kind string (quoted, so "down" cannot false-match
	// "group_down") and by message marker.
	for _, k := range publicVisibleKinds {
		if !strings.Contains(body, `"kind":"`+k+`"`) {
			t.Errorf("anonymous payload should contain kind %q, got: %s", k, body)
		}
	}
	for _, k := range publicHiddenKinds {
		if strings.Contains(body, `"kind":"`+k+`"`) {
			t.Errorf("anonymous payload leaks hidden kind %q: %s", k, body)
		}
	}
	for _, marker := range []string{"pubvis-down", "pubvis-recovered", "pubvis-group-down", "pubvis-group-recovered"} {
		if !strings.Contains(body, marker) {
			t.Errorf("anonymous payload should contain %q, got: %s", marker, body)
		}
	}
	for _, marker := range []string{"pubhid-test", "pubhid-batch", "pubhid-quiet", "pubhid-scoredrop",
		"pubhid-skipped", "pubhid-retirepending", "pubhid-retired"} {
		if strings.Contains(body, marker) {
			t.Errorf("anonymous payload leaks hidden event %q: %s", marker, body)
		}
	}
}

// TestAnonymousAlertsGlobalAcrossHubs pins the global scope of the anonymous
// view: incident events from every hub are visible without a session, same
// level as the public status board overview. A future mistake of
// hub-filtering the anonymous branch fails this test.
func TestAnonymousAlertsGlobalAcrossHubs(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	epA := seedOneEndpoint(t, db, "pub-hub-a", "pub-model-a")
	epB := seedOneEndpoint(t, db, "pub-hub-b", "pub-model-b")
	seedAlert(t, db, store.AlertEvent{EndpointID: &epA, Kind: store.AlertKindDown, Message: "pubxhub-a", SentOK: true})
	seedAlert(t, db, store.AlertEvent{EndpointID: &epB, Kind: store.AlertKindDown, Message: "pubxhub-b", SentOK: true})

	body, events := listAlertsAnon(t, ts, "")
	if len(events) != 2 {
		t.Fatalf("anonymous cross-hub payload: expected 2 events, got %d: %s", len(events), body)
	}
	for _, marker := range []string{"pubxhub-a", "pubxhub-b"} {
		if !strings.Contains(body, marker) {
			t.Errorf("anonymous view must be global: expected %q present, got: %s", marker, body)
		}
	}
}

// TestAnonymousAlertsLimitAppliesAfterKindFilter proves the kind filter runs
// inside the store query, before LIMIT: with five newer hidden events on top
// of three older visible ones, limit=3 must return the three visible events.
// A handler post-filter would fetch the three newest (hidden) events and
// render an empty page — diluting the window and letting an empty state
// impersonate a clean record.
func TestAnonymousAlertsLimitAppliesAfterKindFilter(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	endpointID := seedOneEndpoint(t, db, "pub-lim-hub", "pub-lim-model")
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		seedAlert(t, db, store.AlertEvent{
			EndpointID: &endpointID, Kind: store.AlertKindDown,
			Message: "publim-down", SentOK: true, CreatedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	for i := 0; i < 5; i++ {
		seedAlert(t, db, store.AlertEvent{
			Kind: store.AlertKindBatch,
			// Newer than every visible event: an unfiltered LIMIT 3 window is
			// all batch events.
			Message: "publim-batch", SentOK: true, CreatedAt: base.Add(time.Duration(10+i) * time.Second),
		})
	}

	body, events := listAlertsAnon(t, ts, "?limit=3")
	if len(events) != 3 {
		t.Fatalf("limit=3: expected the 3 visible events, got %d: %s", len(events), body)
	}
	for _, e := range events {
		if e["kind"].(string) != "down" {
			t.Errorf("limit=3: expected only down events, got kind %q: %s", e["kind"], body)
		}
	}
	if strings.Contains(body, "publim-batch") {
		t.Errorf("limit window diluted by hidden kinds: %s", body)
	}
}
