package server

import (
	"net/http"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// alertEventDTO is the API representation of an AlertEvent. GroupKey is
// non-null only on vendor group alerts (group_down / group_recovered,
// spec 0017 ticket 3), where it carries the family name. CampaignID is
// non-null only on eval-batch alerts (score_drop / score_drop_skipped,
// GH #156), where it deep-links the reported batch.
type alertEventDTO struct {
	ID         int64   `json:"id"`
	EndpointID *int64  `json:"endpoint_id"`
	Kind       string  `json:"kind"`
	Message    string  `json:"message"`
	SentOK     bool    `json:"sent_ok"`
	CreatedAt  string  `json:"created_at"`
	GroupKey   *string `json:"group_key"`
	CampaignID *int64  `json:"campaign_id"`
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
		GroupKey:   e.GroupKey,
		CampaignID: e.CampaignID,
	}
}

// handleListAlerts handles GET /api/alerts?limit=N. Anonymous callers get
// the public four-kind whitelist view (publicAlertKinds, spec 0019 —
// incident narrative only, global scope, filtered in the store query so the
// limit window is not diluted by hidden kinds). Authenticated callers:
// super_admin sees everything including hub-less events (score_drop, batch;
// no endpoint) via the *All store variant, a hub-scoped user sees their
// hub's endpoint-bound events, and a user with no hub sees an empty list.
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"))

	u := sessionUser(r)
	var events []store.AlertEvent
	var err error
	switch {
	case u == nil:
		events, err = s.db.ListAlertEventsByKinds(limit, publicAlertKinds)
	case u.Role == store.RoleSuperAdmin:
		events, err = s.db.ListAlertEventsAll(limit)
	case u.HubID == nil:
		events = []store.AlertEvent{}
	default:
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
