package server

import (
	"net/http"
	"strconv"
)

// probeRoundResponse is the payload for POST /api/endpoints/{id}/probe.
type probeRoundResponse struct {
	EndpointID int64      `json:"endpoint_id"`
	Results    []probeDTO `json:"results"`
}

// handleProbeEndpoint handles POST /api/endpoints/{id}/probe. Runs one round
// (non-streaming then streaming) synchronously and returns both records.
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
	for _, p := range probes {
		results = append(results, toProbeDTO(p))
	}

	writeData(w, http.StatusOK, probeRoundResponse{
		EndpointID: id,
		Results:    results,
	})
}

// handleListProbes handles GET /api/endpoints/{id}/probes?limit=N.
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

	limit := parseLimit(r.URL.Query().Get("limit"))

	probes, err := s.db.ListProbes(id, limit)
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
