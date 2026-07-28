package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/evaluator"
	"github.com/taliove/hubscope/internal/evaluator/ifeval"
	"github.com/taliove/hubscope/internal/store"
)

// validVerdictTypes and validRuleModes enumerate the accepted case configs.
// mcq (ADR 0013), numeric (ticket 95), output_match (ticket 98) and
// ifeval (ticket 97) must stay listed or admins could not curate benchmark
// cases through the case API (PATCH revalidates the merged case).
var validVerdictTypes = map[string]bool{"rule": true, "judge": true}
var validRuleModes = map[string]bool{"exact": true, "regex": true, "contains": true, "mcq": true, "numeric": true, "output_match": true, "ifeval": true}

// validDifficulties enumerates the accepted difficulty tiers.
var validDifficulties = map[string]bool{"basic": true, "intermediate": true, "hard": true}

// handleListSuites handles GET /api/suites. Each suite carries its cases.
// The optional capability query parameter filters to one capability
// dimension (ADR 0010); an empty value lists every suite, retired included.
func (s *Server) handleListSuites(w http.ResponseWriter, r *http.Request) {
	suites, err := s.db.ListSuites()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list suites")
		return
	}

	capability := r.URL.Query().Get("capability")
	dtos := make([]suiteDTO, 0, len(suites))
	for _, suite := range suites {
		if capability != "" && suite.Capability != capability {
			continue
		}
		cases, err := s.db.ListCases(suite.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load cases")
			return
		}
		dtos = append(dtos, toSuiteDTO(suite, cases))
	}

	writeData(w, http.StatusOK, dtos)
}

// createCaseRequest is the body for POST /api/cases. enabled defaults to
// true when omitted; difficulty defaults to basic; sample_count null means
// the case inherits the global default sample count.
type createCaseRequest struct {
	SuiteID     int64          `json:"suite_id"`
	Prompt      string         `json:"prompt"`
	VerdictType string         `json:"verdict_type"`
	RuleConfig  *ruleConfigDTO `json:"rule_config"`
	Rubric      *string        `json:"rubric"`
	Difficulty  *string        `json:"difficulty"`
	SampleCount *int           `json:"sample_count"`
	Enabled     *bool          `json:"enabled"`
}

// patchCaseRequest is the body for PATCH /api/cases/{id}. All fields are
// optional; absent fields stay unchanged. rubric is a pointer so it can be
// set but not distinguished from an explicit null (treated as unchanged).
// sample_count is raw so an explicit null clears the per-case override
// (back to inheriting the global default) while an absent field keeps it.
type patchCaseRequest struct {
	Prompt      *string         `json:"prompt"`
	VerdictType *string         `json:"verdict_type"`
	RuleConfig  *ruleConfigDTO  `json:"rule_config"`
	Rubric      *string         `json:"rubric"`
	Difficulty  *string         `json:"difficulty"`
	SampleCount json.RawMessage `json:"sample_count"`
	Enabled     *bool           `json:"enabled"`
}

// handleCreateCase handles POST /api/cases.
func (s *Server) handleCreateCase(w http.ResponseWriter, r *http.Request) {
	var req createCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.SuiteID == 0 {
		writeError(w, http.StatusBadRequest, "suite_id is required")
		return
	}
	if _, err := s.db.GetSuite(req.SuiteID); err != nil {
		writeError(w, http.StatusNotFound, "suite not found")
		return
	}

	c := store.Case{
		SuiteID:     req.SuiteID,
		Prompt:      strings.TrimSpace(req.Prompt),
		VerdictType: req.VerdictType,
		Difficulty:  "basic",
		SampleCount: req.SampleCount,
		Enabled:     true,
		Rubric:      req.Rubric,
	}
	if req.Difficulty != nil {
		c.Difficulty = *req.Difficulty
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if req.RuleConfig != nil {
		c.RuleMode = &req.RuleConfig.Mode
		c.RuleExpected = &req.RuleConfig.Expected
	}

	if err := validateCase(c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.db.CreateCase(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create case")
		return
	}

	s.audit(r, "case.create", "case", strconv.FormatInt(created.ID, 10),
		fmt.Sprintf("suite_id=%d verdict=%s", created.SuiteID, created.VerdictType), "success")
	writeData(w, http.StatusCreated, toCaseDTO(*created))
}

// handlePatchCase handles PATCH /api/cases/{id}. Cases are immutable: a
// content change never edits the stored row — it disables the old case and
// creates a new one carrying the merged fields, so historical run results
// keep rendering the old prompt. An enabled-only change toggles the existing
// row in place. Both paths bump the parent suite's version; a patch that
// changes nothing is a no-op. The response carries the effective case (the
// new one for content edits).
func (s *Server) handlePatchCase(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid case id")
		return
	}

	existing, err := s.db.GetCase(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}

	var req patchCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	merged := *existing
	if req.Prompt != nil {
		merged.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.VerdictType != nil {
		merged.VerdictType = *req.VerdictType
	}
	if req.RuleConfig != nil {
		merged.RuleMode = &req.RuleConfig.Mode
		merged.RuleExpected = &req.RuleConfig.Expected
		if req.RuleConfig.Mode != "ifeval" {
			// check_params only applies to the ifeval mode; switching a case
			// to another mode drops the params from the minted case.
			merged.CheckParams = nil
		}
	}
	if req.Rubric != nil {
		merged.Rubric = req.Rubric
	}
	if req.Difficulty != nil {
		merged.Difficulty = *req.Difficulty
	}
	if len(req.SampleCount) > 0 {
		sampleCount, err := parseSampleCountPatch(req.SampleCount)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		merged.SampleCount = sampleCount
	}
	if req.Enabled != nil {
		merged.Enabled = *req.Enabled
	}

	if err := validateCase(merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if sameCase(*existing, merged) {
		writeData(w, http.StatusOK, toCaseDTO(*existing))
		return
	}

	var effective *store.Case
	if sameCaseContent(*existing, merged) {
		// Only the enabled flag changed: toggle in place.
		effective, err = s.db.SetCaseEnabled(id, merged.Enabled)
	} else {
		// Content changed: retire the old case, insert the merged copy.
		effective, err = s.db.ReplaceCase(id, merged)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update case")
		return
	}

	s.audit(r, "case.update", "case", strconv.FormatInt(effective.ID, 10), "", "success")
	writeData(w, http.StatusOK, toCaseDTO(*effective))
}

// sameCase reports whether two cases are identical in every mutable field.
func sameCase(a, b store.Case) bool {
	return sameCaseContent(a, b) && a.Enabled == b.Enabled
}

// sameCaseContent reports whether two cases carry the same question content
// (everything except the enabled flag and identity fields).
func sameCaseContent(a, b store.Case) bool {
	return a.Prompt == b.Prompt &&
		a.VerdictType == b.VerdictType &&
		strPtrEqual(a.RuleMode, b.RuleMode) &&
		strPtrEqual(a.RuleExpected, b.RuleExpected) &&
		strPtrEqual(a.Rubric, b.Rubric) &&
		a.Difficulty == b.Difficulty &&
		intPtrEqual(a.SampleCount, b.SampleCount) &&
		strPtrEqual(a.CheckParams, b.CheckParams)
}

// strPtrEqual compares two nullable strings by value.
func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// intPtrEqual compares two nullable ints by value.
func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// parseSampleCountPatch interprets the raw sample_count patch field: JSON
// null clears the override (nil), an integer sets it. Anything else is a 400.
func parseSampleCountPatch(raw json.RawMessage) (*int, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, fmt.Errorf("sample_count must be an integer or null")
	}
	return &n, nil
}

// numericExpectationPattern accepts the spellings a numeric expectation may
// use: optional leading "$", optional sign, integer part either plain or
// with correctly placed comma thousands separators, optional decimal part.
// "1,000" and "14000" pass; "1,2,3" does not (misplaced separators).
var numericExpectationPattern = regexp.MustCompile(`^\$?-?(\d{1,3}(,\d{3})+|\d+)(\.\d+)?$`)

// numericExpectationValid reports whether s canonicalizes to a plain number
// the numeric verdict could ever match.
func numericExpectationValid(s string) bool {
	return numericExpectationPattern.MatchString(strings.TrimSpace(s))
}

// validateCase checks a fully-populated case for consistency.
func validateCase(c store.Case) error {
	if c.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if !validVerdictTypes[c.VerdictType] {
		return fmt.Errorf("verdict_type must be rule or judge")
	}
	if c.VerdictType == "rule" {
		if c.RuleMode == nil || !validRuleModes[*c.RuleMode] {
			return fmt.Errorf("rule_config.mode must be exact, regex, contains, mcq, numeric, output_match or ifeval")
		}
		if *c.RuleMode == "ifeval" {
			// An ifeval case carries structured check parameters instead of
			// an expected string (ticket 97). They are cast only by the
			// benchmark seed — the admin API never authors check_params, so
			// this branch is reachable only when editing an existing seeded
			// case, and it revalidates the params fail-closed.
			if c.CheckParams == nil {
				return fmt.Errorf("rule_config.mode ifeval requires seeded check_params")
			}
			if err := ifeval.Validate(*c.CheckParams); err != nil {
				return fmt.Errorf("check_params is not valid IFEval check parameters: %v", err)
			}
		} else if c.RuleExpected == nil || *c.RuleExpected == "" {
			return fmt.Errorf("rule_config.expected is required")
		}
		if *c.RuleMode == "regex" {
			if _, err := regexp.Compile(*c.RuleExpected); err != nil {
				return fmt.Errorf("rule_config.expected is not a valid regex: %v", err)
			}
		}
		if *c.RuleMode == "mcq" {
			// An mcq expectation is one option letter; anything else could
			// never score a hit (ADR 0013).
			exp := strings.ToUpper(strings.TrimSpace(*c.RuleExpected))
			if len(exp) != 1 || !strings.Contains("ABCD", exp) {
				return fmt.Errorf("rule_config.expected must be a single option letter A-D for mcq")
			}
		}
		if *c.RuleMode == "numeric" {
			// A numeric expectation must canonicalize to a plain number
			// (sign, decimals, thousands separators allowed); anything else
			// could never score a hit (ticket 95).
			if !numericExpectationValid(*c.RuleExpected) {
				return fmt.Errorf("rule_config.expected must be a number for numeric")
			}
		}
		if *c.RuleMode == "output_match" {
			// An output_match expectation is the precomputed standard output
			// as a Python literal; anything else could never score a hit
			// (ticket 98). Validation is pure parsing — no code execution.
			if _, ok := evaluator.CanonicalPyLiteral(*c.RuleExpected); !ok {
				return fmt.Errorf("rule_config.expected must be a Python literal for output_match")
			}
		}
	}
	if c.VerdictType == "judge" && (c.Rubric == nil || strings.TrimSpace(*c.Rubric) == "") {
		return fmt.Errorf("rubric is required for judge cases")
	}
	if !validDifficulties[c.Difficulty] {
		return fmt.Errorf("difficulty must be basic, intermediate or hard")
	}
	if c.SampleCount != nil && (*c.SampleCount < 1 || *c.SampleCount > store.MaxSampleCount) {
		return fmt.Errorf("sample_count must be between 1 and %d", store.MaxSampleCount)
	}
	return nil
}

// createEvalRequest is the body for POST /api/evals. model_ids holds model
// database IDs (not model ID strings). suite_id omitted (zero) means a full
// sweep: every suite against every active chat-capable model, and model_ids
// is ignored.
type createEvalRequest struct {
	SuiteID  int64   `json:"suite_id"`
	ModelIDs []int64 `json:"model_ids"`
}

// handleCreateEval handles POST /api/evals. Every trigger produces a
// campaign: a full sweep (suite_id omitted) attaches one run per suite, a
// single-suite trigger attaches exactly one run. Runs execute asynchronously
// and sequentially; status and results are persisted and polled via GET.
// The response is the created campaign (runs may still be pending creation).
func (s *Server) handleCreateEval(w http.ResponseWriter, r *http.Request) {
	var req createEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Snapshot the configured judge model at creation; the evaluator re-reads
	// settings at run start and updates the record if it changed in between.
	judgeModel, err := s.db.GetSetting(store.SettingJudgeModel, store.DefaultJudgeModel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read judge model setting")
		return
	}

	if req.SuiteID == 0 {
		s.handleFullSweep(w, r, judgeModel)
		return
	}

	if len(req.ModelIDs) == 0 {
		writeError(w, http.StatusBadRequest, "model_ids must be a non-empty array")
		return
	}

	suite, err := s.db.GetSuite(req.SuiteID)
	if err != nil {
		writeError(w, http.StatusNotFound, "suite not found")
		return
	}

	for _, id := range req.ModelIDs {
		model, err := s.db.GetModel(id)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("model %d not found", id))
			return
		}
		if model.Capability != "chat" {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("model %d (%s) has capability %q and cannot be evaluated", id, model.ModelID, model.Capability))
			return
		}
	}

	campaign, err := s.db.CreateCampaign("manual", req.ModelIDs, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create campaign")
		return
	}
	run, err := s.db.CreateEvalRun(campaign.ID, suite.ID, "manual", judgeModel)
	if err != nil {
		// The campaign would otherwise linger as a running orphan until the
		// next restart's cleanup; close it out now.
		if serr := s.db.SettleCampaign(campaign.ID, time.Now().UTC()); serr != nil {
			slog.Error("settle orphan campaign after run creation failure", "campaign_id", campaign.ID, "error", serr)
		}
		writeError(w, http.StatusInternalServerError, "failed to create eval run")
		return
	}

	// Single-instance async execution: detached context so the run survives
	// the request; state is persisted in the store for polling. Settling goes
	// through the evaluator so a done campaign fires the alert hook.
	go func() {
		_ = s.evaluator.RunEval(context.Background(), run.ID, req.ModelIDs)
		s.evaluator.SettleCampaign(context.Background(), campaign.ID)
	}()

	s.audit(r, "eval.create", "campaign", strconv.FormatInt(campaign.ID, 10),
		fmt.Sprintf("suite_id=%d models=%d judge=%q", req.SuiteID, len(req.ModelIDs), judgeModel), "accepted")
	s.writeCampaignCreated(w, campaign.ID)
}

// handleListEvals handles GET /api/evals, newest first, scoped to the
// session's hub.
func (s *Server) handleListEvals(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	var runs []store.EvalRun
	var err error
	if u == nil || u.Role == store.RoleSuperAdmin {
		runs, err = s.db.ListEvalRunsAll()
	} else if u.HubID == nil {
		runs = []store.EvalRun{}
	} else {
		runs, err = s.db.ListEvalRunsByHub(*u.HubID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list eval runs")
		return
	}

	dtos := make([]evalRunDTO, 0, len(runs))
	for _, run := range runs {
		results, err := s.db.ListEvalResults(run.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load eval results")
			return
		}
		dtos = append(dtos, toEvalRunDTO(run, averageScore(results, run.Nadir)))
	}

	writeData(w, http.StatusOK, dtos)
}

// handleGetEval handles GET /api/evals/{id}, including per-case results.
func (s *Server) handleGetEval(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid eval run id")
		return
	}

	run, err := s.db.GetEvalRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "eval run not found")
		return
	}

	results, err := s.db.ListEvalResults(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load eval results")
		return
	}

	resultDTOs := make([]evalResultDTO, 0, len(results))
	for _, res := range results {
		resultDTOs = append(resultDTOs, toEvalResultDTO(res))
	}

	writeData(w, http.StatusOK, evalRunDetailDTO{
		evalRunDTO: toEvalRunDTO(*run, averageScore(results, run.Nadir)),
		Results:    resultDTOs,
	})
}

// handleLatestEvals handles GET /api/evals/latest: for every (suite, model)
// pair with at least one done run, the aggregate score of the most recent
// one, scoped to the session's hub.
func (s *Server) handleLatestEvals(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	var latest []store.LatestEvalScore
	var err error
	if u == nil || u.Role == store.RoleSuperAdmin {
		latest, err = s.db.ListLatestEvalScores()
	} else if u.HubID == nil {
		latest = []store.LatestEvalScore{}
	} else {
		latest, err = s.db.ListLatestEvalScoresByHub(*u.HubID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load latest eval scores")
		return
	}

	dtos := make([]latestScoreDTO, 0, len(latest))
	for _, ls := range latest {
		dtos = append(dtos, toLatestScoreDTO(ls))
	}
	writeData(w, http.StatusOK, dtos)
}

// averageScore computes the mean of all non-null scores, scaled through the
// ADR-0009 nadir normalization with the run's own nadir snapshot — the same
// caliber as the leaderboard, kept on the 0~1 wire scale the eval API has
// always spoken (nadir=0 degenerates to the legacy raw mean). Null scores
// (unjudged cases) are excluded, so they never drag the aggregate down.
// Returns nil when no scored result exists.
func averageScore(results []store.EvalResult, nadir float64) *float64 {
	var sum float64
	var n int
	for _, r := range results {
		if r.Score != nil {
			sum += *r.Score
			n++
		}
	}
	if n == 0 {
		return nil
	}
	avg := normalizeScore01(sum/float64(n), nadir)
	return &avg
}
