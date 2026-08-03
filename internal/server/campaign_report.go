package server

import (
	"encoding/json"
	"net/http"

	"github.com/taliove/hubscope/internal/registry"
	"github.com/taliove/hubscope/internal/store"
)

// reportSuiteDTO is the suite metadata the leaderboard needs: identity plus
// the question-bank version the campaign's runs scored against.
type reportSuiteDTO struct {
	ID      int64  `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// reportBaselineDTO names the previous done campaign the leaderboard's
// deltas compare against, and whether that comparison is same-caliber
// (ADR 0007/0008): a question-bank version change, a scoring-caliber
// (verdict profile) change, or a suite the baseline never covered breaks
// comparability, and reason says which ("suite_changed", "profile_changed"
// or "suite_missing").
type reportBaselineDTO struct {
	CampaignID int64  `json:"campaign_id"`
	Comparable bool   `json:"comparable"`
	Reason     string `json:"reason,omitempty"`
}

// reportRowDTO is one leaderboard row: a live model with its per-suite
// scores (0-100, nadir-normalized per ADR 0009, null when unscored), the
// weighted total (ADR 0005), and the total's delta against the baseline
// campaign (null when there is no comparable baseline or no score on either
// side). Cells carry the per-suite progress detail (ticket 52) plus the
// coverage/sample confidence markers (ticket 51); on an unfinished campaign
// the row list is the live half-scored board — ranked by the half-scored
// total descending with null totals sunk (GH #40), rendered de-emphasized
// on the frontend so a partial total never reads as a final grade
// (spec 0004).
//
// On a settled campaign the coverage gate applies (spec 0014 decision A):
// Complete reports whether every covered suite produced a non-null score
// (at least one judged case each — W7's "judge failure is null, not zero"
// caliber is untouched); an incomplete row forfeits its total, its delta
// and its rank, carries MissingSuites, and sinks below every complete row.
// Both fields are absent on the live board: the gate never touches an
// unfinished campaign.
type reportRowDTO struct {
	ModelDBID     int64               `json:"model_db_id"`
	ModelID       string              `json:"model_id"`
	Family        string              `json:"family"`
	TotalScore    *float64            `json:"total_score"`
	TotalDelta    *float64            `json:"total_delta"`
	SuiteScores   map[string]*float64 `json:"suite_scores"`
	Cells         []reportCellDTO     `json:"cells"`
	Complete      *bool               `json:"complete,omitempty"`
	MissingSuites int                 `json:"missing_suites,omitempty"`
}

// campaignCostDTO is the batch-level cost summary (GH #42, console-only):
// Σ latency / Σ input / Σ output tokens over the campaign's results, null
// tokens counted as 0.
type campaignCostDTO struct {
	LatencyMs    int64 `json:"latency_ms"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// costRowDTO is one line of the report page's cost detail table (GH #42,
// console-only): one model's cost inside one run. Token fields are null
// when the model recorded no token at all in that run (the table renders a
// dash), while the batch and cell sums count those same nulls as 0.
type costRowDTO struct {
	ModelID      string `json:"model_id"`
	SuiteKey     string `json:"suite_key"`
	SuiteName    string `json:"suite_name"`
	Status       string `json:"status"`
	LatencyMs    int64  `json:"latency_ms"`
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	// AvgTPS is the model's mean output speed in the run (GH #178); null
	// when it produced no answer.
	AvgTPS *float64 `json:"avg_tps"`
	// ExamCost prices the row's answer tokens against the registry
	// (GH #178); null when the model's price is not registered.
	ExamCost *float64 `json:"exam_cost"`
}

// queueDepthDTO is the live queue state of a running campaign's pipeline
// (GH #178, console-only): exam = answer-call cells, judge = jury votes.
type queueDepthDTO struct {
	ExamPending   int `json:"exam_pending"`
	ExamInflight  int `json:"exam_inflight"`
	JudgePending  int `json:"judge_pending"`
	JudgeInflight int `json:"judge_inflight"`
}

// estimatedCostDTO is the campaign-level estimated cost split (GH #178,
// console-only): the sum of every run's exam/judge portions, with the
// count of runs whose estimate is null because some component's price is
// not registered.
type estimatedCostDTO struct {
	Exam        float64 `json:"exam"`
	Judge       float64 `json:"judge"`
	UnknownRuns int     `json:"unknown_runs"`
}

// campaignReportDTO is GET /api/campaigns/{id}/report: the campaign, the
// suites it covers, the effective weights used for the totals, the
// leaderboard rows, and the previous-batch baseline the rows' deltas
// compare against (null when no earlier done campaign exists).
// FailedResults counts the batch's null-score (failed) result rows (GH #28):
// on a settled campaign, failed_results > 0 is the retry-failed button's
// render condition. Cost and CostRows carry the GH #42 cost metrics; both
// are session-only and stay absent on the shared/public surfaces
// (operational data, outside the ticket-54 public caliber).
type campaignReportDTO struct {
	campaignDTO
	Suites        []reportSuiteDTO   `json:"suites"`
	Weights       map[string]float64 `json:"weights"`
	Rows          []reportRowDTO     `json:"rows"`
	Baseline      *reportBaselineDTO `json:"baseline"`
	FailedResults int                `json:"failed_results"`
	Cost          *campaignCostDTO   `json:"cost,omitempty"`
	CostRows      []costRowDTO       `json:"cost_rows,omitempty"`
	// EstimatedCost is the campaign's registry-priced cost split (GH #178);
	// session-only like the other cost fields.
	EstimatedCost *estimatedCostDTO `json:"estimated_cost,omitempty"`
	// QueueDepth is the live two-stage queue state (GH #178), present only
	// while the campaign is executing on this process.
	QueueDepth *queueDepthDTO `json:"queue_depth,omitempty"`
}

// handleGetCampaignReport handles GET /api/campaigns/{id}/report. Only done
// runs contribute scores; deleted models (ticket 26 semantics) never rank.
// Settled campaigns serve the ranked board; unfinished campaigns serve the
// live half-scored board (ranked by partial total descending, GH #40) plus
// the per-(model, suite) progress cells (ticket 52). Query params: family
// filters rows to one model family; sort picks the ranking column of a
// settled board ("total" by default, or a covered suite key), always
// descending with unscored models last.
func (s *Server) handleGetCampaignReport(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}

	s.writeCampaignReport(w, r, campaign, false, true)
}

// reportBuildError carries an HTTP failure out of buildCampaignReport so
// callers (the session report, the shared report and the public eval
// board) can each decide how to write it.
type reportBuildError struct {
	status int
	msg    string
}

// writeCampaignReport builds and writes the report payload for one campaign.
// It is shared by the session-gated report endpoint and the token-gated
// shared-report endpoint (ADR 0006), so both views render the same settled
// board. The views diverge on an unfinished campaign, and the divergence
// follows two distinct data classes (ticket 54, revising the HIGH-1
// caliber): progress metadata — run status and judged-case coverage — is
// operational fact and may cross the session boundary, so the shared view
// serves the stripped progress board (sharedProgressRows); half-baked
// scores, rankings, deltas and the samples confidence marker are evaluation
// conclusions and never leave the session boundary before the batch
// settles, so the live half-scored board (ticket 52) stays session-only.
// withCost attaches the GH #42 cost metrics (batch totals, per-run detail
// rows and per-cell sums); it is true only for the session-gated endpoint —
// cost is operational data and never crosses to the shared or public board.
func (s *Server) writeCampaignReport(w http.ResponseWriter, r *http.Request, campaign *store.Campaign, shared, withCost bool) {
	report, berr := s.buildCampaignReport(r, campaign, shared, withCost)
	if berr != nil {
		writeError(w, berr.status, berr.msg)
		return
	}
	writeData(w, http.StatusOK, report)
}

// buildCampaignReport assembles the report payload for one campaign; the
// data-class split described on writeCampaignReport lives here. Query
// params: family filters rows to one model family; sort picks the ranking
// column of a settled board ("total" by default, or a covered suite key),
// always descending with unscored models last.
func (s *Server) buildCampaignReport(r *http.Request, campaign *store.Campaign, shared, withCost bool) (campaignReportDTO, *reportBuildError) {
	fail := func(status int, msg string) (campaignReportDTO, *reportBuildError) {
		return campaignReportDTO{}, &reportBuildError{status: status, msg: msg}
	}
	id := campaign.ID

	runs, err := s.db.ListEvalRunsByCampaign(id)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to load campaign runs")
	}

	suites, err := s.campaignSuites(runs)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to load suites")
	}

	sortKey := r.URL.Query().Get("sort")
	if sortKey == "" {
		sortKey = "total"
	}
	if sortKey != "total" && !hasSuiteKey(suites, sortKey) {
		return fail(http.StatusBadRequest, "sort must be \"total\" or a suite key of this campaign")
	}

	configured, err := s.db.GetSuiteWeights()
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to read suite weights")
	}
	weights := effectiveWeights(suites, configured)

	scores, err := s.db.ListCampaignSuiteScores(id)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to aggregate campaign scores")
	}

	// Nadir constants come from the runs' snapshots (ADR 0009), so every
	// score on this board is normalized with the constant its run locked in.
	nadirs := nadirBySuiteKey(suites, runs)

	cellProgress, err := s.db.ListCampaignCellProgress(id)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to load campaign progress")
	}

	expectedBySuite, err := s.enabledCaseCounts(suites)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to count suite cases")
	}

	family := r.URL.Query().Get("family")
	var rows []reportRowDTO
	var baseline *reportBaselineDTO
	if campaign.Status == "done" || campaign.Status == "failed" {
		rows = buildReportRows(scores, weights, nadirs)
		// The coverage gate (spec 0014 decision A) is a settled-batch
		// ranking rule: incomplete models forfeit total/delta/rank but keep
		// their real per-suite scores. It runs before the sort so the
		// ranking sinks them, and never touches the live paths below. Only
		// suites with enabled cases gate: an emptied suite assesses
		// nothing, so its unavoidable null is bank configuration, not a
		// judging gap, and must not disqualify every model.
		gateSuites := make([]reportSuiteDTO, 0, len(suites))
		for _, suite := range suites {
			if expectedBySuite[suite.ID] > 0 {
				gateSuites = append(gateSuites, suite)
			}
		}
		rows = applyCoverageGate(rows, gateSuites)
		if family != "" {
			rows = filterReportRowsByFamily(rows, family)
		}
		sortReportRows(rows, sortKey)

		// The previous-batch baseline only matters once rows are ranked.
		// Rows are re-created with deltas applied rather than mutated in
		// place (immutable data rule).
		var deltas map[int64]*float64
		baseline, deltas, err = s.reportBaseline(campaign.ID, suites, gateSuites, weights, rows)
		if err != nil {
			return fail(http.StatusInternalServerError, "failed to build report baseline")
		}
		if len(deltas) > 0 {
			withDeltas := make([]reportRowDTO, len(rows))
			for i, row := range rows {
				if delta, ok := deltas[row.ModelDBID]; ok {
					row.TotalDelta = delta
				}
				withDeltas[i] = row
			}
			rows = withDeltas
		}
		rows = attachReportCells(rows, suites, runs, cellProgress, expectedBySuite, withCost)
	} else if shared {
		// Public shared view of an unfinished campaign (ticket 54): progress
		// metadata only — the stripped progress board whose constructor
		// never sees scores/weights/nadirs; family/sort params are ignored
		// and the cells (already attached, samples withheld) stay as built.
		members, err := s.db.ListCampaignMembers(id)
		if err != nil {
			return fail(http.StatusInternalServerError, "failed to load campaign members")
		}
		rows = sharedProgressRows(members, suites, runs, cellProgress, expectedBySuite)
	} else {
		// Spec 0004 (revising spec 0002 review condition 3): an unfinished
		// campaign serves a live half-scored board — every snapshotted member
		// (ticket 53), ranked by partial total descending (GH #40), no
		// baseline deltas. Unscored suites stay out of the totals (they drop
		// from numerator and denominator alike, ADR 0005).
		members, err := s.db.ListCampaignMembers(id)
		if err != nil {
			return fail(http.StatusInternalServerError, "failed to load campaign members")
		}
		rows = liveReportRows(members, cellProgress, scores, weights, nadirs)
		if family != "" {
			rows = filterReportRowsByFamily(rows, family)
		}
		rows = attachReportCells(rows, suites, runs, cellProgress, expectedBySuite, withCost)
	}

	progress := store.CampaignWithProgress{Campaign: *campaign}
	for _, run := range runs {
		progress.Progress.Total++
		switch run.Status {
		case "done":
			progress.Progress.Done++
		case "failed":
			progress.Progress.Failed++
		default:
			progress.Progress.Running++
		}
	}

	failedResults, err := s.db.CountCampaignNullScoreResults(id)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to count failed results")
	}

	// GH #42 cost metrics: session-only. The shared-report and public-board
	// callers pass withCost=false, so the fields stay absent there by
	// construction (never serialized as zeroed values).
	var cost *campaignCostDTO
	var costRows []costRowDTO
	var estimated *estimatedCostDTO
	var queueDepth *queueDepthDTO
	if !shared {
		queueDepth = s.liveQueueDepth(id, campaign.Status)
	}
	if withCost {
		totals, err := s.db.CampaignCostTotals(id)
		if err != nil {
			return fail(http.StatusInternalServerError, "failed to aggregate campaign cost")
		}
		cost = &campaignCostDTO{
			LatencyMs:    totals.LatencyMs,
			InputTokens:  totals.InputTokens,
			OutputTokens: totals.OutputTokens,
		}
		rows2, err := s.db.ListCampaignCostRows(id)
		if err != nil {
			return fail(http.StatusInternalServerError, "failed to load campaign cost rows")
		}
		costRows = make([]costRowDTO, 0, len(rows2))
		for _, cr := range rows2 {
			costRows = append(costRows, costRowDTO{
				ModelID:      cr.ModelID,
				SuiteKey:     cr.SuiteKey,
				SuiteName:    cr.SuiteName,
				Status:       cr.RunStatus,
				LatencyMs:    cr.LatencyMs,
				InputTokens:  cr.InputTokens,
				OutputTokens: cr.OutputTokens,
				AvgTPS:       cr.AvgTPS,
				ExamCost:     examCostOf(cr, s.registryOverrides()),
			})
		}
		estimated = s.campaignEstimatedCost(id)
	}

	return campaignReportDTO{
		campaignDTO:   toCampaignDTO(progress),
		Suites:        suites,
		Weights:       weights,
		Rows:          rows,
		Baseline:      baseline,
		FailedResults: failedResults,
		Cost:          cost,
		CostRows:      costRows,
		// Cost metrics stay console-only (the GH #42 caliber the ticket-54
		// review registered): the shared/public board strips both the
		// registry-priced estimate and the live queue state.
		EstimatedCost: estimated,
		QueueDepth:    queueDepth,
	}, nil
}

// campaignEstimatedCost sums every run's registry-priced split; runs with a
// null estimate are counted, never guessed.
func (s *Server) campaignEstimatedCost(campaignID int64) *estimatedCostDTO {
	runs, err := s.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		return nil
	}
	out := &estimatedCostDTO{}
	for _, run := range runs {
		if run.EstimatedCost == "" {
			out.UnknownRuns++
			continue
		}
		var split struct {
			Exam  float64 `json:"exam"`
			Judge float64 `json:"judge"`
		}
		if err := json.Unmarshal([]byte(run.EstimatedCost), &split); err != nil {
			out.UnknownRuns++
			continue
		}
		out.Exam += split.Exam
		out.Judge += split.Judge
	}
	return out
}

// registryOverrides reads the administrator's registry overrides for
// report-side pricing (GH #178); a corrupt stored value falls back to the
// built-in table.
func (s *Server) registryOverrides() []registry.Override {
	raw, err := s.db.GetSetting(store.SettingModelRegistryOverrides, "")
	if err != nil {
		return nil
	}
	overrides, err := registry.ParseOverrides(raw)
	if err != nil {
		return nil
	}
	return overrides
}

// examCostOf prices one cost row's answer tokens against the registry:
// null when the model's price is unregistered or the run recorded no
// tokens (GH #178).
func examCostOf(cr store.CampaignCostRow, overrides []registry.Override) *float64 {
	info := registry.Lookup(cr.ModelID, overrides)
	if info.PriceIn == nil || info.PriceOut == nil || cr.InputTokens == nil || cr.OutputTokens == nil {
		return nil
	}
	cost := (float64(*cr.InputTokens)**info.PriceIn + float64(*cr.OutputTokens)**info.PriceOut) / 1e6
	return &cost
}

// liveQueueDepth reads the campaign's pipeline queue state while it is
// executing; nil on settled campaigns or when the batch is not running on
// this process.
func (s *Server) liveQueueDepth(campaignID int64, status string) *queueDepthDTO {
	if status != store.CampaignStatusRunning {
		return nil
	}
	examPending, examInflight, judgePending, judgeInflight, ok := s.evaluator.LiveQueueDepth(campaignID)
	if !ok {
		return nil
	}
	return &queueDepthDTO{
		ExamPending:   examPending,
		ExamInflight:  examInflight,
		JudgePending:  judgePending,
		JudgeInflight: judgeInflight,
	}
}

// reportBaseline resolves the previous done campaign and, when it covered
// the same suites at the same question-bank versions scored under the same
// verdict profile, computes per-row total deltas against it (same weights,
// same suite set — the same caliber). Any version, profile or coverage
// mismatch marks the baseline incomparable and yields no deltas, so neither
// a question-bank change (ADR 0007) nor a scoring-caliber change (ADR 0008)
// ever reads as a model change. The baseline's own totals pass through the
// same coverage gate as the current rows (gateSuites), so a partial
// baseline total — a coverage artifact, not a score — offers nothing to
// compare against and yields no delta either. Deltas come back as a map
// keyed by model database id; the caller applies them to fresh rows.
func (s *Server) reportBaseline(campaignID int64, suites, gateSuites []reportSuiteDTO, weights map[string]float64, rows []reportRowDTO) (*reportBaselineDTO, map[int64]*float64, error) {
	prev, err := s.db.PreviousDoneCampaign(campaignID)
	if err != nil {
		return nil, nil, err
	}
	if prev == nil {
		return nil, nil, nil
	}
	baseline := &reportBaselineDTO{CampaignID: prev.ID, Comparable: true}

	prevRuns, err := s.db.ListEvalRunsByCampaign(prev.ID)
	if err != nil {
		return nil, nil, err
	}
	prevVersions := suiteVersionSnapshot(prevRuns)
	for _, suite := range suites {
		prevVersion, covered := prevVersions[suite.ID]
		if !covered {
			baseline.Comparable = false
			baseline.Reason = "suite_missing"
			break
		}
		if prevVersion != suite.Version {
			baseline.Comparable = false
			baseline.Reason = "suite_changed"
			break
		}
	}
	if baseline.Comparable {
		reason, err := s.profileBreakReason(campaignID, prev.ID, suites)
		if err != nil {
			return nil, nil, err
		}
		if reason != "" {
			baseline.Comparable = false
			baseline.Reason = reason
		}
	}
	if !baseline.Comparable {
		return baseline, nil, nil
	}

	prevScores, err := s.db.ListCampaignSuiteScores(prev.ID)
	if err != nil {
		return nil, nil, err
	}
	// The baseline's totals normalize with the baseline runs' own nadir
	// snapshots — comparable caliber does not imply identical constants, and
	// each side must be scaled by the constant it was scored under.
	prevNadirs := nadirBySuiteKey(suites, prevRuns)
	prevTotals := map[int64]*float64{}
	for _, row := range applyCoverageGate(buildReportRows(prevScores, weights, prevNadirs), gateSuites) {
		prevTotals[row.ModelDBID] = row.TotalScore
	}
	deltas := map[int64]*float64{}
	for _, row := range rows {
		prevTotal, ok := prevTotals[row.ModelDBID]
		if !ok || prevTotal == nil || row.TotalScore == nil {
			continue
		}
		delta := *row.TotalScore - *prevTotal
		deltas[row.ModelDBID] = &delta
	}
	return baseline, deltas, nil
}

// profileBreakReason reports "profile_changed" when the two campaigns scored
// any shared suite under different verdict profiles (ADR 0008), the empty
// string otherwise. A suite with no results on either side cannot prove a
// break and is skipped — the version check upstream already guarantees the
// same question bank.
func (s *Server) profileBreakReason(campaignID, prevID int64, suites []reportSuiteDTO) (string, error) {
	curProfiles, err := s.db.CampaignVerdictProfiles(campaignID)
	if err != nil {
		return "", err
	}
	prevProfiles, err := s.db.CampaignVerdictProfiles(prevID)
	if err != nil {
		return "", err
	}
	for _, suite := range suites {
		cur, okCur := curProfiles[suite.ID]
		prevProfile, okPrev := prevProfiles[suite.ID]
		if okCur && okPrev && cur != prevProfile {
			return "profile_changed", nil
		}
	}
	return "", nil
}

// campaignSuites returns the suites covered by the campaign's runs, deduped
// and ordered by suite id.
func (s *Server) campaignSuites(runs []store.EvalRun) ([]reportSuiteDTO, error) {
	versionBySuite := suiteVersionSnapshot(runs)
	all, err := s.db.ListSuites()
	if err != nil {
		return nil, err
	}
	suites := make([]reportSuiteDTO, 0, len(versionBySuite))
	for _, suite := range all {
		if version, covered := versionBySuite[suite.ID]; covered {
			suites = append(suites, reportSuiteDTO{
				ID:      suite.ID,
				Key:     suite.Key,
				Name:    suite.Name,
				Version: version,
			})
		}
	}
	return suites, nil
}

// suiteVersionSnapshot maps each suite covered by the runs to the
// question-bank version the runs scored against. The version comes from the
// run's suite_version snapshot, not the suites table: after a question-bank
// edit the historical report must keep showing the version it actually
// scored against (ADR 0007).
func suiteVersionSnapshot(runs []store.EvalRun) map[int64]int {
	versions := make(map[int64]int, len(runs))
	for _, run := range runs {
		versions[run.SuiteID] = run.SuiteVersion
	}
	return versions
}

// hasSuiteKey reports whether key names one of the given suites.
func hasSuiteKey(suites []reportSuiteDTO, key string) bool {
	for _, suite := range suites {
		if suite.Key == key {
			return true
		}
	}
	return false
}

// effectiveWeights resolves the configured weight map against the campaign's
// suites: a configured positive value wins, everything else weighs 1 (equal
// weighting is the default). Returns a fresh map; inputs stay untouched.
func effectiveWeights(suites []reportSuiteDTO, configured map[string]float64) map[string]float64 {
	weights := make(map[string]float64, len(suites))
	for _, suite := range suites {
		w := configured[suite.Key]
		if w <= 0 {
			w = 1
		}
		weights[suite.Key] = w
	}
	return weights
}
