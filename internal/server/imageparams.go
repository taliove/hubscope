package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/taliove/hubscope/internal/imageparams"
)

// imageParamRuleDTO is the API representation of an image param rule.
type imageParamRuleDTO struct {
	ID       int64             `json:"id"`
	Keyword  string            `json:"keyword"`
	Params   map[string]string `json:"params"`
	Priority int               `json:"priority"`
}

// createImageParamRuleRequest is the body for POST /api/image-param-rules.
// Params stays raw so a non-string value produces a targeted 400 rather than
// a generic decode failure.
type createImageParamRuleRequest struct {
	Keyword  string          `json:"keyword"`
	Params   json.RawMessage `json:"params"`
	Priority *int            `json:"priority"`
}

// patchImageParamRuleRequest is the body for PATCH
// /api/image-param-rules/{id}. Nil fields stay unchanged; a present-but-null
// params is rejected (a rule without params is meaningless).
type patchImageParamRuleRequest struct {
	Keyword  *string          `json:"keyword"`
	Params   *json.RawMessage `json:"params"`
	Priority *int             `json:"priority"`
}

// toImageParamRuleDTO maps an imageparams.Rule to its API representation.
func toImageParamRuleDTO(r imageparams.Rule) imageParamRuleDTO {
	return imageParamRuleDTO{
		ID:       r.ID,
		Keyword:  r.Keyword,
		Params:   r.Params,
		Priority: r.Priority,
	}
}

// parseImageParams decodes and validates a rule's params object: it must be a
// non-empty string map and may not touch the reserved keys (model/prompt/n —
// the probe contract owns them). Values are strings only in v1 (GH #33
// decision 3).
func parseImageParams(raw json.RawMessage) (map[string]string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "params is required"
	}
	var params map[string]string
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, "params must be an object with string values"
	}
	if len(params) == 0 {
		return nil, "params must not be empty"
	}
	for key := range params {
		if strings.TrimSpace(key) == "" {
			return nil, "params keys must not be empty"
		}
		if imageparams.IsReservedKey(key) {
			return nil, fmt.Sprintf("params key %q is reserved (model/prompt/n belong to the probe contract)", key)
		}
	}
	return params, ""
}

// handleListImageParamRules handles GET /api/image-param-rules.
func (s *Server) handleListImageParamRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.db.ListImageParamRules()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	dtos := make([]imageParamRuleDTO, 0, len(rules))
	for _, rule := range rules {
		dtos = append(dtos, toImageParamRuleDTO(rule))
	}
	writeData(w, http.StatusOK, dtos)
}

// handleCreateImageParamRule handles POST /api/image-param-rules. Matching
// re-resolves from the store on every probe, so the rule takes effect on the
// very next trial or round — nothing to recompute here.
func (s *Server) handleCreateImageParamRule(w http.ResponseWriter, r *http.Request) {
	var req createImageParamRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Keywords normalize to lowercase: matching is case-insensitive, and case
	// variants of one word must not become two rules (UNIQUE(keyword)).
	keyword := strings.ToLower(strings.TrimSpace(req.Keyword))
	if keyword == "" {
		writeError(w, http.StatusBadRequest, "keyword is required")
		return
	}
	params, errMsg := parseImageParams(req.Params)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	priority := defaultRulePriority
	if req.Priority != nil {
		priority = *req.Priority
	}
	if priority < 1 || priority > maxRulePriority {
		writeError(w, http.StatusBadRequest, "priority must be between 1 and 10000")
		return
	}

	rule, err := s.db.CreateImageParamRule(keyword, params, priority)
	if err != nil {
		if isUniqueViolation(err) {
			s.audit(r, "image_param_rule.create", "image_param_rule", "",
				fmt.Sprintf("%q", keyword), "failed: duplicate rule")
			writeError(w, http.StatusConflict, "a rule with this keyword already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}

	s.audit(r, "image_param_rule.create", "image_param_rule", strconv.FormatInt(rule.ID, 10),
		fmt.Sprintf("%q -> %v (priority %d)", rule.Keyword, rule.Params, rule.Priority), "success")
	writeData(w, http.StatusCreated, toImageParamRuleDTO(*rule))
}

// handlePatchImageParamRule handles PATCH /api/image-param-rules/{id}.
func (s *Server) handlePatchImageParamRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	var req patchImageParamRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Keyword != nil {
		trimmed := strings.ToLower(strings.TrimSpace(*req.Keyword))
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "keyword must not be empty")
			return
		}
		req.Keyword = &trimmed
	}
	var params map[string]string
	if req.Params != nil {
		parsed, errMsg := parseImageParams(*req.Params)
		if errMsg != "" {
			writeError(w, http.StatusBadRequest, errMsg)
			return
		}
		params = parsed
	}
	if req.Priority != nil && (*req.Priority < 1 || *req.Priority > maxRulePriority) {
		writeError(w, http.StatusBadRequest, "priority must be between 1 and 10000")
		return
	}

	if _, err := s.db.GetImageParamRule(id); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	rule, err := s.db.UpdateImageParamRule(id, req.Keyword, params, req.Priority)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a rule with this keyword already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}

	// A no-op patch (all fields absent) changes nothing — skip the audit.
	if req.Keyword == nil && req.Params == nil && req.Priority == nil {
		writeData(w, http.StatusOK, toImageParamRuleDTO(*rule))
		return
	}
	s.audit(r, "image_param_rule.update", "image_param_rule", strconv.FormatInt(rule.ID, 10),
		fmt.Sprintf("%q -> %v (priority %d)", rule.Keyword, rule.Params, rule.Priority), "success")
	writeData(w, http.StatusOK, toImageParamRuleDTO(*rule))
}

// handleDeleteImageParamRule handles DELETE /api/image-param-rules/{id}.
func (s *Server) handleDeleteImageParamRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	if _, err := s.db.GetImageParamRule(id); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	if err := s.db.DeleteImageParamRule(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	s.audit(r, "image_param_rule.delete", "image_param_rule", strconv.FormatInt(id, 10), "", "success")
	writeNoContent(w)
}
