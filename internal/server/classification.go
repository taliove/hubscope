package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.github.net/taliove2009/ai-hub-checker/internal/classifier"
)

// classificationRuleDTO is the API representation of a classification rule.
type classificationRuleDTO struct {
	ID        int64  `json:"id"`
	Dimension string `json:"dimension"`
	Keyword   string `json:"keyword"`
	Category  string `json:"category"`
	Priority  int    `json:"priority"`
}

// createRuleRequest is the body for POST /api/classification-rules.
type createRuleRequest struct {
	Dimension string `json:"dimension"`
	Keyword   string `json:"keyword"`
	Category  string `json:"category"`
	Priority  *int   `json:"priority"`
}

// patchRuleRequest is the body for PATCH /api/classification-rules/{id}.
// Nil fields stay unchanged.
type patchRuleRequest struct {
	Keyword  *string `json:"keyword"`
	Category *string `json:"category"`
	Priority *int    `json:"priority"`
}

// defaultRulePriority applies when a create request omits priority.
const defaultRulePriority = 100

// maxRulePriority bounds the accepted priority range.
const maxRulePriority = 10000

// toRuleDTO maps a classifier.Rule to its API representation.
func toRuleDTO(r classifier.Rule) classificationRuleDTO {
	return classificationRuleDTO{
		ID:        r.ID,
		Dimension: r.Dimension,
		Keyword:   r.Keyword,
		Category:  r.Category,
		Priority:  r.Priority,
	}
}

// handleListClassificationRules handles GET /api/classification-rules.
func (s *Server) handleListClassificationRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.db.ListClassificationRules()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	dtos := make([]classificationRuleDTO, 0, len(rules))
	for _, rule := range rules {
		dtos = append(dtos, toRuleDTO(rule))
	}
	writeData(w, http.StatusOK, dtos)
}

// handleCreateClassificationRule handles POST /api/classification-rules and
// reclassifies all models against the new rule set.
func (s *Server) handleCreateClassificationRule(w http.ResponseWriter, r *http.Request) {
	var req createRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Dimension = strings.TrimSpace(req.Dimension)
	req.Keyword = strings.TrimSpace(req.Keyword)
	req.Category = strings.TrimSpace(req.Category)
	if req.Dimension != classifier.DimensionCapability && req.Dimension != classifier.DimensionFamily {
		writeError(w, http.StatusBadRequest, "dimension must be capability or family")
		return
	}
	if req.Keyword == "" || req.Category == "" {
		writeError(w, http.StatusBadRequest, "keyword and category are required")
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

	rule, err := s.db.CreateClassificationRule(req.Dimension, strings.ToLower(req.Keyword), req.Category, priority)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a rule with this dimension and keyword already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}

	if err := s.db.ReclassifyAll(); err != nil {
		writeError(w, http.StatusInternalServerError, "rule saved but reclassification failed")
		return
	}
	writeData(w, http.StatusCreated, toRuleDTO(*rule))
}

// handlePatchClassificationRule handles PATCH /api/classification-rules/{id}
// and reclassifies all models against the updated rule set.
func (s *Server) handlePatchClassificationRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	var req patchRuleRequest
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
	if req.Category != nil {
		trimmed := strings.TrimSpace(*req.Category)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "category must not be empty")
			return
		}
		req.Category = &trimmed
	}
	if req.Priority != nil && (*req.Priority < 1 || *req.Priority > maxRulePriority) {
		writeError(w, http.StatusBadRequest, "priority must be between 1 and 10000")
		return
	}

	if _, err := s.db.GetClassificationRule(id); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	rule, err := s.db.UpdateClassificationRule(id, req.Keyword, req.Category, req.Priority)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a rule with this dimension and keyword already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}

	// A no-op patch (all fields absent) changes nothing — skip the reclassify.
	if req.Keyword == nil && req.Category == nil && req.Priority == nil {
		writeData(w, http.StatusOK, toRuleDTO(*rule))
		return
	}
	if err := s.db.ReclassifyAll(); err != nil {
		writeError(w, http.StatusInternalServerError, "rule saved but reclassification failed")
		return
	}
	writeData(w, http.StatusOK, toRuleDTO(*rule))
}

// handleDeleteClassificationRule handles DELETE /api/classification-rules/{id}
// and reclassifies all models against the remaining rule set.
func (s *Server) handleDeleteClassificationRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	if _, err := s.db.GetClassificationRule(id); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	if err := s.db.DeleteClassificationRule(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}

	if err := s.db.ReclassifyAll(); err != nil {
		writeError(w, http.StatusInternalServerError, "rule saved but reclassification failed")
		return
	}
	writeNoContent(w)
}
