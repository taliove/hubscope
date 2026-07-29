package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/taliove/hubscope/internal/store"
)

// minIntervalSeconds is the smallest allowed per-endpoint interval override.
const minIntervalSeconds = 60

// patchEndpointRequest is the body for PATCH /api/endpoints/{id}. Both fields
// are optional. interval_seconds is a RawMessage so the handler can
// distinguish "absent" (leave unchanged) from an explicit null (clear the
// override and fall back to the global default).
type patchEndpointRequest struct {
	Enabled         *bool           `json:"enabled"`
	IntervalSeconds json.RawMessage `json:"interval_seconds"`
}

// handlePatchEndpoint handles PATCH /api/endpoints/{id}.
func (s *Server) handlePatchEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	if _, err := s.db.GetEndpoint(id); err != nil {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}

	var req patchEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	interval, err := parseIntervalPatch(req.IntervalSeconds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	endpoint, err := s.db.UpdateEndpoint(id, req.Enabled, interval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update endpoint")
		return
	}

	s.audit(r, "endpoint.patch", "endpoint", strconv.FormatInt(id, 10),
		endpointPatchDetail(req, interval), "success")
	s.InvalidateOverview()
	writeData(w, http.StatusOK, toEndpointDTO(*endpoint))
}

// handleDeleteEndpoint handles DELETE /api/endpoints/{id}. The endpoint's
// probe history and alert events are removed together with it.
func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	if _, err := s.db.GetEndpoint(id); err != nil {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}

	if err := s.db.DeleteEndpoint(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete endpoint")
		return
	}

	s.audit(r, "endpoint.delete", "endpoint", strconv.FormatInt(id, 10), "", "success")
	s.InvalidateOverview()
	writeNoContent(w)
}

// handlePruneDeadEndpoints handles POST /api/endpoints/prune-dead. It removes
// every disabled endpoint that never had a successful probe, with its
// history, and returns how many were pruned.
func (s *Server) handlePruneDeadEndpoints(w http.ResponseWriter, r *http.Request) {
	pruned, err := s.db.PruneDeadEndpoints()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prune dead endpoints")
		return
	}

	s.audit(r, "endpoint.prune_dead", "endpoint", "",
		fmt.Sprintf("pruned=%d", pruned), "success")
	if pruned > 0 {
		s.InvalidateOverview()
	}
	writeData(w, http.StatusOK, map[string]int64{"pruned": pruned})
}

// parseIntervalPatch converts the raw interval_seconds JSON value into a
// tri-state store.IntervalPatch: absent leaves the override unchanged, an
// explicit null clears it, and a number sets it.
func parseIntervalPatch(raw json.RawMessage) (store.IntervalPatch, error) {
	if raw == nil {
		return store.IntervalPatch{}, nil
	}
	if string(raw) == "null" {
		return store.IntervalPatch{Set: true}, nil
	}

	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return store.IntervalPatch{}, fmt.Errorf("interval_seconds must be a number or null")
	}
	if v < minIntervalSeconds {
		return store.IntervalPatch{}, fmt.Errorf("interval_seconds must be at least %d", minIntervalSeconds)
	}
	return store.IntervalPatch{Set: true, Value: &v}, nil
}

// endpointPatchDetail summarizes which fields an endpoint patch changed.
func endpointPatchDetail(req patchEndpointRequest, interval store.IntervalPatch) string {
	fields := []string{}
	if req.Enabled != nil {
		fields = append(fields, fmt.Sprintf("enabled=%v", *req.Enabled))
	}
	if interval.Set {
		if interval.Value == nil {
			fields = append(fields, "interval_seconds cleared")
		} else {
			fields = append(fields, fmt.Sprintf("interval_seconds=%d", *interval.Value))
		}
	}
	if len(fields) == 0 {
		return "no changes"
	}
	return strings.Join(fields, " ")
}
