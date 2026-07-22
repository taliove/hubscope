package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// createModelRequest is the body for POST /api/models.
type createModelRequest struct {
	HubID   int64  `json:"hub_id"`
	ModelID string `json:"model_id"`
}

// modelProtocols lists both hub API protocols in canonical endpoint order.
var modelProtocols = []string{"anthropic", "openai"}

// handleCreateModel handles POST /api/models. The model is trial-probed on
// both protocols first: an endpoint is created per protocol that answered.
// A model unreachable on both is rejected with 400 and nothing is stored.
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

	hub, err := s.db.GetHub(req.HubID)
	if err != nil {
		writeError(w, http.StatusNotFound, "hub not found")
		return
	}

	working, failures := s.trialProtocols(r.Context(), *hub, req.ModelID)
	if len(working) == 0 {
		s.audit(r, "model.create", "model", req.ModelID, failures, "failed: unreachable on both protocols")
		writeError(w, http.StatusBadRequest,
			"model is unreachable on both anthropic and openai protocols: "+failures)
		return
	}

	model, err := s.db.CreateModel(req.HubID, req.ModelID, working)
	if err != nil {
		if isUniqueViolation(err) {
			s.audit(r, "model.create", "model", req.ModelID, "", "failed: duplicate model_id")
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

	s.audit(r, "model.create", "model", strconv.FormatInt(model.ID, 10),
		fmt.Sprintf("model_id=%q hub_id=%d protocols=%v capability=%s family=%s",
			model.ModelID, model.HubID, working, model.Capability, model.Family), "success")
	writeData(w, http.StatusCreated, toModelDTO(*model, endpoints))
}

// trialProtocols probes the model on both hub protocols and returns the
// protocols that answered plus a human-readable summary of the failures.
func (s *Server) trialProtocols(ctx context.Context, hub store.Hub, modelID string) ([]string, string) {
	client := hubclient.New()
	working := []string{}
	failures := []string{}
	for _, protocol := range modelProtocols {
		result := client.Probe(ctx, hub.BaseURL, hub.Token, protocol, modelID, false)
		if result.OK {
			working = append(working, protocol)
			continue
		}
		reason := fmt.Sprintf("%s: HTTP %d", protocol, result.HTTPStatus)
		if result.ErrorSummary != nil {
			reason = fmt.Sprintf("%s: %s", protocol, *result.ErrorSummary)
		}
		failures = append(failures, reason)
	}
	return working, strings.Join(failures, "; ")
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

// handleDeleteModel handles DELETE /api/models/{id}. Only manual models can
// be deleted (a discovered one would be resurrected by the next sync); the
// model's endpoints and their history are removed together with it.
func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid model id")
		return
	}

	if _, err := s.db.GetModel(id); err != nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	if err := s.db.DeleteModel(id); err != nil {
		if errors.Is(err, store.ErrModelNotManual) {
			s.audit(r, "model.delete", "model", strconv.FormatInt(id, 10), "", "failed: discovered model")
			writeError(w, http.StatusConflict, "discovered models cannot be deleted; disable their endpoints instead")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete model")
		return
	}

	s.audit(r, "model.delete", "model", strconv.FormatInt(id, 10), "", "success")
	writeNoContent(w)
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
