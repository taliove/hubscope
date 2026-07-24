package server

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/taliove/hubscope/internal/store"
)

// handleRunDiscovery handles POST /api/discovery/run. It synchronously runs
// one full model-discovery pass over all hubs and returns the aggregated
// stats. Every synced hub registers a discovery_sync task in the task center.
func (s *Server) handleRunDiscovery(w http.ResponseWriter, r *http.Request) {
	stats, err := s.discovery.Sync(r.Context(), store.TaskSourceManual)
	if err != nil {
		slog.Error("discovery run failed", "error", err)
		writeError(w, http.StatusInternalServerError, "discovery sync failed")
		return
	}
	s.audit(r, "discovery.run", "discovery", "",
		fmt.Sprintf("added=%d updated=%d retired=%d endpoints_created=%d", stats.Added, stats.Updated, stats.Retired, stats.EndpointsCreated), "success")
	writeData(w, http.StatusOK, stats)
}
