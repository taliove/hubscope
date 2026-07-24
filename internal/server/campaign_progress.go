package server

import (
	"sort"

	"github.com/taliove/hubscope/internal/store"
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
// (ticket 52, spec 0004; membership per ticket 53): every snapshotted member
// gets a row from the first run on — a member the sweep has not reached yet
// shows pending cells instead of staying invisible until its first result
// lands. The member list is unioned with the models that recorded results,
// so a campaign whose snapshot missed someone (only possible for
// pre-membership campaigns, backfilled from results) never hides a model
// that has results. Scores come from done runs only, and rows stay in
// model-id lexicographic order — no ranking information leaks before the
// batch settles.
func liveReportRows(members []store.CampaignMember, progress []store.CampaignCellProgress, scores []store.CampaignSuiteScore, weights map[string]float64, nadirs map[string]float64) []reportRowDTO {
	scoredByModel := map[int64]reportRowDTO{}
	for _, row := range buildReportRows(scores, weights, nadirs) {
		scoredByModel[row.ModelDBID] = row
	}
	rows := []reportRowDTO{}
	seen := map[int64]bool{}
	appendRow := func(modelDBID int64, modelID, family string) {
		if seen[modelDBID] {
			return
		}
		seen[modelDBID] = true
		if row, ok := scoredByModel[modelDBID]; ok {
			rows = append(rows, row)
			return
		}
		rows = append(rows, reportRowDTO{
			ModelDBID:   modelDBID,
			ModelID:     modelID,
			Family:      family,
			SuiteScores: map[string]*float64{},
		})
	}
	for _, m := range members {
		appendRow(m.ModelDBID, m.ModelID, m.Family)
	}
	for _, p := range progress {
		appendRow(p.ModelDBID, p.ModelID, p.Family)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ModelID < rows[j].ModelID })
	return rows
}

// cellProgressKey indexes one model's progress inside one run.
type cellProgressKey struct {
	runID     int64
	modelDBID int64
}

// cellIndex groups runs by suite and progress rows by (run, model) so grid
// cells can be built without re-scanning either slice.
type cellIndex struct {
	runBySuite     map[int64]store.EvalRun
	progressByCell map[cellProgressKey]store.CampaignCellProgress
}

func newCellIndex(runs []store.EvalRun, progress []store.CampaignCellProgress) cellIndex {
	idx := cellIndex{
		runBySuite:     make(map[int64]store.EvalRun, len(runs)),
		progressByCell: make(map[cellProgressKey]store.CampaignCellProgress, len(progress)),
	}
	for _, run := range runs {
		idx.runBySuite[run.SuiteID] = run
	}
	for _, p := range progress {
		idx.progressByCell[cellProgressKey{p.RunID, p.ModelDBID}] = p
	}
	return idx
}

// cellFor builds one model x suite progress cell; ok=false means the campaign
// has no run for the suite (no cell rendered). withSamples=false keeps the
// confidence marker at zero — required at the shared public boundary (ticket
// 54), where samples are score-side metadata and never cross pre-settle.
func (idx cellIndex) cellFor(suite reportSuiteDTO, modelDBID int64, planned int, withSamples bool) (cell reportCellDTO, ok bool) {
	run, ok := idx.runBySuite[suite.ID]
	if !ok {
		return reportCellDTO{}, false
	}
	p, has := idx.progressByCell[cellProgressKey{run.ID, modelDBID}]
	cell = reportCellDTO{
		SuiteKey:      suite.Key,
		Status:        cellStatus(run.Status, has, p, planned),
		JudgedCases:   p.JudgedCases,
		ExpectedCases: cellExpected(run.Status, has, p, planned),
	}
	if withSamples {
		cell.Samples = p.Samples
	}
	return cell, true
}

// sharedProgressRows builds the progress-only board the public shared view
// serves for an unfinished campaign (ticket 54, spec 0004 shared boundary —
// the revised HIGH-1 caliber): progress metadata — the model x suite
// four-state cells with judged/expected coverage — may cross the session
// boundary, while half-baked scores, ranking information and the samples
// confidence marker never leave before the batch settles. The constructor
// is structurally unable to leak them: scores, weights and nadirs are not
// among its inputs, so no "build the live board then blank the score
// fields" mishap is possible. Rows are the member snapshot unioned with the
// models that recorded results (read-time filtered: deleted/retired models
// never appear), in model-id lexicographic order; family/sort query params
// are ignored. Every row's score fields stay empty (null total/delta, empty
// suite scores, no baseline) and every cell's Samples stays zero.
func sharedProgressRows(members []store.CampaignMember, suites []reportSuiteDTO, runs []store.EvalRun, progress []store.CampaignCellProgress, expectedBySuite map[int64]int) []reportRowDTO {
	idx := newCellIndex(runs, progress)

	type modelRef struct {
		dbID    int64
		modelID string
		family  string
	}
	ordered := []modelRef{}
	seen := map[int64]bool{}
	appendRef := func(dbID int64, modelID, family string) {
		if seen[dbID] {
			return
		}
		seen[dbID] = true
		ordered = append(ordered, modelRef{dbID, modelID, family})
	}
	for _, m := range members {
		appendRef(m.ModelDBID, m.ModelID, m.Family)
	}
	for _, p := range progress {
		appendRef(p.ModelDBID, p.ModelID, p.Family)
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].modelID < ordered[j].modelID })

	rows := make([]reportRowDTO, 0, len(ordered))
	for _, ref := range ordered {
		cells := make([]reportCellDTO, 0, len(suites))
		for _, suite := range suites {
			// Samples deliberately excluded: the confidence marker is
			// score-side metadata and never crosses the shared boundary
			// pre-settle.
			if cell, ok := idx.cellFor(suite, ref.dbID, expectedBySuite[suite.ID], false); ok {
				cells = append(cells, cell)
			}
		}
		rows = append(rows, reportRowDTO{
			ModelDBID:   ref.dbID,
			ModelID:     ref.modelID,
			Family:      ref.family,
			SuiteScores: map[string]*float64{},
			Cells:       cells,
		})
	}
	return rows
}

// attachReportCells returns fresh rows with the per-suite progress cells
// filled in (ticket 52); inputs stay untouched. Every row gets one cell per
// campaign suite, so the grid always renders the full matrix.
func attachReportCells(rows []reportRowDTO, suites []reportSuiteDTO, runs []store.EvalRun, progress []store.CampaignCellProgress, expectedBySuite map[int64]int) []reportRowDTO {
	idx := newCellIndex(runs, progress)

	out := make([]reportRowDTO, len(rows))
	for i, row := range rows {
		cells := make([]reportCellDTO, 0, len(suites))
		for _, suite := range suites {
			if cell, ok := idx.cellFor(suite, row.ModelDBID, expectedBySuite[suite.ID], true); ok {
				cells = append(cells, cell)
			}
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
