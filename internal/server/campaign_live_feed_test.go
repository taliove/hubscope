package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// getLiveFeed fetches GET /api/campaigns/{id}/live-feed with the given raw
// query string using the given client, asserts HTTP 200, and returns the
// decoded entries (ascending by id per the contract).
func getLiveFeed(t *testing.T, client *http.Client, base string, campaignID int64, query string) []map[string]interface{} {
	t.Helper()
	url := fmt.Sprintf("%s/api/campaigns/%d/live-feed", base, campaignID)
	if query != "" {
		url += "?" + query
	}
	resp := getResp(t, client, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET live-feed (campaign=%d, query=%q): expected 200, got %d: %s",
			campaignID, query, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode live-feed: %v", err)
	}
	var raw []interface{}
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatalf("unmarshal live-feed entries: %v", err)
	}
	entries := make([]map[string]interface{}, 0, len(raw))
	for _, e := range raw {
		entries = append(entries, e.(map[string]interface{}))
	}
	return entries
}

// liveFeedIDs extracts the entry ids in response order.
func liveFeedIDs(entries []map[string]interface{}) []int64 {
	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, int64(e["id"].(float64)))
	}
	return ids
}

// TestCampaignLiveFeedCursorAndFields pins the issue #17 live-feed contract:
// the stream serves every judged-case event of the campaign ascending by id
// with the full field set (model / suite / case / verdict method / score /
// latency / time), a null score survives as JSON null (judge failure is
// null, never zero — W7), the since_id cursor is strictly greater-than so
// repeated polls never re-send old entries, an empty increment is an empty
// array, limit caps the page, and bad cursors answer 400 while unknown
// campaigns answer 404.
func TestCampaignLiveFeedCursorAndFields(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hub, err := db.CreateHub("feed-hub", "http://feed.test", "tok-feed-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	model, err := db.CreateModel(hub.ID, "feed-model", []string{"openai"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
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

	campaign, err := db.CreateCampaign("manual", []int64{model.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	run, err := db.CreateEvalRun(campaign.ID, suite.ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	one, half := 1.0, 0.5
	mkResult := func(score *float64, latency int) *store.EvalResult {
		t.Helper()
		r, err := db.CreateEvalResult(store.EvalResult{
			EvalRunID: run.ID,
			ModelDBID: model.ID,
			ModelID:   "feed-model",
			CaseID:    caseRow.ID,
			Score:     score,
			LatencyMs: latency,
		})
		if err != nil {
			t.Fatalf("create result: %v", err)
		}
		return r
	}
	r1 := mkResult(&one, 120)
	r2 := mkResult(nil, 240) // judge failure: score must stay JSON null
	r3 := mkResult(&half, 360)

	client := authedClient(t, ts.URL)

	// Full pull: every event, ascending by id, full field set.
	entries := getLiveFeed(t, client, ts.URL, campaign.ID, "")
	if got, want := liveFeedIDs(entries), []int64{r1.ID, r2.ID, r3.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("full pull ids = %v, want %v (ascending)", got, want)
	}
	e := entries[0]
	for _, field := range []string{"model_id", "suite_key", "suite_name", "case_id", "case_prompt", "verdict_type", "score", "latency_ms", "created_at"} {
		if _, ok := e[field]; !ok {
			t.Errorf("entry missing field %q: %v", field, e)
		}
	}
	if e["model_id"] != "feed-model" {
		t.Errorf("model_id = %v, want feed-model", e["model_id"])
	}
	if e["suite_key"] != suite.Key || e["suite_name"] != suite.Name {
		t.Errorf("suite = %v/%v, want %v/%v", e["suite_key"], e["suite_name"], suite.Key, suite.Name)
	}
	if int64(e["case_id"].(float64)) != caseRow.ID {
		t.Errorf("case_id = %v, want %d", e["case_id"], caseRow.ID)
	}
	if e["case_prompt"] != caseRow.Prompt {
		t.Errorf("case_prompt = %q, want the case prompt", e["case_prompt"])
	}
	if e["verdict_type"] != caseRow.VerdictType {
		t.Errorf("verdict_type = %v, want %v", e["verdict_type"], caseRow.VerdictType)
	}
	if e["score"] != 1.0 {
		t.Errorf("score = %v, want 1.0 (raw 0~1 scale)", e["score"])
	}
	if int(e["latency_ms"].(float64)) != 120 {
		t.Errorf("latency_ms = %v, want 120", e["latency_ms"])
	}
	if ts_, ok := e["created_at"].(string); !ok || ts_ == "" {
		t.Errorf("created_at = %v, want a non-empty RFC3339 string", e["created_at"])
	}
	// Judge failure: the score key is present and null — never dropped, never 0.
	if v, ok := entries[1]["score"]; !ok || v != nil {
		t.Errorf("judge-failure entry score = %v (present=%v), want explicit null", v, ok)
	}

	// Cursor is strictly greater-than: r2's id excludes r2 itself.
	entries = getLiveFeed(t, client, ts.URL, campaign.ID, fmt.Sprintf("since_id=%d", r2.ID))
	if got, want := liveFeedIDs(entries), []int64{r3.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("since_id=%d ids = %v, want %v", r2.ID, got, want)
	}

	// Empty increment is an empty array, not an error and not null.
	resp := getResp(t, client, fmt.Sprintf("%s/api/campaigns/%d/live-feed?since_id=%d", ts.URL, campaign.ID, r3.ID))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty increment: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"data":[]`) {
		t.Errorf("empty increment body = %s, want \"data\":[]", body)
	}

	// New results land after the cursor: exactly the increment, in order.
	r4 := mkResult(&one, 10)
	r5 := mkResult(&half, 20)
	entries = getLiveFeed(t, client, ts.URL, campaign.ID, fmt.Sprintf("since_id=%d", r3.ID))
	if got, want := liveFeedIDs(entries), []int64{r4.ID, r5.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("increment after new results ids = %v, want %v", got, want)
	}

	// Limit caps the page at the lowest ids (the client walks forward).
	entries = getLiveFeed(t, client, ts.URL, campaign.ID, "limit=2")
	if got, want := liveFeedIDs(entries), []int64{r1.ID, r2.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("limit=2 ids = %v, want %v", got, want)
	}

	// Bad cursors are client errors; unknown campaigns are 404.
	for _, query := range []string{"since_id=abc", "since_id=-1"} {
		resp := getResp(t, client, fmt.Sprintf("%s/api/campaigns/%d/live-feed?%s", ts.URL, campaign.ID, query))
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d", query, resp.StatusCode)
		}
	}
	resp = getResp(t, client, fmt.Sprintf("%s/api/campaigns/99999/live-feed", ts.URL))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown campaign: expected 404, got %d", resp.StatusCode)
	}
}

// TestCampaignLiveFeedHubIsolation pins the W6 data-isolation caliber of the
// live feed: it follows the campaigns list reachability rule — a hub-scoped
// session only sees campaigns whose membership includes one of its hub's
// models; another hub's campaign answers the same 404 as an unknown one (no
// enumeration oracle), super_admin sees everything, and anonymous callers
// answer 401 (the path is not in publicReadPattern, and the share-token
// surface never serves it).
func TestCampaignLiveFeedHubIsolation(t *testing.T) {
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

	seedCampaign := func(hubName, modelID string) (int64, int64) {
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
		return c.ID, h.ID
	}
	campaignA, hubAID := seedCampaign("feed-iso-a", "feed-model-a")
	campaignB, _ := seedCampaign("feed-iso-b", "feed-model-b")

	seedUserWithRole(t, db, "feed-a-admin", store.RoleAdmin, &hubAID)
	aClient := loginAsClient(t, ts.URL, "feed-a-admin")
	saClient := authedClient(t, ts.URL)

	// Own-hub campaign: served.
	entries := getLiveFeed(t, aClient, ts.URL, campaignA, "")
	if len(entries) != 1 || entries[0]["model_id"] != "feed-model-a" {
		t.Fatalf("hub-A admin own feed = %v, want the single feed-model-a entry", entries)
	}

	// Cross-hub campaign: same 404 as an unknown campaign — no oracle.
	for _, id := range []int64{campaignB, 99999} {
		resp := getResp(t, aClient, fmt.Sprintf("%s/api/campaigns/%d/live-feed", ts.URL, id))
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("hub-A admin campaign %d: expected 404, got %d: %s", id, resp.StatusCode, body)
		}
	}

	// Super_admin sees both hubs' campaigns.
	entries = getLiveFeed(t, saClient, ts.URL, campaignB, "")
	if len(entries) != 1 || entries[0]["model_id"] != "feed-model-b" {
		t.Fatalf("super_admin hub-B feed = %v, want the single feed-model-b entry", entries)
	}

	// Anonymous: 401 (session-gated; not whitelisted in publicReadPattern).
	anonClient := &http.Client{}
	resp := getResp(t, anonClient, fmt.Sprintf("%s/api/campaigns/%d/live-feed", ts.URL, campaignA))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous live-feed: expected 401, got %d", resp.StatusCode)
	}
}
