package server_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// TestCampaignReportLiveRanksByPartialTotal pins the GH #40 live-board
// ranking caliber: an unfinished campaign's rows are ordered by the
// half-scored total descending — the ADR-0005 weighted mean over the
// already-judged suites (unscored suites drop out of numerator and
// denominator alike, never "null counts as zero") — with model-id
// lexicographic tie-break, and rows without any judged suite (null total)
// sunk below every scored row (lexicographic among themselves). The settled
// board's ranking is untouched (covered by the other report tests).
func TestCampaignReportLiveRanksByPartialTotal(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hub, err := db.CreateHub("live-rank-hub", "http://live-rank.test", "tok-live-rank-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	// Model ids are chosen so the lexicographic order (aaa/mmm/zzz style)
	// disagrees with the score order on purpose: the pre-GH-#40 board was
	// lexicographic, so a pass here proves the new ranking.
	modelIDs := map[string]int64{}
	for _, name := range []string{"zzz-top", "ccc-mid", "bbb-mid", "mmm-none"} {
		m, err := db.CreateModel(hub.ID, name, []string{"openai"})
		if err != nil {
			t.Fatalf("create model %s: %v", name, err)
		}
		modelIDs[name] = m.ID
	}
	suites, err := db.ListSuites()
	if err != nil || len(suites) == 0 {
		t.Fatalf("list suites: %v (n=%d)", err, len(suites))
	}
	// A nadir-0 suite keeps the staged raw scores on the 0-100 scale the
	// assertions use (nadir-bearing suites normalize, ADR 0009 — mmlu's 0.5
	// would surface as 33.3, not 50).
	var suite store.Suite
	for _, s := range suites {
		if s.Nadir == 0 {
			suite = s
			break
		}
	}
	if suite.ID == 0 {
		t.Fatalf("no nadir-0 suite in the rotation: %v", suites)
	}
	cases, err := db.ListCases(suite.ID)
	if err != nil || len(cases) == 0 {
		t.Fatalf("list cases: %v (n=%d)", err, len(cases))
	}
	caseRow := cases[0]

	campaign, err := db.CreateCampaign("manual", []int64{
		modelIDs["zzz-top"], modelIDs["ccc-mid"], modelIDs["bbb-mid"], modelIDs["mmm-none"],
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	run, err := db.CreateEvalRun(campaign.ID, suite.ID, "manual", "judge-x")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	full, half := 1.0, 0.5
	mkResult := func(model string, score *float64) {
		t.Helper()
		if _, err := db.CreateEvalResult(store.EvalResult{
			EvalRunID: run.ID, ModelDBID: modelIDs[model], ModelID: model, CaseID: caseRow.ID, Score: score,
		}); err != nil {
			t.Fatalf("create result (%s): %v", model, err)
		}
	}
	mkResult("zzz-top", &full)
	mkResult("ccc-mid", &half)
	mkResult("bbb-mid", &half)
	// mmm-none records no result at all: a member whose total stays null.
	if err := db.FinishEvalRun(run.ID, "done", time.Now().UTC()); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	report := getCampaignReport(t, ts.URL, campaign.ID, "")
	if report["status"] != "running" {
		t.Fatalf("campaign status = %v, want running (live board)", report["status"])
	}
	rows := reportRows(t, report)
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row["model_id"].(string))
	}
	want := []string{"zzz-top", "bbb-mid", "ccc-mid", "mmm-none"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("live board order = %v, want %v (partial total desc, ties lexicographic, null total sunk)", got, want)
	}
	if rows[0]["total_score"] != 100.0 {
		t.Errorf("zzz-top total = %v, want 100 (0-100 scale)", rows[0]["total_score"])
	}
	if rows[1]["total_score"] != 50.0 || rows[2]["total_score"] != 50.0 {
		t.Errorf("mid totals = %v/%v, want 50/50", rows[1]["total_score"], rows[2]["total_score"])
	}
	if v, ok := rows[3]["total_score"]; !ok || v != nil {
		t.Errorf("mmm-none total = %v (present=%v), want explicit null", v, ok)
	}
}
