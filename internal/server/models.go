package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// createModelRequest is the body for POST /api/models.
type createModelRequest struct {
	HubID   int64  `json:"hub_id"`
	ModelID string `json:"model_id"`
}

// handleCreateModel handles POST /api/models. Auto-creates two endpoints.
func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.HubID == 0 || req.ModelID == "" {
		writeError(w, http.StatusBadRequest, "hub_id and model_id are required")
		return
	}

	if _, err := s.db.GetHub(req.HubID); err != nil {
		writeError(w, http.StatusNotFound, "hub not found")
		return
	}

	model, err := s.db.CreateModel(req.HubID, req.ModelID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "model_id already exists for this hub")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create model")
		return
	}

	endpoints, err := s.db.ListEndpointsByModelID(model.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load endpoints")
		return
	}

	writeData(w, http.StatusCreated, toModelDTO(*model, endpoints))
}

// handleListModels handles GET /api/models. Includes endpoints per model.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.db.ListModels()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}

	dtos := make([]modelDTO, 0, len(models))
	for _, m := range models {
		endpoints, err := s.db.ListEndpointsByModelID(m.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load endpoints")
			return
		}
		dtos = append(dtos, toModelDTO(m, endpoints))
	}

	writeData(w, http.StatusOK, dtos)
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
