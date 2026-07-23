package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/taliove2009/hubscope/internal/store"
)

// shareTokenBytes is the raw entropy of a share-link token: 32 crypto/rand
// bytes (256 bits, above the 128-bit floor of ADR 0006), hex-encoded.
const shareTokenBytes = 32

// shareLinkNotFound is the single 404 message for every failed token lookup
// (unknown, malformed-shape, or revoked): one body, no enumeration oracle.
const shareLinkNotFound = "share link not found"

// shareLinkDTO is the API representation of a ShareLink. The token is the
// capability itself (W6): it is only ever returned to the session-gated
// management views, never through the public shared-report endpoint.
type shareLinkDTO struct {
	ID         int64   `json:"id"`
	Token      string  `json:"token"`
	CampaignID int64   `json:"campaign_id"`
	CreatedBy  string  `json:"created_by"`
	CreatedAt  string  `json:"created_at"`
	RevokedAt  *string `json:"revoked_at"`
}

// toShareLinkDTO maps a store.ShareLink to its API representation.
func toShareLinkDTO(l store.ShareLink) shareLinkDTO {
	return shareLinkDTO{
		ID:         l.ID,
		Token:      l.Token,
		CampaignID: l.CampaignID,
		CreatedBy:  l.CreatedBy,
		CreatedAt:  l.CreatedAt.Format(time.RFC3339),
		RevokedAt:  formatTimePtr(l.RevokedAt),
	}
}

// mintShareToken generates a fresh high-entropy share token.
func mintShareToken() (string, error) {
	buf := make([]byte, shareTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// handleCreateShareLink handles POST /api/campaigns/{id}/share-links: mints a
// token-gated read-only link onto the campaign's report (session required,
// audited per ADR 0006).
func (s *Server) handleCreateShareLink(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	if _, err := s.db.GetCampaign(id); err != nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	token, err := mintShareToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mint share token")
		return
	}
	link, err := s.db.CreateShareLink(token, id, actorOr(r), time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create share link")
		return
	}

	s.audit(r, "share_link.create", "share_link", strconv.FormatInt(link.ID, 10),
		fmt.Sprintf("campaign_id=%d", id), "success")
	writeData(w, http.StatusCreated, toShareLinkDTO(*link))
}

// handleListShareLinks handles GET /api/share-links: every link, newest
// first, for the admin management view.
func (s *Server) handleListShareLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.db.ListShareLinks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list share links")
		return
	}
	dtos := make([]shareLinkDTO, 0, len(links))
	for _, l := range links {
		dtos = append(dtos, toShareLinkDTO(l))
	}
	writeData(w, http.StatusOK, dtos)
}

// handleRevokeShareLink handles DELETE /api/share-links/{id}: stamps
// revoked_at (the row stays for audit), after which the token 404s like a
// wrong one. Revocation is idempotent; an unknown id is a plain 404.
func (s *Server) handleRevokeShareLink(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid share link id")
		return
	}
	found, err := s.db.RevokeShareLink(id, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke share link")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "share link not found")
		return
	}
	s.audit(r, "share_link.revoke", "share_link", strconv.FormatInt(id, 10), "", "success")
	writeNoContent(w)
}

// handleGetSharedReport handles GET /api/shared-reports/{token}: the public,
// session-free report view. The token is the only credential; an unknown or
// revoked token answers the identical 404 so existence cannot be probed.
func (s *Server) handleGetSharedReport(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	link, err := s.db.GetShareLinkByToken(token)
	if err != nil {
		// A database failure is not "link missing": merging them would hide
		// real outages behind the anti-enumeration 404.
		slog.Error("shared report: load share link", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if link == nil || link.RevokedAt != nil {
		writeError(w, http.StatusNotFound, shareLinkNotFound)
		return
	}
	campaign, err := s.db.GetCampaign(link.CampaignID)
	if err != nil {
		// A dangling campaign reference must not leak a different shape.
		writeError(w, http.StatusNotFound, shareLinkNotFound)
		return
	}
	s.writeCampaignReport(w, r, campaign, true)
}
