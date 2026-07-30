package server

import (
	"net/http"
	"time"

	"github.com/taliove/hubscope/internal/store"
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

// handleListAlerts handles GET /api/alerts?limit=N. Public (read), scoped to
// the session's hub for non-super_admin. Hub-less events (score_drop, batch;
// no endpoint) are visible only to super_admin via the *All store variant.
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"))

	u := sessionUser(r)
	var events []store.AlertEvent
	var err error
	if u == nil || u.Role == store.RoleSuperAdmin {
		events, err = s.db.ListAlertEventsAll(limit)
	} else if u.HubID == nil {
		events = []store.AlertEvent{}
	} else {
		events, err = s.db.ListAlertEventsByHub(*u.HubID, limit)
	}
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
