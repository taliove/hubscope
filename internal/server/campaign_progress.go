package server

import (
	"sort"

	"github.com/taliove2009/hubscope/internal/store"
)

// reportCellDTO is one model x suite progress cell (ticket 52): the run's
// status from the model's perspective (pending until the run records the
// model's first result, running while results trickle in, done once the
// run — or the model's full case set — completed, failed when the run
// failed) plus the judged-case coverage. Expected cases is the run's actual
// case count for finished runs and the suite's current enabled count for
// in-flight runs (what the run is evaluating right now). Samples is the sum
// of the declared per-case sample counts over judged cases (default 1) —
// the second half of the score's confidence marker (ticket 51). It counts
// what the cases declare, not the judge calls that actually succeeded, so a
// case whose judge samples partially failed still contributes its full
// declared count and the marker can read high.
type reportCellDTO struct {
	SuiteKey      string `json:"suite_key"`
	Status        string `json:"status"`
	JudgedCases   int    `json:"judged_cases"`
	ExpectedCases int    `json:"expected_cases"`
	Samples       int    `json:"samples"`
}

// enabledCaseCounts resolves the current enabled-case count of each suite —
// the planned case total of an in-flight run. Cases are immutable (ADR
// 0007), so a bank edit retires and re-mints rather than rewriting; a
// historical run's actual case count comes from its results instead.
func (s *Server) enabledCaseCounts(suites []reportSuiteDTO) (map[int64]int, error) {
	counts := make(map[int64]int, len(suites))
	for _, suite := range suites {
		cases, err := s.db.ListEnabledCases(suite.ID)
		if err != nil {
			return nil, err
		}
		counts[suite.ID] = len(cases)
	}
	return counts, nil
}

// liveReportRows builds the half-finished board of an unfinished campaign
// (ticket 52, spec 0004): every model with recorded results gets a row,
// scores come from done runs only, and rows stay in model-id lexicographic
// order — no ranking information leaks before the batch settles. Models the
// sweep has not reached yet have no results and therefore no row (runs
// carry no model membership); they appear as soon as their first result
// lands.
func liveReportRows(progress []store.CampaignCellProgress, scores []store.CampaignSuiteScore, weights map[string]float64, nadirs map[string]float64) []reportRowDTO {
	scoredByModel := map[int64]reportRowDTO{}
	for _, row := range buildReportRows(scores, weights, nadirs) {
		scoredByModel[row.ModelDBID] = row
	}
	rows := []reportRowDTO{}
	seen := map[int64]bool{}
	for _, p := range progress {
		if seen[p.ModelDBID] {
			continue
		}
		seen[p.ModelDBID] = true
		if row, ok := scoredByModel[p.ModelDBID]; ok {
			rows = append(rows, row)
			continue
		}
		rows = append(rows, reportRowDTO{
			ModelDBID:   p.ModelDBID,
			ModelID:     p.ModelID,
			Family:      p.Family,
			SuiteScores: map[string]*float64{},
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ModelID < rows[j].ModelID })
	return rows
}

// attachReportCells returns fresh rows with the per-suite progress cells
// filled in (ticket 52); inputs stay untouched. Every row gets one cell per
// campaign suite, so the grid always renders the full matrix.
func attachReportCells(rows []reportRowDTO, suites []reportSuiteDTO, runs []store.EvalRun, progress []store.CampaignCellProgress, expectedBySuite map[int64]int) []reportRowDTO {
	runBySuite := make(map[int64]store.EvalRun, len(runs))
	for _, run := range runs {
		runBySuite[run.SuiteID] = run
	}
	type cellKey struct {
		runID     int64
		modelDBID int64
	}
	progressByCell := make(map[cellKey]store.CampaignCellProgress, len(progress))
	for _, p := range progress {
		progressByCell[cellKey{p.RunID, p.ModelDBID}] = p
	}

	out := make([]reportRowDTO, len(rows))
	for i, row := range rows {
		cells := make([]reportCellDTO, 0, len(suites))
		for _, suite := range suites {
			run, ok := runBySuite[suite.ID]
			if !ok {
				continue
			}
			p, has := progressByCell[cellKey{run.ID, row.ModelDBID}]
			cells = append(cells, reportCellDTO{
				SuiteKey:      suite.Key,
				Status:        cellStatus(run.Status, has, p, expectedBySuite[suite.ID]),
				JudgedCases:   p.JudgedCases,
				ExpectedCases: cellExpected(run.Status, has, p, expectedBySuite[suite.ID]),
				Samples:       p.Samples,
			})
		}
		row.Cells = cells
		out[i] = row
	}
	return out
}

// cellStatus derives one grid cell's four-state status (waiting / running /
// done / failed) from the run's status and the model's recorded results.
// Inside a running run the models execute sequentially: a model with no
// results yet is still waiting, one with its full case set recorded is
// effectively done, anything between is running.
func cellStatus(runStatus string, hasResults bool, p store.CampaignCellProgress, planned int) string {
	switch runStatus {
	case "done":
		return "done"
	case "failed":
		return "failed"
	}
	if !hasResults {
		return "pending"
	}
	if planned > 0 && p.TotalCases >= planned {
		return "done"
	}
	return "running"
}

// cellExpected resolves a cell's expected case count: a finished run's
// actual case count (its results are the historical truth, immune to later
// bank edits), otherwise the suite's current enabled count — exactly what
// the in-flight run is evaluating.
func cellExpected(runStatus string, hasResults bool, p store.CampaignCellProgress, planned int) int {
	if runStatus == "done" && hasResults {
		return p.TotalCases
	}
	if planned > 0 {
		return planned
	}
	return p.TotalCases
}
