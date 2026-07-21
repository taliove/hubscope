package server

import (
	"net/http"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// alertEventDTO is the API representation of an AlertEvent.
type alertEventDTO struct {
	ID         int64  `json:"id"`
	EndpointID *int64 `json:"endpoint_id"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	SentOK     bool   `json:"sent_ok"`
	CreatedAt  string `json:"created_at"`
}

// toAlertEventDTO maps a store.AlertEvent to its API representation.
func toAlertEventDTO(e store.AlertEvent) alertEventDTO {
	return alertEventDTO{
		ID:         e.ID,
		EndpointID: e.EndpointID,
		Kind:       e.Kind,
		Message:    e.Message,
		SentOK:     e.SentOK,
		CreatedAt:  e.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// handleListAlerts handles GET /api/alerts?limit=N. Public (read).
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"))

	events, err := s.db.ListAlertEvents(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list alert events")
		return
	}

	dtos := make([]alertEventDTO, 0, len(events))
	for _, e := range events {
		dtos = append(dtos, toAlertEventDTO(e))
	}
	writeData(w, http.StatusOK, dtos)
}
