package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/taliove2009/hubscope/internal/server"
	"github.com/taliove2009/hubscope/internal/store"
)

// shareTokenPattern pins the token contract: 32 crypto/rand bytes hex-encoded
// (256 bits of entropy, well above the 128-bit floor of ADR 0006).
var shareTokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// anonPost issues a session-less POST, for asserting the share-link write
// endpoints stay behind the admin session.
func anonPost(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// anonDelete issues a session-less DELETE.
func anonDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		t.Fatalf("build DELETE %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp
}

// readBody returns the full response body as a string.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// createShareLink posts a share-link creation for the campaign (authed) and
// asserts 201, returning the decoded link payload.
func createShareLink(t *testing.T, base string, campaignID int64) map[string]interface{} {
	t.Helper()
	resp := doPost(t, fmt.Sprintf("%s/api/campaigns/%d/share-links", base, campaignID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create share link: expected 201, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode share link: %v", err)
	}
	var link map[string]interface{}
	if err := json.Unmarshal(env.Data, &link); err != nil {
		t.Fatalf("unmarshal share link: %v", err)
	}
	return link
}

// setupDoneCampaign builds one scored model and a settled campaign, returning
// the campaign ID.
func setupDoneCampaign(t *testing.T) (*httptest.Server, int64) {
	t.Helper()
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	basicID := suiteIDByKey(t, ts.URL, "basic")
	runID := triggerEval(t, ts.URL, basicID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)
	return ts, campaignID
}

// getSharedReport fetches the public shared report by token and asserts 200.
func getSharedReport(t *testing.T, base, token string) map[string]interface{} {
	t.Helper()
	resp := plainGet(t, base+"/api/shared-reports/"+token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("shared report: expected 200, got %d: %s", resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode shared report: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(env.Data, &report); err != nil {
		t.Fatalf("unmarshal shared report: %v", err)
	}
	return report
}

// TestShareLinkLifecycle covers the ticket 33 acceptance criteria: an
// anonymous reader opens the report by token alone; a wrong or malformed
// token and a revoked link all answer the identical 404 (no enumeration
// oracle); creation and revocation land in the audit log; the management
// endpoints stay session-gated.
func TestShareLinkLifecycle(t *testing.T) {
	ts, campaignID := setupDoneCampaign(t)

	// The authed report is the baseline the shared view must mirror.
	authedReport := getCampaignReport(t, ts.URL, campaignID, "")
	authedRows := reportRows(t, authedReport)
	if len(authedRows) != 1 {
		t.Fatalf("baseline report rows = %v, want exactly one scored model", authedRows)
	}

	// Creation is session-gated.
	resp := anonPost(t, fmt.Sprintf("%s/api/campaigns/%d/share-links", ts.URL, campaignID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous create: expected 401, got %d", resp.StatusCode)
	}

	link := createShareLink(t, ts.URL, campaignID)
	token, _ := link["token"].(string)
	if !shareTokenPattern.MatchString(token) {
		t.Fatalf("token %q does not match the 64-hex contract", token)
	}
	if int64(link["campaign_id"].(float64)) != campaignID {
		t.Errorf("link campaign_id = %v, want %d", link["campaign_id"], campaignID)
	}
	if link["created_by"] != "admin" {
		t.Errorf("link created_by = %v, want admin", link["created_by"])
	}
	if link["revoked_at"] != nil {
		t.Errorf("fresh link revoked_at = %v, want null", link["revoked_at"])
	}

	// Anonymous read by token: 200 with the same leaderboard rows.
	sharedResp := plainGet(t, ts.URL+"/api/shared-reports/"+token)
	if sharedResp.StatusCode != http.StatusOK {
		t.Fatalf("shared report: expected 200, got %d: %s", sharedResp.StatusCode, readBody(t, sharedResp))
	}
	var env envelope
	if err := json.NewDecoder(sharedResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode shared report: %v", err)
	}
	sharedResp.Body.Close()
	var shared map[string]interface{}
	if err := json.Unmarshal(env.Data, &shared); err != nil {
		t.Fatalf("unmarshal shared report: %v", err)
	}
	sharedRows := reportRows(t, shared)
	if len(sharedRows) != 1 || sharedRows[0]["model_id"] != authedRows[0]["model_id"] {
		t.Errorf("shared rows = %v, want the same single row as the authed report", sharedRows)
	}
	if sharedRows[0]["total_score"] != authedRows[0]["total_score"] {
		t.Errorf("shared total = %v, want %v", sharedRows[0]["total_score"], authedRows[0]["total_score"])
	}

	// The token opens only the shared view: the authed report API and the
	// management list still reject anonymous callers.
	resp = plainGet(t, fmt.Sprintf("%s/api/campaigns/%d/report", ts.URL, campaignID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous authed-report read: expected 401, got %d", resp.StatusCode)
	}
	resp = plainGet(t, ts.URL+"/api/share-links")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous share-link list: expected 401, got %d", resp.StatusCode)
	}
	resp = anonDelete(t, fmt.Sprintf("%s/api/share-links/%d", ts.URL, int64(link["id"].(float64))))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous revoke: expected 401, got %d", resp.StatusCode)
	}

	// A wrong but well-formed token 404s; capture the exact body as the
	// anti-enumeration reference for the revoked-link comparison below.
	wrongToken := "0000000000000000000000000000000000000000000000000000000000000000"
	wrongResp := plainGet(t, ts.URL+"/api/shared-reports/"+wrongToken)
	wrongBody := readBody(t, wrongResp)
	if wrongResp.StatusCode != http.StatusNotFound {
		t.Errorf("wrong token: expected 404, got %d", wrongResp.StatusCode)
	}

	// A malformed token 404s identically (never 401, never a different body).
	malformedResp := plainGet(t, ts.URL+"/api/shared-reports/not-a-real-token")
	malformedBody := readBody(t, malformedResp)
	if malformedResp.StatusCode != http.StatusNotFound {
		t.Errorf("malformed token: expected 404, got %d", malformedResp.StatusCode)
	}
	if malformedBody != wrongBody {
		t.Errorf("malformed-token body %q differs from wrong-token body %q", malformedBody, wrongBody)
	}

	// The management list (authed) shows the live link.
	listResp := doGet(t, ts.URL+"/api/share-links")
	var listEnv envelope
	if err := json.NewDecoder(listResp.Body).Decode(&listEnv); err != nil {
		t.Fatalf("decode share-link list: %v", err)
	}
	listResp.Body.Close()
	var links []map[string]interface{}
	if err := json.Unmarshal(listEnv.Data, &links); err != nil {
		t.Fatalf("unmarshal share-link list: %v", err)
	}
	if len(links) != 1 || links[0]["token"] != token {
		t.Fatalf("share-link list = %v, want the one created link", links)
	}

	// Revoke: 204, and the audit trail records both lifecycle events.
	revResp := doDelete(t, fmt.Sprintf("%s/api/share-links/%d", ts.URL, int64(link["id"].(float64))))
	revResp.Body.Close()
	if revResp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: expected 204, got %d", revResp.StatusCode)
	}
	// Revocation is idempotent: a retry still succeeds.
	retryResp := doDelete(t, fmt.Sprintf("%s/api/share-links/%d", ts.URL, int64(link["id"].(float64))))
	retryResp.Body.Close()
	if retryResp.StatusCode != http.StatusNoContent {
		t.Errorf("re-revoke: expected 204, got %d", retryResp.StatusCode)
	}
	// An unknown link id is a plain 404.
	missingResp := doDelete(t, ts.URL+"/api/share-links/9999")
	missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Errorf("revoke unknown id: expected 404, got %d", missingResp.StatusCode)
	}

	// After revocation the token reads exactly like a wrong token.
	revokedResp := plainGet(t, ts.URL+"/api/shared-reports/"+token)
	revokedBody := readBody(t, revokedResp)
	if revokedResp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked token: expected 404, got %d", revokedResp.StatusCode)
	}
	if revokedBody != wrongBody {
		t.Errorf("revoked-token body %q differs from wrong-token body %q (enumeration oracle)", revokedBody, wrongBody)
	}

	// Audit: creation and revocation both recorded.
	for _, action := range []string{"share_link.create", "share_link.revoke"} {
		auditResp := doGet(t, ts.URL+"/api/audit-logs?action="+action)
		var auditEnv envelope
		if err := json.NewDecoder(auditResp.Body).Decode(&auditEnv); err != nil {
			t.Fatalf("decode audit logs: %v", err)
		}
		auditResp.Body.Close()
		var page map[string]interface{}
		if err := json.Unmarshal(auditEnv.Data, &page); err != nil {
			t.Fatalf("unmarshal audit page: %v", err)
		}
		if total := page["total"].(float64); total < 1 {
			t.Errorf("audit action %q: total = %v, want at least one entry", action, total)
		}
	}
}

// TestShareLinkCreationValidation covers create-side validation: unknown
// campaigns 404, and every minted token is unique.
func TestShareLinkCreationValidation(t *testing.T) {
	ts, campaignID := setupDoneCampaign(t)

	resp := doPost(t, ts.URL+"/api/campaigns/9999/share-links", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("create for unknown campaign: expected 404, got %d", resp.StatusCode)
	}

	first := createShareLink(t, ts.URL, campaignID)
	second := createShareLink(t, ts.URL, campaignID)
	if first["token"] == second["token"] {
		t.Error("two share links minted the same token")
	}
}

// TestSharedReportRateLimited pins the public-read rate tier on the shared
// report endpoint (ticket 33: the public GET tier applies).
func TestSharedReportRateLimited(t *testing.T) {
	db := openTempDB(t)
	ts := httptest.NewServer(server.New(db, testAdminPassword,
		server.WithRateLimits(server.RateLimits{
			Read: server.RateTier{PerMinute: 2, Burst: 2},
		}),
	))
	t.Cleanup(ts.Close)

	url := ts.URL + "/api/shared-reports/" + "a1b2c3d4e5f60000000000000000000000000000000000000000000000000000"
	for i := 1; i <= 2; i++ {
		resp := plainGet(t, url)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("read %d: unexpectedly rate limited", i)
		}
	}
	resp := plainGet(t, url)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("read 3: expected 429, got %d", resp.StatusCode)
	}
}

// TestSharedReportHidesUnfinishedBoard pins the public boundary of an
// unsettled campaign's report (ticket 54, revising the HIGH-1 caliber of
// ticket 52): the two data classes part ways at the session edge. Progress
// metadata — the model x suite four-state cells with judged/expected
// coverage — is operational fact and crosses to the shared view, so a
// running campaign answers its stripped progress board (lexicographic rows,
// samples withheld). Half-baked scores, rankings and deltas are evaluation
// conclusions and never leave the session boundary pre-settle: every score
// field stays null/empty even though the session-gated report serves the
// live half-scored board for the same campaign. Retired models stay off the
// shared board (the store-layer member/progress filters are inherited). Once
// the batch settles, the shared view renders the full ranked board like the
// session view.
//
// Scenario mirrors TestCampaignReportProgressGrid: the judge model is frozen
// after three judge calls, catching the sweep mid-cap_language with results
// already recorded; gamma is a discovered model retired mid-flight.
func TestSharedReportHidesUnfinishedBoard(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	createEvalModel(t, ts.URL, stub.URL, "alpha-model")
	createEvalModel(t, ts.URL, stub.URL, "beta-model")
	stub.markBroken("alpha-model", true)

	// Gamma joins the sweep as a discovered model and retires mid-flight:
	// the shared board must drop it (store-layer filter inheritance).
	discovery := newDiscoveryStubHub(t, []string{"gamma-model"})
	createHubViaAPI(t, ts.URL, discovery.URL)
	runDiscovery(t, ts.URL)

	stub.resetCalls()
	stub.blockModelAfter(store.DefaultJudgeModel, 3)
	t.Cleanup(func() { stub.releaseModel(store.DefaultJudgeModel) })

	campaign := triggerFullSweep(t, ts.URL)
	campaignID := int64(campaign["id"].(float64))
	// The fourth judge call being recorded proves the freeze point: the
	// campaign is running with results already on record.
	waitFor(t, "fourth judge call reaching the stub", func() bool {
		return stub.callTotal(store.DefaultJudgeModel) >= 4
	})

	link := createShareLink(t, ts.URL, campaignID)
	token, _ := link["token"].(string)

	// Mid-flight: the session view serves the live half-scored board…
	authed := getCampaignReport(t, ts.URL, campaignID, "")
	if authed["status"] != store.CampaignStatusRunning {
		t.Fatalf("campaign status = %v, want running", authed["status"])
	}
	if rows := reportRows(t, authed); len(rows) == 0 {
		t.Fatalf("session report of a running campaign must serve the live board, got %v", rows)
	}

	// …and gamma's retirement must not disturb the in-flight sweep: the hub
	// stops listing it (an empty list would be treated as a fetch anomaly
	// and retire nothing).
	discovery.setModels([]string{"replacement-model"})
	runDiscovery(t, ts.URL)

	// The shared view of the same campaign answers the stripped progress
	// board: rows in model-id lexicographic order with the retired model
	// absent, progress cells attached, and every evaluation-conclusion
	// field empty.
	shared := getSharedReport(t, ts.URL, token)
	if shared["status"] != store.CampaignStatusRunning {
		t.Fatalf("shared campaign status = %v, want running", shared["status"])
	}
	if shared["baseline"] != nil {
		t.Errorf("shared running report baseline = %v, want null", shared["baseline"])
	}
	rows := reportRows(t, shared)
	if len(rows) != 2 {
		t.Fatalf("shared running rows = %v, want alpha and beta only (retired gamma hidden)", rows)
	}
	for i, want := range []string{"alpha-model", "beta-model"} {
		if rows[i]["model_id"] != want {
			t.Errorf("shared row %d = %v, want %s (lexicographic, retired model hidden)", i, rows[i]["model_id"], want)
		}
	}
	for _, row := range rows {
		if row["total_score"] != nil {
			t.Errorf("shared model %v total_score = %v, want null pre-settle", row["model_id"], row["total_score"])
		}
		if row["total_delta"] != nil {
			t.Errorf("shared model %v total_delta = %v, want null pre-settle", row["model_id"], row["total_delta"])
		}
		if scores, _ := row["suite_scores"].(map[string]interface{}); len(scores) != 0 {
			t.Errorf("shared model %v suite_scores = %v, want empty pre-settle", row["model_id"], scores)
		}
		cells := reportCells(t, row)
		if len(cells) != suiteCount(t, ts.URL) {
			t.Fatalf("shared model %v has %d cells, want one per suite", row["model_id"], len(cells))
		}
		for _, cell := range cells {
			if got := int(cell["samples"].(float64)); got != 0 {
				t.Errorf("shared model %v suite %v samples = %d, want 0 (confidence marker withheld)",
					row["model_id"], cell["suite_key"], got)
			}
		}
	}

	// Cell states and coverage mirror the session grid's caliber: the four
	// rule-only suites are done, cap_language is mid-flight (alpha's broken
	// results fully recorded, beta seven of ten judged).
	alpha, beta := rows[0], rows[1]
	for _, key := range []string{"cap_instruction", "cap_reasoning", "cap_coding", "cap_knowledge"} {
		assertCell(t, alpha, key, "done", 0, 10)
		assertCell(t, beta, key, "done", 10, 10)
	}
	assertCell(t, alpha, "cap_language", "done", 0, 10)
	assertCell(t, beta, "cap_language", "running", 7, 10)

	// Settled: the shared view renders the same full board as the session
	// view, cells included.
	stub.releaseModel(store.DefaultJudgeModel)
	waitCampaignStatus(t, ts.URL, campaignID, store.CampaignStatusDone)

	sharedRows := reportRows(t, getSharedReport(t, ts.URL, token))
	authedRows := reportRows(t, getCampaignReport(t, ts.URL, campaignID, ""))
	if len(sharedRows) == 0 || len(sharedRows) != len(authedRows) {
		t.Fatalf("settled shared rows = %v, want the full board (%d rows)", sharedRows, len(authedRows))
	}
	for i, row := range sharedRows {
		if row["model_id"] != authedRows[i]["model_id"] || row["total_score"] != authedRows[i]["total_score"] {
			t.Errorf("settled shared row %d = %v, want %v", i, row, authedRows[i])
		}
		if cells := reportCells(t, row); len(cells) == 0 {
			t.Errorf("settled shared row %v must carry progress cells", row["model_id"])
		}
	}
}
