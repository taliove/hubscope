package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// handleFullSweep is the "one-click full evaluation" (POST /api/evals
// without a suite_id): one campaign covering every suite in the evaluation
// rotation (retired suites are excluded, ADR 0010), one run per suite, over
// all active chat-capable models (non_chat and retired models are excluded
// by construction). Runs execute sequentially inside RunCampaign, keeping
// hub load predictable.
func (s *Server) handleFullSweep(w http.ResponseWriter, r *http.Request, judgeModel string) {
	modelIDs, err := s.db.ListActiveChatModelIDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	if len(modelIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no active chat-capable models to evaluate")
		return
	}

	suites, err := s.db.ListEnabledSuites()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list suites")
		return
	}

	campaign, err := s.db.CreateCampaign("manual", modelIDs, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create campaign")
		return
	}

	// Detached context: the sweep outlives the request; state is polled.
	// Tests may force synchronous execution via WithSyncEval.
	if s.syncEval {
		s.evaluator.RunCampaign(context.Background(), campaign.ID, "manual", suites, modelIDs, judgeModel)
	} else {
		go s.evaluator.RunCampaign(context.Background(), campaign.ID, "manual", suites, modelIDs, judgeModel)
	}

	s.audit(r, "eval.create", "campaign", strconv.FormatInt(campaign.ID, 10),
		fmt.Sprintf("full sweep suites=%d models=%d judge=%q", len(suites), len(modelIDs), judgeModel), "accepted")
	s.writeCampaignCreated(w, campaign.ID)
}

// writeCampaignCreated responds 202 with the freshly created campaign and
// its (possibly still empty) run list.
func (s *Server) writeCampaignCreated(w http.ResponseWriter, campaignID int64) {
	campaign, err := s.db.GetCampaign(campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign")
		return
	}
	runs, err := s.db.ListEvalRunsByCampaign(campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign runs")
		return
	}
	detail, err := s.buildCampaignDetail(*campaign, runs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign results")
		return
	}
	writeData(w, http.StatusAccepted, detail)
}

// handleListCampaigns handles GET /api/campaigns: every campaign reachable
// from the session's hub scope, newest first, each with its aggregated
// run-progress counts.
func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	var campaigns []store.CampaignWithProgress
	var err error
	if u == nil || u.Role == store.RoleSuperAdmin {
		campaigns, err = s.db.ListCampaignsAll()
	} else if u.HubID == nil {
		// A hub-scoped role without a hub_id is a data inconsistency; fall
		// back to an empty result rather than leaking the full set.
		campaigns = []store.CampaignWithProgress{}
	} else {
		campaigns, err = s.db.ListCampaignsByHub(*u.HubID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list campaigns")
		return
	}

	dtos := make([]campaignDTO, 0, len(campaigns))
	for _, c := range campaigns {
		dtos = append(dtos, toCampaignDTO(c))
	}
	if err := s.fillActiveCampaignUnits(dtos); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign unit progress")
		return
	}
	writeData(w, http.StatusOK, dtos)
}

// fillActiveCampaignUnits attaches the judged-unit aggregate to every
// active campaign in the list (2026-08-03 sidebar pill caliber): runs
// settle late under model-major feeding, so the live progress of a running
// batch is judged cases over expected cases. The GH #153 mutex means at
// most one campaign is active, so the extra aggregate query is singular.
func (s *Server) fillActiveCampaignUnits(dtos []campaignDTO) error {
	for i := range dtos {
		if dtos[i].Status != store.CampaignStatusRunning && dtos[i].Status != store.CampaignStatusPending {
			continue
		}
		runs, err := s.db.ListEvalRunsByCampaign(dtos[i].ID)
		if err != nil {
			return err
		}
		runIDs := make([]int64, len(runs))
		for j, run := range runs {
			runIDs[j] = run.ID
		}
		units, err := s.db.GetRunUnitProgress(runIDs)
		if err != nil {
			return err
		}
		for _, u := range units {
			dtos[i].Progress.JudgedUnits += u.DoneUnits
			dtos[i].Progress.ExpectedUnits += u.TotalUnits
		}
	}
	return nil
}

// handleGetCampaign handles GET /api/campaigns/{id}: the campaign, its
// progress aggregate, and its member runs with per-run aggregate scores.
// Hub-isolated per loadVisibleCampaign (GH #149).
func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	campaign, ok := s.loadVisibleCampaign(w, r)
	if !ok {
		return
	}
	id := campaign.ID

	runs, err := s.db.ListEvalRunsByCampaign(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign runs")
		return
	}

	detail, err := s.buildCampaignDetail(*campaign, runs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign results")
		return
	}
	writeData(w, http.StatusOK, detail)
}

// buildCampaignDetail assembles the detail DTO for one campaign: live
// progress counts plus each member run's read-time aggregate score.
func (s *Server) buildCampaignDetail(campaign store.Campaign, runs []store.EvalRun) (campaignDetailDTO, error) {
	runIDs := make([]int64, len(runs))
	for i, run := range runs {
		runIDs[i] = run.ID
	}
	// One aggregate query for every run's mean score (GH #151), not one
	// detail-row fetch per run.
	avgs, err := s.db.ListEvalRunAverageScores(runIDs)
	if err != nil {
		return campaignDetailDTO{}, err
	}
	// Failure reasons ride along: a failed batch must show why (2026-08-05).
	failReasons, err := s.db.LastErrorLogByRun(runIDs)
	if err != nil {
		return campaignDetailDTO{}, err
	}

	withProgress := store.CampaignWithProgress{Campaign: campaign}
	runDTOs := make([]evalRunDTO, 0, len(runs))
	for _, run := range runs {
		withProgress.Progress.Total++
		switch run.Status {
		case "done":
			withProgress.Progress.Done++
		case "failed":
			withProgress.Progress.Failed++
		default:
			withProgress.Progress.Running++
		}
		dto := toEvalRunDTO(run, runScoreFromAvg(avgs, run))
		dto.FailureReason = failReasons[run.ID]
		runDTOs = append(runDTOs, dto)
	}
	return campaignDetailDTO{
		campaignDTO: toCampaignDTO(withProgress),
		Runs:        runDTOs,
	}, nil
}
