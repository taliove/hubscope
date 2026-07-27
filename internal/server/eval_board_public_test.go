package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// getPublicEvalBoard fetches GET /api/public/eval/board anonymously (no
// session cookie — the endpoint is public by design, spec 0010) and returns
// the decoded payload.
func getPublicEvalBoard(t *testing.T, base string) map[string]interface{} {
	t.Helper()
	resp := plainGet(t, base+"/api/public/eval/board")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/public/eval/board: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode public eval board: %v", err)
	}
	var board map[string]interface{}
	if err := json.Unmarshal(env.Data, &board); err != nil {
		t.Fatalf("unmarshal public eval board: %v", err)
	}
	return board
}

// TestPublicEvalBoardEmpty covers the no-settled-campaign case: an anonymous
// request still answers 200, the report is null and the running flag false.
func TestPublicEvalBoardEmpty(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	board := getPublicEvalBoard(t, ts.URL)
	if board["report"] != nil {
		t.Errorf("report = %v, want null without any settled campaign", board["report"])
	}
	if board["running"] != false {
		t.Errorf("running = %v, want false without any campaign", board["running"])
	}
}

// TestPublicEvalBoardOnlyRunning covers the combination state: a campaign in
// flight but nothing settled yet — the report stays null while the running
// flag is true (the public page renders its empty state plus the hint line).
func TestPublicEvalBoardOnlyRunning(t *testing.T) {
	ts, _, db := setupEvalEnv(t)

	if _, err := db.CreateCampaign("manual", nil, time.Now().UTC()); err != nil {
		t.Fatalf("seed running campaign: %v", err)
	}
	board := getPublicEvalBoard(t, ts.URL)
	if board["report"] != nil {
		t.Errorf("report = %v, want null with only a running campaign", board["report"])
	}
	if board["running"] != true {
		t.Errorf("running = %v, want true with a campaign in flight", board["running"])
	}
}

// TestPublicEvalBoardIgnoresQueryParams pins spec 0010: the endpoint takes
// no sort/family params — an anonymous request carrying them gets the
// default board (total-desc, unfiltered), identical to a param-free call.
func TestPublicEvalBoardIgnoresQueryParams(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "dumb-model")

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	plain := getPublicEvalBoard(t, ts.URL)
	resp := plainGet(t, ts.URL+"/api/public/eval/board?family=zzz&sort=nosuch")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/public/eval/board with params: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode public eval board: %v", err)
	}
	var withParams map[string]interface{}
	if err := json.Unmarshal(env.Data, &withParams); err != nil {
		t.Fatalf("unmarshal public eval board: %v", err)
	}
	if !reflect.DeepEqual(plain, withParams) {
		t.Errorf("query params must not affect the board:\nplain:  %v\nparams: %v", plain, withParams)
	}
}

// TestPublicEvalBoardLatestSettled covers the core contract (spec 0010): the
// board serves the newest settled campaign — done or failed alike, skipping
// running/pending ones — the payload is byte-identical to the session-gated
// campaign report of the same campaign, and the running flag tracks the
// existence of an unfinished campaign.
func TestPublicEvalBoardLatestSettled(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "smart-model")
	createEvalModel(t, ts.URL, stub.URL, "dumb-model")

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	// Settled done campaign: anonymously reachable, no unfinished campaign.
	board := getPublicEvalBoard(t, ts.URL)
	report, ok := board["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing after a done campaign: %v", board)
	}
	if got := int64(report["id"].(float64)); got != campaignID {
		t.Errorf("report campaign id = %d, want %d", got, campaignID)
	}
	if board["running"] != false {
		t.Errorf("running = %v, want false with every campaign settled", board["running"])
	}

	// Same shape as the session-gated report — the public endpoint reuses
	// the same serialization, so the two payloads are identical for a
	// settled campaign (default total-desc ranking, no family filter).
	sessionReport := getCampaignReport(t, ts.URL, campaignID, "")
	if !reflect.DeepEqual(report, sessionReport) {
		t.Errorf("public report differs from the session report:\npublic:  %v\nsession: %v", report, sessionReport)
	}
	// The board keeps the report contract: ranked rows with consistent
	// totals, weights and the baseline slot.
	assertRowTotals(t, report)
	if _, ok := report["weights"]; !ok {
		t.Error("report weights missing")
	}
	if _, ok := report["baseline"]; report["baseline"] == nil && !ok {
		t.Error("report baseline key missing")
	}
	rows := reportRows(t, report)
	if len(rows) != 2 {
		t.Fatalf("report rows = %d, want 2", len(rows))
	}
	first, firstOk := rows[0]["total_score"].(float64)
	second, secondOk := rows[1]["total_score"].(float64)
	if firstOk && secondOk && first < second {
		t.Errorf("rows not ranked by total descending: %v then %v", first, second)
	}

	// A running campaign flips the flag but never displaces the settled one.
	seeded, err := db.CreateCampaign("manual", nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("seed running campaign: %v", err)
	}
	board = getPublicEvalBoard(t, ts.URL)
	if board["running"] != true {
		t.Errorf("running = %v, want true with a running campaign", board["running"])
	}
	if got := int64(board["report"].(map[string]interface{})["id"].(float64)); got != campaignID {
		t.Errorf("report campaign id = %d, want still %d (running campaign must not leak)", got, campaignID)
	}

	// Failed campaigns count as settled too: once the seeded campaign
	// settles (zero runs -> failed), it becomes the board's latest.
	if err := db.SettleCampaign(seeded.ID, time.Now().UTC()); err != nil {
		t.Fatalf("settle seeded campaign: %v", err)
	}
	board = getPublicEvalBoard(t, ts.URL)
	latest, ok := board["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing after the failed campaign settled: %v", board)
	}
	if got := int64(latest["id"].(float64)); got != seeded.ID {
		t.Errorf("report campaign id = %d, want the freshly failed campaign %d", got, seeded.ID)
	}
	if latest["status"] != store.CampaignStatusFailed {
		t.Errorf("latest campaign status = %v, want failed", latest["status"])
	}
	if board["running"] != false {
		t.Errorf("running = %v, want false after every campaign settled", board["running"])
	}
}

// TestPublicEvalBoardNoWriteExposure asserts the public path exposes no
// write surface: an anonymous POST is rejected (405 from the router, or 401
// from the session gate — never a 2xx).
func TestPublicEvalBoardNoWriteExposure(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	resp, err := http.Post(ts.URL+"/api/public/eval/board", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/public/eval/board: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("anonymous POST must not succeed, got %d: %s", resp.StatusCode, b)
	}
}
