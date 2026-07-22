package server

import (
	"net/http"
	"sort"

	"github.com/taliove2009/hubscope/internal/store"
)

// reportSuiteDTO is the suite metadata the leaderboard needs: identity plus
// the question-bank version the campaign's runs scored against.
type reportSuiteDTO struct {
	ID      int64  `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// reportRowDTO is one leaderboard row: a live model with its per-suite
// scores (0-100, null when unscored) and the weighted total (ADR 0005).
type reportRowDTO struct {
	ModelDBID   int64               `json:"model_db_id"`
	ModelID     string              `json:"model_id"`
	Family      string              `json:"family"`
	TotalScore  *float64            `json:"total_score"`
	SuiteScores map[string]*float64 `json:"suite_scores"`
}

// campaignReportDTO is GET /api/campaigns/{id}/report: the campaign, the
// suites it covers, the effective weights used for the totals, and the
// leaderboard rows.
type campaignReportDTO struct {
	campaignDTO
	Suites  []reportSuiteDTO   `json:"suites"`
	Weights map[string]float64 `json:"weights"`
	Rows    []reportRowDTO     `json:"rows"`
}

// handleGetCampaignReport handles GET /api/campaigns/{id}/report. Only done
// runs contribute; deleted models (ticket 26 semantics) never rank. Query
// params: family filters rows to one model family; sort picks the ranking
// column ("total" by default, or a covered suite key), always descending
// with unscored models last.
func (s *Server) handleGetCampaignReport(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	campaign, err := s.db.GetCampaign(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	runs, err := s.db.ListEvalRunsByCampaign(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign runs")
		return
	}

	suites, err := s.campaignSuites(runs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load suites")
		return
	}

	sortKey := r.URL.Query().Get("sort")
	if sortKey == "" {
		sortKey = "total"
	}
	if sortKey != "total" && !hasSuiteKey(suites, sortKey) {
		writeError(w, http.StatusBadRequest, "sort must be \"total\" or a suite key of this campaign")
		return
	}

	configured, err := s.db.GetSuiteWeights()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read suite weights")
		return
	}
	weights := effectiveWeights(suites, configured)

	scores, err := s.db.ListCampaignSuiteScores(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to aggregate campaign scores")
		return
	}

	rows := buildReportRows(scores, weights)
	if family := r.URL.Query().Get("family"); family != "" {
		rows = filterReportRowsByFamily(rows, family)
	}
	sortReportRows(rows, sortKey)
	// Non-terminal campaigns show progress only (spec 0002 review
	// condition): half-scored rows must not leak to readers or future
	// share links. An empty slice keeps the contract as [] rather than null.
	if campaign.Status != "done" && campaign.Status != "failed" {
		rows = make([]reportRowDTO, 0)
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

	writeData(w, http.StatusOK, campaignReportDTO{
		campaignDTO: toCampaignDTO(progress),
		Suites:      suites,
		Weights:     weights,
		Rows:        rows,
	})
}

// campaignSuites returns the suites covered by the campaign's runs, deduped
// and ordered by suite id.
func (s *Server) campaignSuites(runs []store.EvalRun) ([]reportSuiteDTO, error) {
	// Version comes from the run's suite_version snapshot, not the suites
	// table: after a question-bank edit the historical report must keep
	// showing the version it actually scored against (ADR 0007).
	versionBySuite := map[int64]int{}
	for _, run := range runs {
		versionBySuite[run.SuiteID] = run.SuiteVersion
	}
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

// buildReportRows groups per-(model, suite) aggregates into leaderboard rows,
// scaling scores to 0-100 and attaching the ADR-0005 weighted total.
func buildReportRows(scores []store.CampaignSuiteScore, weights map[string]float64) []reportRowDTO {
	rows := []reportRowDTO{}
	indexByModel := map[int64]int{}
	for _, sc := range scores {
		idx, ok := indexByModel[sc.ModelDBID]
		if !ok {
			idx = len(rows)
			indexByModel[sc.ModelDBID] = idx
			rows = append(rows, reportRowDTO{
				ModelDBID:   sc.ModelDBID,
				ModelID:     sc.ModelID,
				Family:      sc.Family,
				SuiteScores: map[string]*float64{},
			})
		}
		var scaled *float64
		if sc.Score != nil {
			v := *sc.Score * 100
			scaled = &v
		}
		rows[idx].SuiteScores[sc.SuiteKey] = scaled
	}
	for i := range rows {
		rows[i].TotalScore = totalScore(rows[i].SuiteScores, weights)
	}
	return rows
}

// totalScore is the ADR-0005 total: the weighted mean of the model's scored
// suites on the 0-100 scale. Unscored suites drop out of both numerator and
// denominator; nil when the model scored nothing at all.
func totalScore(suiteScores map[string]*float64, weights map[string]float64) *float64 {
	var sum, wsum float64
	for key, score := range suiteScores {
		if score == nil {
			continue
		}
		w := weights[key]
		if w <= 0 {
			w = 1
		}
		sum += w * *score
		wsum += w
	}
	if wsum == 0 {
		return nil
	}
	total := sum / wsum
	return &total
}

// filterReportRowsByFamily returns a new slice with only the rows of the
// given model family; the input slice is left untouched.
func filterReportRowsByFamily(rows []reportRowDTO, family string) []reportRowDTO {
	filtered := make([]reportRowDTO, 0, len(rows))
	for _, row := range rows {
		if row.Family == family {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// sortReportRows ranks rows descending by the chosen column ("total" or a
// suite key). Models without a score in that column rank last; ties break by
// model id for a stable, deterministic board.
func sortReportRows(rows []reportRowDTO, sortKey string) {
	scoreOf := func(row reportRowDTO) *float64 {
		if sortKey == "total" {
			return row.TotalScore
		}
		return row.SuiteScores[sortKey]
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := scoreOf(rows[i]), scoreOf(rows[j])
		if a == nil || b == nil {
			return a != nil
		}
		if *a != *b {
			return *a > *b
		}
		return rows[i].ModelID < rows[j].ModelID
	})
}
