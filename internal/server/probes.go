package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// probeRoundResponse is the payload for POST /api/endpoints/{id}/probe.
type probeRoundResponse struct {
	EndpointID int64      `json:"endpoint_id"`
	Results    []probeDTO `json:"results"`
}

// handleProbeEndpoint handles POST /api/endpoints/{id}/probe. Runs one round
// synchronously and returns its records: non-streaming then streaming for
// chat protocols, a single record for image protocols (no streaming or TTFT
// concept, spec 0014).
func (s *Server) handleProbeEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	if _, err := s.db.GetEndpoint(id); err != nil {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}

	probes, err := s.prober.RunRound(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to run probe")
		return
	}

	results := make([]probeDTO, 0, len(probes))
	okCount := 0
	for _, p := range probes {
		if p.OK {
			okCount++
		}
		results = append(results, toProbeDTO(p))
	}

	s.audit(r, "endpoint.probe", "endpoint", strconv.FormatInt(id, 10),
		fmt.Sprintf("ok=%d/%d", okCount, len(probes)), "success")
	writeData(w, http.StatusOK, probeRoundResponse{
		EndpointID: id,
		Results:    results,
	})
}

// handleListProbes handles GET /api/endpoints/{id}/probes?limit=N&ok=BOOL&hours=H.
// hours (2026-07-30, quick-view latency-detail curve) opens a time window:
// records with created_at >= now-hours, capped at 2000 rows (store
// maxProbeWindowLimit); when both hours and limit are given, hours wins.
func (s *Server) handleListProbes(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	if _, err := s.db.GetEndpoint(id); err != nil {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}

	okFilter, err := parseOKFilter(r.URL.Query().Get("ok"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hours, err := parseHours(r.URL.Query().Get("hours"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var probes []store.Probe
	if hours > 0 {
		since := s.now().Add(-time.Duration(hours) * time.Hour)
		probes, err = s.db.ListProbesSince(id, since, okFilter)
	} else {
		limit := parseLimit(r.URL.Query().Get("limit"))
		probes, err = s.db.ListProbes(id, limit, okFilter)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list probes")
		return
	}

	dtos := make([]probeDTO, 0, len(probes))
	for _, p := range probes {
		dtos = append(dtos, toProbeDTO(p))
	}

	writeData(w, http.StatusOK, dtos)
}

// parseOKFilter validates the optional ok query parameter: "true" or "false"
// select one probe kind, an absent parameter returns both.
func parseOKFilter(raw string) (*bool, error) {
	switch raw {
	case "":
		return nil, nil
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	default:
		return nil, fmt.Errorf("ok must be true or false")
	}
}

// parseHours validates the optional hours query parameter: a positive integer
// hour window. Absent returns 0 (window mode off, the limit path applies);
// anything else that is not a positive integer is rejected with a 400 (same
// caliber as the ok filter).
func parseHours(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("hours must be a positive integer")
	}
	return n, nil
}

// parseLimit clamps the limit query param to [1, 200] with a default of 50.
func parseLimit(raw string) int {
	if raw == "" {
		return 50
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}
