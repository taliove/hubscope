package server

import (
	"context"
	"net/http"
	"strconv"
)

// handleRetryCampaignFailed handles POST /api/campaigns/{id}/retry-failed
// (GH #28): re-evaluate a settled batch's failed (null-score) results. Only
// done/failed campaigns with at least one null-score result qualify —
// anything else conflicts; the preconditions are re-checked under the store's
// state guard (ReopenCampaignForRetry) so a concurrent duplicate request
// loses the race instead of double-firing a retry. The reopen also migrates
// every run holding failed results back to running in the same transaction,
// and the executor finishes each of them done/failed when its retried cells
// complete (GH #39) — the progress grid therefore shows the in-flight retry
// instead of the stale terminal state, and the batch re-settles from the
// runs' fresh statuses. Execution is asynchronous (synchronous under
// WithSyncEval in tests), runs through the same bounded (run × model) cell
// pool as a normal batch, and settles through the shared SettleCampaign
// path — the progress grid, live feed and settle transition need no
// retry-specific handling.
func (s *Server) handleRetryCampaignFailed(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	campaign, err := s.db.GetCampaign(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	if campaign.Status != "done" && campaign.Status != "failed" {
		writeError(w, http.StatusConflict, "campaign is not settled")
		return
	}

	nulls, err := s.db.CountCampaignNullScoreResults(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect campaign results")
		return
	}
	if nulls == 0 {
		writeError(w, http.StatusConflict, "campaign has no failed results to retry")
		return
	}

	reopened, err := s.db.ReopenCampaignForRetry(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reopen campaign")
		return
	}
	if !reopened {
		// Lost a concurrent retry race: the other request owns the rerun.
		writeError(w, http.StatusConflict, "campaign is not settled")
		return
	}

	exec := func() {
		s.evaluator.RetryFailedResults(context.Background(), id)
	}
	if s.syncEval {
		exec()
	} else {
		go exec()
	}

	s.audit(r, "eval.retry_failed", "campaign", strconv.FormatInt(id, 10),
		"", "accepted")
	s.writeCampaignCreated(w, id)
}
