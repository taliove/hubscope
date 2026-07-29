package server

import (
	"net/http"
)

// publicEvalBoardDTO is GET /api/public/eval/board (spec 0010): the newest
// settled campaign's report — the exact same shape as
// /api/campaigns/{id}/report so the frontend consumes both isomorphically —
// plus a flag telling whether another batch is currently in flight. The
// information level is identical to the token-gated shared report
// (/report/:token): evaluation conclusions of settled batches, no tasks, no
// trigger tokens, nothing session-grade. The leaderboard is a public
// product page; the endpoint never accepts sort/family params (the client
// ranks and filters the one full payload) and exposes no write surface.
type publicEvalBoardDTO struct {
	Report  *campaignReportDTO `json:"report"`
	Running bool               `json:"running"`
}

// handleGetPublicEvalBoard handles GET /api/public/eval/board. Anonymous by
// design (the path is whitelisted in publicReadPattern, same mechanism as
// the status board and the shared report). A nil report means no campaign
// has settled yet — the public page renders its empty state.
func (s *Server) handleGetPublicEvalBoard(w http.ResponseWriter, r *http.Request) {
	campaign, err := s.db.LatestSettledCampaign()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the latest settled campaign")
		return
	}
	running, err := s.db.HasUnfinishedCampaign()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check unfinished campaigns")
		return
	}
	if campaign == nil {
		writeData(w, http.StatusOK, publicEvalBoardDTO{Report: nil, Running: running})
		return
	}
	// The endpoint takes no sort/family params (spec 0010): strip any
	// incoming query so the shared builder always serves the default board
	// (total-desc, unfiltered) — an anonymous ?family=x&sort=y must not
	// take effect.
	qr := r.Clone(r.Context())
	qr.URL.RawQuery = ""
	report, berr := s.buildCampaignReport(qr, campaign, false, false)
	if berr != nil {
		writeError(w, berr.status, berr.msg)
		return
	}
	writeData(w, http.StatusOK, publicEvalBoardDTO{Report: &report, Running: running})
}
