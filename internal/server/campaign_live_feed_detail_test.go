package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// getLiveFeedResult fetches GET /api/campaigns/{id}/live-feed/{resultID},
// asserts 200 and returns the decoded detail.
func getLiveFeedResult(t *testing.T, client *http.Client, base string, campaignID, resultID int64) map[string]interface{} {
	t.Helper()
	resp := getResp(t, client, fmt.Sprintf("%s/api/campaigns/%d/live-feed/%d", base, campaignID, resultID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET live-feed result (campaign=%d, result=%d): expected 200, got %d: %s",
			campaignID, resultID, resp.StatusCode, b)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode live-feed result: %v", err)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("unmarshal live-feed result: %v", err)
	}
	return detail
}

// findCaseByVerdict locates a seeded case of the given verdict type whose
// expectation field (rule_expected for rule, rubric for judge) is set.
func findCaseByVerdict(t *testing.T, db *store.DB, verdictType string) (store.Suite, store.Case) {
	t.Helper()
	suites, err := db.ListSuites()
	if err != nil {
		t.Fatalf("list suites: %v", err)
	}
	for _, suite := range suites {
		cases, err := db.ListCases(suite.ID)
		if err != nil {
			t.Fatalf("list cases (%s): %v", suite.Key, err)
		}
		for _, c := range cases {
			if c.VerdictType != verdictType {
				continue
			}
			if verdictType == "rule" && c.RuleExpected != nil {
				return suite, c
			}
			if verdictType == "judge" && c.Rubric != nil {
				return suite, c
			}
		}
	}
	t.Fatalf("no seeded %s case with an expectation field", verdictType)
	return store.Suite{}, store.Case{}
}

// TestCampaignLiveFeedResultDetail pins the GH #41 expansion contract: the
// console-only detail endpoint serves the case prompt, the verdict-method
// expectation fork (rule -> rule_expected, judge -> rubric), the model's
// answer text, the score and the verdict detail for one result of the
// campaign; results of another campaign and unknown ids answer 404.
func TestCampaignLiveFeedResultDetail(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	ruleSuite, ruleCase := findCaseByVerdict(t, db, "rule")
	// The post-cutover bank seeds rule cases only (ticket 99), so the judge
	// leg of the fork installs its own case into the same suite.
	judgeSuite := ruleSuite
	judgeRubric := "评分要点:切题得 1 分,否则 0 分。"
	judgeCasePtr, err := db.CreateCase(store.Case{
		SuiteID:     judgeSuite.ID,
		Prompt:      "LIVEFEED-JUDGE:请作答",
		VerdictType: "judge",
		Rubric:      &judgeRubric,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create judge case: %v", err)
	}
	judgeCase := *judgeCasePtr

	hub, err := db.CreateHub("detail-hub", "http://detail.test", "tok-detail-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	model, err := db.CreateModel(hub.ID, "detail-model", []string{"openai"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	campaign, err := db.CreateCampaign("manual", []int64{model.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	ruleRun, err := db.CreateEvalRun(campaign.ID, ruleSuite.ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create rule run: %v", err)
	}
	judgeRun, err := db.CreateEvalRun(campaign.ID, judgeSuite.ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create judge run: %v", err)
	}
	answer, detailText := "模型作答原文", "裁判判定明细"
	one := 1.0
	ruleResult, err := db.CreateEvalResult(store.EvalResult{
		EvalRunID: ruleRun.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: ruleCase.ID,
		AnswerText: &answer, Score: &one, VerdictDetail: &detailText, LatencyMs: 120,
	})
	if err != nil {
		t.Fatalf("create rule result: %v", err)
	}
	judgeResult, err := db.CreateEvalResult(store.EvalResult{
		EvalRunID: judgeRun.ID, ModelDBID: model.ID, ModelID: model.ModelID, CaseID: judgeCase.ID,
		AnswerText: &answer, Score: &one, VerdictDetail: &detailText, LatencyMs: 240,
	})
	if err != nil {
		t.Fatalf("create judge result: %v", err)
	}

	client := authedClient(t, ts.URL)

	rule := getLiveFeedResult(t, client, ts.URL, campaign.ID, ruleResult.ID)
	if rule["case_prompt"] != ruleCase.Prompt {
		t.Errorf("rule detail case_prompt = %v, want the case prompt", rule["case_prompt"])
	}
	if rule["verdict_type"] != "rule" {
		t.Errorf("rule detail verdict_type = %v, want rule", rule["verdict_type"])
	}
	if rule["expected"] != *ruleCase.RuleExpected {
		t.Errorf("rule detail expected = %v, want the rule_expected standard answer", rule["expected"])
	}
	if rule["answer_text"] != answer {
		t.Errorf("rule detail answer_text = %v, want the stored answer", rule["answer_text"])
	}
	if rule["verdict_detail"] != detailText {
		t.Errorf("rule detail verdict_detail = %v, want the stored detail", rule["verdict_detail"])
	}
	if rule["score"] != 1.0 {
		t.Errorf("rule detail score = %v, want 1.0 (raw 0~1 scale)", rule["score"])
	}

	judge := getLiveFeedResult(t, client, ts.URL, campaign.ID, judgeResult.ID)
	if judge["verdict_type"] != "judge" {
		t.Errorf("judge detail verdict_type = %v, want judge", judge["verdict_type"])
	}
	if judge["expected"] != *judgeCase.Rubric {
		t.Errorf("judge detail expected = %v, want the rubric scoring points", judge["expected"])
	}

	// A result of another campaign and an unknown id both answer 404.
	otherCampaign, err := db.CreateCampaign("manual", []int64{model.ID}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create other campaign: %v", err)
	}
	for _, tc := range []struct {
		campaignID, resultID int64
	}{
		{otherCampaign.ID, ruleResult.ID}, // result exists but not in this campaign
		{campaign.ID, 999999},             // unknown result
	} {
		resp := getResp(t, client, fmt.Sprintf("%s/api/campaigns/%d/live-feed/%d", ts.URL, tc.campaignID, tc.resultID))
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("campaign=%d result=%d: expected 404, got %d: %s", tc.campaignID, tc.resultID, resp.StatusCode, body)
		}
	}
}

// TestCampaignLiveFeedResultHubIsolation pins the W6 boundary of the detail
// endpoint (same caliber as the feed list): anonymous 401, a hub-scoped
// session sees its own hub's campaign details, another hub's campaign
// answers the same 404 as an unknown one (no enumeration oracle).
func TestCampaignLiveFeedResultHubIsolation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	ruleSuite, ruleCase := findCaseByVerdict(t, db, "rule")

	seed := func(hubName, modelID string) (campaignID, resultID, hubID int64) {
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
		run, err := db.CreateEvalRun(c.ID, ruleSuite.ID, "manual", "judge-x")
		if err != nil {
			t.Fatalf("create run (%s): %v", hubName, err)
		}
		score := 0.8
		res, err := db.CreateEvalResult(store.EvalResult{
			EvalRunID: run.ID, ModelDBID: m.ID, ModelID: modelID, CaseID: ruleCase.ID, Score: &score,
		})
		if err != nil {
			t.Fatalf("create result (%s): %v", hubName, err)
		}
		return c.ID, res.ID, h.ID
	}
	campaignA, resultA, hubAID := seed("det-iso-a", "det-model-a")
	campaignB, resultB, _ := seed("det-iso-b", "det-model-b")

	seedUserWithRole(t, db, "det-a-admin", store.RoleAdmin, &hubAID)
	aClient := loginAsClient(t, ts.URL, "det-a-admin")

	// Own hub: served.
	detail := getLiveFeedResult(t, aClient, ts.URL, campaignA, resultA)
	if detail["id"] != float64(resultA) {
		t.Errorf("hub-A admin own detail id = %v, want %d", detail["id"], resultA)
	}

	// Cross-hub campaign: same 404 as an unknown campaign — no oracle.
	for _, id := range []int64{campaignB, 99999} {
		resp := getResp(t, aClient, fmt.Sprintf("%s/api/campaigns/%d/live-feed/%d", ts.URL, id, resultB))
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("hub-A admin campaign %d detail: expected 404, got %d: %s", id, resp.StatusCode, body)
		}
	}

	// Anonymous: 401 (session-gated; never in publicReadPattern).
	anonClient := &http.Client{}
	resp := getResp(t, anonClient, fmt.Sprintf("%s/api/campaigns/%d/live-feed/%d", ts.URL, campaignA, resultA))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous detail: expected 401, got %d", resp.StatusCode)
	}
}
