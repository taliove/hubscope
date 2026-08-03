package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/taliove/hubscope/internal/store"
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

	// Cross-campaign mutex (GH #153): retrying on top of another active
	// campaign would stack a second cell pool on the Hub.
	active, err := s.db.HasUnfinishedCampaign()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check active campaigns")
		return
	}
	if active {
		writeError(w, http.StatusConflict, "an evaluation campaign is already running")
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

// maxRetryUnits bounds one retry-units request's item list: the units feed
// an OR-of-ANDs SQL fragment, so the cap keeps the query shape bounded.
const maxRetryUnits = 200

// retryUnitsRequest is POST /api/campaigns/{id}/retry-units's body: the
// explicit (model, case) units to re-evaluate, 1..maxRetryUnits items.
type retryUnitsRequest struct {
	Items []retryUnitItem `json:"items"`
}

type retryUnitItem struct {
	ModelDBID int64 `json:"model_db_id"`
	CaseID    int64 `json:"case_id"`
}

// retryUnitsResponse reports how many requested units were accepted for
// re-evaluation (currently null-score) versus skipped (already judged or
// unknown to the campaign — W7: judged results are never re-asked).
type retryUnitsResponse struct {
	Accepted int `json:"accepted"`
	Skipped  int `json:"skipped"`
}

// handleRetryCampaignUnits handles POST /api/campaigns/{id}/retry-units: the
// targeted sibling of retry-failed, re-evaluating an explicit set of
// (model, case) units instead of every failed result. The preconditions,
// state guards and execution path are the batch retry's own: only a settled
// (done/failed) campaign qualifies, hub-isolated per loadVisibleCampaign,
// the cross-campaign mutex (GH #153) applies, and the reopen is re-checked
// under the store's state guard (ReopenCampaignForUnitRetry) so a concurrent
// duplicate loses the race. Only units whose score is currently null are
// accepted — the rest are skipped and counted, never re-asked (W7). A
// request whose units are all already judged changes nothing and answers
// 200 with the counts; an accepted retry runs asynchronously (synchronous
// under WithSyncEval in tests) and settles through the shared
// SettleCampaign path, answering 202.
func (s *Server) handleRetryCampaignUnits(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}
	if campaign.Status != store.CampaignStatusDone && campaign.Status != store.CampaignStatusFailed {
		writeError(w, http.StatusConflict, "campaign is not settled")
		return
	}

	var req retryUnitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Items) == 0 || len(req.Items) > maxRetryUnits {
		writeError(w, http.StatusBadRequest, "items must contain 1 to 200 units")
		return
	}
	// Duplicate items name the same unit: collapse them so the counts and
	// the reopen's unit set reflect distinct units only.
	seen := make(map[store.RetryUnit]struct{}, len(req.Items))
	units := make([]store.RetryUnit, 0, len(req.Items))
	for _, item := range req.Items {
		if item.ModelDBID <= 0 || item.CaseID <= 0 {
			writeError(w, http.StatusBadRequest, "model_db_id and case_id must be positive")
			return
		}
		u := store.RetryUnit{ModelDBID: item.ModelDBID, CaseID: item.CaseID}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		units = append(units, u)
	}

	nullUnits, err := s.db.CampaignNullScoreUnits(campaign.ID, units)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect campaign results")
		return
	}
	accepted := make([]store.RetryUnit, 0, len(nullUnits))
	for _, u := range units {
		if _, ok := nullUnits[u]; ok {
			accepted = append(accepted, u)
		}
	}
	resp := retryUnitsResponse{Accepted: len(accepted), Skipped: len(units) - len(accepted)}
	if len(accepted) == 0 {
		// Every requested unit is already judged: nothing to re-run, no
		// state change — the counts are the whole answer.
		writeData(w, http.StatusOK, resp)
		return
	}

	// Cross-campaign mutex (GH #153): retrying on top of another active
	// campaign would stack a second cell pool on the Hub.
	active, err := s.db.HasUnfinishedCampaign()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check active campaigns")
		return
	}
	if active {
		writeError(w, http.StatusConflict, "an evaluation campaign is already running")
		return
	}

	reopened, err := s.db.ReopenCampaignForUnitRetry(campaign.ID, accepted)
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
		s.evaluator.RetryUnits(context.Background(), campaign.ID, accepted)
	}
	if s.syncEval {
		exec()
	} else {
		go exec()
	}

	s.audit(r, "eval.retry_units", "campaign", strconv.FormatInt(campaign.ID, 10),
		fmt.Sprintf("accepted=%d skipped=%d", resp.Accepted, resp.Skipped), "accepted")
	writeData(w, http.StatusAccepted, resp)
}

// handleCancelCampaign handles POST /api/campaigns/{id}/cancel (GH #152):
// stops the locally executing batch — in-flight cells run to completion,
// unstarted cells are dropped, their runs fail and the campaign settles
// failed through the normal machinery. Hub-isolated per
// loadVisibleCampaign; a settled campaign conflicts, a campaign not
// executing on this process (already canceled, or a stale running row from
// a crashed process) conflicts too.
func (s *Server) handleCancelCampaign(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}
	if campaign.Status == store.CampaignStatusDone || campaign.Status == store.CampaignStatusFailed {
		writeError(w, http.StatusConflict, "campaign is already settled")
		return
	}
	if !s.evaluator.CancelCampaign(campaign.ID) {
		writeError(w, http.StatusConflict, "campaign is not actively executing")
		return
	}
	s.audit(r, "eval.cancel", "campaign", strconv.FormatInt(campaign.ID, 10), "", "accepted")
	writeData(w, http.StatusAccepted, map[string]bool{"canceled": true})
}
