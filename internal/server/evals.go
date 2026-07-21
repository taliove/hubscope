package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"git.github.net/taliove2009/ai-hub-checker/internal/evaluator"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// validVerdictTypes and validRuleModes enumerate the accepted case configs.
var validVerdictTypes = map[string]bool{"rule": true, "judge": true}
var validRuleModes = map[string]bool{"exact": true, "regex": true, "contains": true}

// handleListSuites handles GET /api/suites. Each suite carries its cases.
func (s *Server) handleListSuites(w http.ResponseWriter, r *http.Request) {
	suites, err := s.db.ListSuites()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list suites")
		return
	}

	dtos := make([]suiteDTO, 0, len(suites))
	for _, suite := range suites {
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
// true when omitted.
type createCaseRequest struct {
	SuiteID     int64          `json:"suite_id"`
	Prompt      string         `json:"prompt"`
	VerdictType string         `json:"verdict_type"`
	RuleConfig  *ruleConfigDTO `json:"rule_config"`
	Rubric      *string        `json:"rubric"`
	Enabled     *bool          `json:"enabled"`
}

// patchCaseRequest is the body for PATCH /api/cases/{id}. All fields are
// optional; absent fields stay unchanged. rubric is a pointer so it can be
// set but not distinguished from an explicit null (treated as unchanged).
type patchCaseRequest struct {
	Prompt      *string        `json:"prompt"`
	VerdictType *string        `json:"verdict_type"`
	RuleConfig  *ruleConfigDTO `json:"rule_config"`
	Rubric      *string        `json:"rubric"`
	Enabled     *bool          `json:"enabled"`
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
		Enabled:     true,
		Rubric:      req.Rubric,
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

	writeData(w, http.StatusCreated, toCaseDTO(*created))
}

// handlePatchCase handles PATCH /api/cases/{id}. The patch is merged onto
// the stored case and the merged result is validated as a whole.
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
	}
	if req.Rubric != nil {
		merged.Rubric = req.Rubric
	}
	if req.Enabled != nil {
		merged.Enabled = *req.Enabled
	}

	if err := validateCase(merged); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.db.UpdateCase(merged)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update case")
		return
	}

	writeData(w, http.StatusOK, toCaseDTO(*updated))
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
			return fmt.Errorf("rule_config.mode must be exact, regex or contains")
		}
		if c.RuleExpected == nil || *c.RuleExpected == "" {
			return fmt.Errorf("rule_config.expected is required")
		}
		if *c.RuleMode == "regex" {
			if _, err := regexp.Compile(*c.RuleExpected); err != nil {
				return fmt.Errorf("rule_config.expected is not a valid regex: %v", err)
			}
		}
	}
	if c.VerdictType == "judge" && (c.Rubric == nil || strings.TrimSpace(*c.Rubric) == "") {
		return fmt.Errorf("rubric is required for judge cases")
	}
	return nil
}

// createEvalRequest is the body for POST /api/evals. model_ids holds model
// database IDs (not model ID strings).
type createEvalRequest struct {
	SuiteID  int64   `json:"suite_id"`
	ModelIDs []int64 `json:"model_ids"`
}

// handleCreateEval handles POST /api/evals. The run executes asynchronously
// in a goroutine; status and results are persisted and polled via GET.
func (s *Server) handleCreateEval(w http.ResponseWriter, r *http.Request) {
	var req createEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.SuiteID == 0 {
		writeError(w, http.StatusBadRequest, "suite_id is required")
		return
	}
	if len(req.ModelIDs) == 0 {
		writeError(w, http.StatusBadRequest, "model_ids must be a non-empty array")
		return
	}

	if _, err := s.db.GetSuite(req.SuiteID); err != nil {
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
				fmt.Sprintf("model %d (%s) is non_chat and cannot be evaluated", id, model.ModelID))
			return
		}
	}

	run, err := s.db.CreateEvalRun(req.SuiteID, "manual", evaluator.DefaultJudgeModel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create eval run")
		return
	}

	// Single-instance async execution: detached context so the run survives
	// the request; state is persisted in the store for polling.
	go func() {
		_ = s.evaluator.RunEval(context.Background(), run.ID, req.ModelIDs)
	}()

	writeData(w, http.StatusAccepted, toEvalRunDTO(*run, nil))
}

// handleListEvals handles GET /api/evals, newest first.
func (s *Server) handleListEvals(w http.ResponseWriter, r *http.Request) {
	runs, err := s.db.ListEvalRuns()
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
		dtos = append(dtos, toEvalRunDTO(run, averageScore(results)))
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
		evalRunDTO: toEvalRunDTO(*run, averageScore(results)),
		Results:    resultDTOs,
	})
}

// averageScore computes the mean of all non-null scores. Null scores
// (unjudged cases) are excluded, so they never drag the aggregate down.
// Returns nil when no scored result exists.
func averageScore(results []store.EvalResult) *float64 {
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
	avg := sum / float64(n)
	return &avg
}
