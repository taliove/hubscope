package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// liveFeedEntryDTO is one judged-case event of a campaign (issue #17): the
// console live feed's unit — model, suite and case identity, verdict
// method, the raw 0~1 score (null on judge failure; the 0-100 scaling is a
// frontend concern, ui-guidelines §7), latency and verdict time.
type liveFeedEntryDTO struct {
	ID          int64    `json:"id"`
	ModelID     string   `json:"model_id"`
	SuiteKey    string   `json:"suite_key"`
	SuiteName   string   `json:"suite_name"`
	CaseID      int64    `json:"case_id"`
	CasePrompt  string   `json:"case_prompt"`
	VerdictType string   `json:"verdict_type"`
	Score       *float64 `json:"score"`
	LatencyMs   int      `json:"latency_ms"`
	CreatedAt   string   `json:"created_at"`
}

// toLiveFeedEntryDTO maps a store.LiveFeedEntry to its API representation.
func toLiveFeedEntryDTO(e store.LiveFeedEntry) liveFeedEntryDTO {
	return liveFeedEntryDTO{
		ID:          e.ID,
		ModelID:     e.ModelID,
		SuiteKey:    e.SuiteKey,
		SuiteName:   e.SuiteName,
		CaseID:      e.CaseID,
		CasePrompt:  e.CasePrompt,
		VerdictType: e.VerdictType,
		Score:       e.Score,
		LatencyMs:   e.LatencyMs,
		CreatedAt:   e.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// loadVisibleCampaign resolves the campaign of the {id} path param and
// enforces the console-side visibility rule shared by the campaign read
// endpoints: session-gated (the routes never enter publicReadPattern) and
// hub-isolated with the campaigns list reachability rule — a hub-scoped
// session only sees campaigns whose membership includes one of its hub's
// models, and anything else answers the same 404 as an unknown campaign (no
// enumeration oracle). On failure it writes the error and returns false.
func (s *Server) loadVisibleCampaign(w http.ResponseWriter, r *http.Request) (*store.Campaign, bool) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return nil, false
	}
	campaign, err := s.db.GetCampaign(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return nil, false
	}

	u := sessionUser(r)
	if u != nil && u.Role != store.RoleSuperAdmin {
		// A hub-scoped role without a hub_id is a data inconsistency; treat
		// it as "not visible" rather than leaking the stream (same fallback
		// as the campaigns list's empty result).
		if u.HubID == nil {
			writeError(w, http.StatusNotFound, "campaign not found")
			return nil, false
		}
		visible, err := s.db.CampaignVisibleToHub(id, *u.HubID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check campaign scope")
			return nil, false
		}
		if !visible {
			writeError(w, http.StatusNotFound, "campaign not found")
			return nil, false
		}
	}
	return campaign, true
}

// handleGetCampaignLiveFeed handles GET /api/campaigns/{id}/live-feed: the
// cursor-pulled judged-case event stream of one campaign (issue #17),
// console-only — visibility per loadVisibleCampaign.
// since_id is exclusive (default 0); limit follows the probes caliber
// (default 50, cap 200). Entries come back ascending by id, an empty
// increment as an empty array.
func (s *Server) handleGetCampaignLiveFeed(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}

	sinceID, err := parseSinceID(r.URL.Query().Get("since_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "since_id must be a non-negative integer")
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"))

	entries, err := s.db.ListCampaignLiveFeed(campaign.ID, sinceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load live feed")
		return
	}
	dtos := make([]liveFeedEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, toLiveFeedEntryDTO(e))
	}
	writeData(w, http.StatusOK, dtos)
}

// liveFeedResultDTO is the on-demand expansion of one live-feed event
// (GH #41): the case's full prompt, the expectation by verdict method (rule
// cases get the standard answer, judge cases the rubric scoring points —
// the frontend labels the block from verdict_type with the same fork), the
// model's answer text, the raw 0~1 score and the verdict detail. Nulls
// survive as JSON nulls (no answer record, unscored, purged case).
type liveFeedResultDTO struct {
	ID            int64    `json:"id"`
	CasePrompt    string   `json:"case_prompt"`
	VerdictType   string   `json:"verdict_type"`
	Expected      *string  `json:"expected"`
	AnswerText    *string  `json:"answer_text"`
	Score         *float64 `json:"score"`
	VerdictDetail *string  `json:"verdict_detail"`
}

// handleGetCampaignLiveFeedResult handles GET
// /api/campaigns/{id}/live-feed/{resultID}: the console-only per-result
// detail behind the feed's row expansion (GH #41). Visibility follows the
// feed list exactly (loadVisibleCampaign); a result that does not exist or
// belongs to another campaign answers the same 404 (no cross-campaign
// oracle). The endpoint never enters the shared/public surface — the
// expectation fields are question-bank content (spec 0004 / W7).
func (s *Server) handleGetCampaignLiveFeedResult(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}
	resultID, err := parseIDParam(r, "resultID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid result id")
		return
	}
	d, err := s.db.GetCampaignLiveFeedResult(campaign.ID, resultID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "result not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load result detail")
		return
	}
	writeData(w, http.StatusOK, liveFeedResultDTO{
		ID:            d.ID,
		CasePrompt:    d.CasePrompt,
		VerdictType:   d.VerdictType,
		Expected:      d.Expected,
		AnswerText:    d.AnswerText,
		Score:         d.Score,
		VerdictDetail: d.VerdictDetail,
	})
}

// parseSinceID parses the exclusive live-feed cursor: empty means 0 (from
// the beginning); anything non-numeric or negative is a client error.
func parseSinceID(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid since_id %q", raw)
	}
	return n, nil
}
