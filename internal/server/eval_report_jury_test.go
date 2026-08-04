package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestRunDetailJuryBreakdown pins the GH #178 jury surface of the run
// detail API: judge cases carry every jury vote (sample, slot, judge,
// score) plus the spread, while rule cases carry none.
func TestRunDetailJuryBreakdown(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	subjectID, caseID := setupJuryCase(t, ts.URL, stub.URL, db)
	stub.setJudgeSeqFor("judge-1", `{"score": 0.4, "reason": "low"}`)
	stub.setJudgeSeqFor("judge-2", `{"score": 0.6, "reason": "mid"}`)
	stub.setJudgeSeqFor("judge-3", `{"score": 0.9, "reason": "high"}`)

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), subjectID)
	run := waitEvalDone(t, ts.URL, runID)

	result := resultByCaseID(t, run, "subject", caseID)
	votes, _ := result["judge_scores"].([]interface{})
	if len(votes) != 3 {
		t.Fatalf("judge_scores = %v, want 3 votes", votes)
	}
	byJudge := map[string]float64{}
	for _, v := range votes {
		vm := v.(map[string]interface{})
		byJudge[vm["judge_model"].(string)] = vm["score"].(float64)
	}
	if byJudge["judge-1"] != 0.4 || byJudge["judge-2"] != 0.6 || byJudge["judge-3"] != 0.9 {
		t.Errorf("votes = %v, want judge-1=0.4 judge-2=0.6 judge-3=0.9", byJudge)
	}
	if spread, ok := result["spread"].(float64); !ok || spread < 0.49 || spread > 0.51 {
		t.Errorf("spread = %v, want 0.5 (0.9-0.4)", result["spread"])
	}

	// Rule cases carry no jury breakdown.
	for _, r := range run["results"].([]interface{}) {
		rm := r.(map[string]interface{})
		if rm["case_id"].(float64) == float64(caseID) {
			continue
		}
		if _, has := rm["judge_scores"]; has {
			t.Errorf("rule case %v must not carry judge_scores", rm["case_id"])
		}
	}

	// The jury tally rides the run detail: every judge with its vote count
	// and priced cost (GH #179).
	summary, _ := run["jury_summary"].([]interface{})
	if len(summary) != 3 {
		t.Fatalf("jury_summary = %v, want 3 judges", summary)
	}
	for _, j := range summary {
		jm := j.(map[string]interface{})
		if votes, _ := jm["votes"].(float64); votes != 1 {
			t.Errorf("judge %v votes = %v, want 1", jm["judge_model"], votes)
		}
		// This run registered IQ only: the judge's price is unregistered,
		// so its cost reads null (never zero).
		if cost, ok := jm["cost"].(float64); ok && cost == 0 {
			t.Errorf("judge %v cost = 0, want null when the price is unregistered", jm["judge_model"])
		}
	}
}

// TestRunEstimatedCost pins the GH #178 cost accounting: priced components
// accumulate into the run's exam/judge split, and an unpriced component
// turns the whole estimate null ("price not registered", never a partial
// sum presented as complete).
func TestRunEstimatedCost(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	subjectID, _ := setupJuryCase(t, ts.URL, stub.URL, db)
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"model_registry_overrides": []map[string]interface{}{
			{"match": "subject", "iq_tier": 10, "price_in": 1000, "price_out": 1000},
			{"match": "judge-1", "iq_tier": 9, "price_in": 1000, "price_out": 1000},
			{"match": "judge-2", "iq_tier": 8, "price_in": 1000, "price_out": 1000},
			{"match": "judge-3", "iq_tier": 7, "price_in": 1000, "price_out": 1000},
		},
	})
	putResp.Body.Close()

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), subjectID)
	run := waitEvalDone(t, ts.URL, runID)

	cost, _ := run["estimated_cost"].(map[string]interface{})
	if cost == nil {
		t.Fatal("estimated_cost must be set when every component is priced")
	}
	exam, _ := cost["exam"].(float64)
	judge, _ := cost["judge"].(float64)
	if exam <= 0 || judge <= 0 {
		t.Errorf("cost split = exam %v judge %v, want both > 0", exam, judge)
	}

	// Second run with the subject's price unregistered: the estimate goes
	// null even though the judges are priced.
	putResp = doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"model_registry_overrides": []map[string]interface{}{
			{"match": "judge-1", "iq_tier": 9, "price_in": 1000, "price_out": 1000},
			{"match": "judge-2", "iq_tier": 8, "price_in": 1000, "price_out": 1000},
			{"match": "judge-3", "iq_tier": 7, "price_in": 1000, "price_out": 1000},
		},
	})
	putResp.Body.Close()
	runID2 := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), subjectID)
	run2 := waitEvalDone(t, ts.URL, runID2)
	if run2["estimated_cost"] != nil {
		t.Errorf("estimated_cost = %v, want null (subject price not registered)", run2["estimated_cost"])
	}
}

// TestCampaignReportQueueDepth pins the GH #178 live surface: a running
// campaign exposes its two-stage queue state while the judges are blocked.
func TestCampaignReportQueueDepth(t *testing.T) {
	// Async eval: observes the queue depth with every judge frozen
	// mid-flight; drained by releaseModel + waitCampaignStatus(done).
	ts, stub, db := setupAsyncEvalEnv(t)
	subjectID, _ := setupJuryCase(t, ts.URL, stub.URL, db)
	stub.resetCalls()
	for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
		stub.blockModelAfter(j, 3)
	}
	defer func() {
		for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
			stub.releaseModel(j)
		}
	}()

	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")
	resp := doPost(t, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id": suiteID, "model_ids": []int64{subjectID},
	})
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var campaign map[string]interface{}
	_ = json.Unmarshal(env.Data, &campaign)
	campaignID := int64(campaign["id"].(float64))

	// All three judge calls blocked in flight, every answer done.
	waitFor(t, "judge calls blocked in flight", func() bool {
		return stub.callTotal("judge-1") >= 4 && stub.callTotal("judge-2") >= 4 && stub.callTotal("judge-3") >= 4
	})

	report := getCampaignReport(t, ts.URL, campaignID, "")
	depth, _ := report["queue_depth"].(map[string]interface{})
	if depth == nil {
		t.Fatal("running campaign must expose queue_depth")
	}
	if got := depth["judge_inflight"].(float64); got != 3 {
		t.Errorf("judge_inflight = %v, want 3 (every judge call blocked)", got)
	}
	if got := depth["exam_pending"].(float64); got != 0 {
		t.Errorf("exam_pending = %v, want 0 (answers completed while judging is blocked)", got)
	}

	for _, j := range []string{"judge-1", "judge-2", "judge-3"} {
		stub.releaseModel(j)
	}
	waitCampaignStatus(t, ts.URL, campaignID, "done")
	report = getCampaignReport(t, ts.URL, campaignID, "")
	if _, has := report["queue_depth"]; has {
		t.Error("settled campaign must not expose queue_depth")
	}
}

// TestCampaignEstimatedCostSummary pins the GH #178 batch-level split: the
// report sums every run's exam/judge estimate and prices each cost row's
// exam tokens through the registry.
func TestCampaignEstimatedCostSummary(t *testing.T) {
	ts, stub, db := setupEvalEnv(t)
	subjectID, _ := setupJuryCase(t, ts.URL, stub.URL, db)
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"model_registry_overrides": []map[string]interface{}{
			{"match": "subject", "iq_tier": 10, "price_in": 1000, "price_out": 1000},
			{"match": "judge-1", "iq_tier": 9, "price_in": 1000, "price_out": 1000},
			{"match": "judge-2", "iq_tier": 8, "price_in": 1000, "price_out": 1000},
			{"match": "judge-3", "iq_tier": 7, "price_in": 1000, "price_out": 1000},
		},
	})
	putResp.Body.Close()

	runID := triggerEval(t, ts.URL, suiteIDByKey(t, ts.URL, "gsm8k"), subjectID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))

	report := getCampaignReport(t, ts.URL, campaignID, "")
	estimated, _ := report["estimated_cost"].(map[string]interface{})
	if estimated == nil {
		t.Fatal("report must carry the estimated cost split")
	}
	if exam, _ := estimated["exam"].(float64); exam <= 0 {
		t.Errorf("estimated exam = %v, want > 0", exam)
	}
	if judge, _ := estimated["judge"].(float64); judge <= 0 {
		t.Errorf("estimated judge = %v, want > 0", judge)
	}
	if unknown, _ := estimated["unknown_runs"].(float64); unknown != 0 {
		t.Errorf("unknown_runs = %v, want 0 (every component priced)", unknown)
	}

	// The report carries the jury panel and probe outcomes (GH #179).
	jury, _ := report["jury"].(map[string]interface{})
	if jury == nil {
		t.Fatal("report must carry the jury panel")
	}
	if jury["policy"] != "iq" {
		t.Errorf("jury policy = %v, want iq", jury["policy"])
	}
	judges, _ := jury["judges"].([]interface{})
	if len(judges) != 3 {
		t.Errorf("jury judges = %v, want 3", judges)
	}
	probe, _ := jury["probe"].(map[string]interface{})
	subjProbe, _ := probe["subject"].(map[string]interface{})
	if subjProbe["ok"] != true || subjProbe["succ"] != 3.0 {
		t.Errorf("subject probe = %v, want ok with 3 successful rounds", subjProbe)
	}

	rows, _ := report["cost_rows"].([]interface{})
	if len(rows) == 0 {
		t.Fatal("cost_rows must not be empty")
	}
	for _, r := range rows {
		rm := r.(map[string]interface{})
		if rm["model_id"] != "subject" {
			continue
		}
		if examCost, ok := rm["exam_cost"].(float64); !ok || examCost <= 0 {
			t.Errorf("subject exam_cost = %v, want a positive registry price", rm["exam_cost"])
		}
		return
	}
	t.Error("subject missing from cost_rows")
}

// TestJudgeConcurrencySetting pins the new pool-size setting's boundary:
// 0 and 17 are rejected, 1..16 round-trip.
func TestJudgeConcurrencySetting(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)
	for _, bad := range []int{0, 17} {
		resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{"judge_concurrency": bad})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("judge_concurrency %d: expected 400, got %d", bad, resp.StatusCode)
		}
	}
	resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{"judge_concurrency": 2})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("judge_concurrency 2: expected 200, got %d", resp.StatusCode)
	}
	getResp := doGet(t, ts.URL+"/api/settings")
	var env envelope
	_ = json.NewDecoder(getResp.Body).Decode(&env)
	getResp.Body.Close()
	var settings map[string]interface{}
	_ = json.Unmarshal(env.Data, &settings)
	if settings["judge_concurrency"] != 2.0 {
		t.Errorf("judge_concurrency read-back = %v, want 2", settings["judge_concurrency"])
	}
}
