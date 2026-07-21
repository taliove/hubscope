package server

import (
	"log"
	"net/http"
)

// handleRunDiscovery handles POST /api/discovery/run. It synchronously runs
// one full model-discovery pass over all hubs and returns the aggregated
// stats.
func (s *Server) handleRunDiscovery(w http.ResponseWriter, r *http.Request) {
	stats, err := s.discovery.Sync(r.Context())
	if err != nil {
		log.Printf("discovery run: %v", err)
		writeError(w, http.StatusInternalServerError, "discovery sync failed")
		return
	}
	writeData(w, http.StatusOK, stats)
}
