package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/taliove/hubscope/internal/discovery"
	"github.com/taliove/hubscope/internal/store"
)

// createHubRequest is the body for POST /api/hubs.
type createHubRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// updateHubRequest is the body for PUT /api/hubs/{id}. Nil fields are unchanged.
type updateHubRequest struct {
	Name    *string `json:"name"`
	BaseURL *string `json:"base_url"`
	Token   *string `json:"token"`
}

// handleCreateHub handles POST /api/hubs.
func (s *Server) handleCreateHub(w http.ResponseWriter, r *http.Request) {
	var req createHubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	if req.Name == "" || req.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "name and base_url are required")
		return
	}

	hub, err := s.db.CreateHub(req.Name, req.BaseURL, req.Token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create hub")
		return
	}

	// Kick off the first model sync in the background so the new hub's
	// models appear without waiting for the periodic full sync. The response
	// must not block on it: a large hub takes minutes to probe.
	if err := s.discovery.StartSync(hub.ID, store.TaskSourceManual); err != nil {
		// Only ErrSyncInProgress is possible, and a fresh hub cannot be
		// syncing — log defensively rather than failing the creation.
		slog.Error("create hub: start sync failed", "hub_id", hub.ID, "error", err)
	} else {
		// StartSync already persisted the syncing mark; reflect it.
		hub.SyncStatus = store.HubSyncRunning
	}

	s.audit(r, "hub.create", "hub", strconv.FormatInt(hub.ID, 10),
		fmt.Sprintf("name=%q base_url=%q", hub.Name, hub.BaseURL), "success")
	writeData(w, http.StatusCreated, toHubDTO(*hub))
}

// handleListHubs handles GET /api/hubs.
func (s *Server) handleListHubs(w http.ResponseWriter, r *http.Request) {
	hubs, err := s.db.ListHubs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list hubs")
		return
	}

	dtos := make([]hubDTO, 0, len(hubs))
	for _, h := range hubs {
		dtos = append(dtos, toHubDTO(h))
	}

	writeData(w, http.StatusOK, dtos)
}

// handleUpdateHub handles PUT /api/hubs/{id}.
func (s *Server) handleUpdateHub(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hub id")
		return
	}

	var req updateHubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if _, err := s.db.GetHub(id); err != nil {
		writeError(w, http.StatusNotFound, "hub not found")
		return
	}

	hub, err := s.db.UpdateHub(id, req.Name, req.BaseURL, req.Token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update hub")
		return
	}

	s.audit(r, "hub.update", "hub", strconv.FormatInt(id, 10), hubUpdateDetail(req), "success")
	writeData(w, http.StatusOK, toHubDTO(*hub))
}

// handleDeleteHub handles DELETE /api/hubs/{id}. Returns 409 if models exist.
func (s *Server) handleDeleteHub(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hub id")
		return
	}

	if _, err := s.db.GetHub(id); err != nil {
		writeError(w, http.StatusNotFound, "hub not found")
		return
	}

	if err := s.db.DeleteHub(id); err != nil {
		if errors.Is(err, store.ErrHubHasModels) {
			s.audit(r, "hub.delete", "hub", strconv.FormatInt(id, 10), "", "failed: hub has models")
			writeError(w, http.StatusConflict, "hub has associated models and cannot be deleted")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete hub")
		return
	}

	s.audit(r, "hub.delete", "hub", strconv.FormatInt(id, 10), "", "success")
	writeNoContent(w)
}

// handleSyncHub handles POST /api/hubs/{id}/sync. It starts an asynchronous
// sync for the hub and answers 202; a sync already in flight yields 409.
func (s *Server) handleSyncHub(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hub id")
		return
	}

	hub, err := s.db.GetHub(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "hub not found")
		return
	}

	if err := s.discovery.StartSync(id, store.TaskSourceManual); err != nil {
		if errors.Is(err, discovery.ErrSyncInProgress) {
			writeError(w, http.StatusConflict, "sync already in progress for this hub")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to start sync")
		return
	}

	s.audit(r, "hub.sync", "hub", strconv.FormatInt(id, 10), "", "accepted")

	// Re-read so the response carries the syncing mark StartSync persisted.
	hub, err = s.db.GetHub(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload hub")
		return
	}
	writeData(w, http.StatusAccepted, toHubDTO(*hub))
}

// parseIDParam extracts an int64 URL parameter.
func parseIDParam(r *http.Request, name string) (int64, error) {
	raw := chi.URLParam(r, name)
	return strconv.ParseInt(raw, 10, 64)
}

// hubUpdateDetail summarizes which fields a hub update touched. The token is
// never written to the audit log — only that it was rotated.
func hubUpdateDetail(req updateHubRequest) string {
	fields := []string{}
	if req.Name != nil {
		fields = append(fields, "name")
	}
	if req.BaseURL != nil {
		fields = append(fields, "base_url")
	}
	if req.Token != nil {
		fields = append(fields, "token(rotated)")
	}
	return "fields=" + strings.Join(fields, ",")
}
