package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/taliove/hubscope/internal/classifier"
	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/store"
)

// createModelRequest is the body for POST /api/models.
type createModelRequest struct {
	HubID   int64  `json:"hub_id"`
	ModelID string `json:"model_id"`
}

// modelProtocols lists the chat hub API protocols in canonical endpoint
// order. Chat models are trialed on them; image and video models are
// trial-free (see trialProtocolsFor).
var modelProtocols = []string{"anthropic", "openai"}

// trialProtocolsFor returns the protocols a manually registered model gets
// endpoints for, split by capability: chat models are trial-probed on
// anthropic/openai; image models get images_generation and images_edit
// endpoints without trial; video models get video_generation without trial
// (GH #100, spec 0018 T2: trial-free creation, same discipline as discovery
// sync). On a rule-read failure the chat-only list is returned: a missed
// image/video trial-free path is backfilled by the next discovery sync.
func (s *Server) trialProtocolsFor(modelID string) []string {
	rules, err := s.db.ListClassificationRules()
	if err != nil {
		slog.Error("trial protocols: load classification rules, trialing chat protocols only", "error", err)
		return append([]string(nil), modelProtocols...)
	}
	capability, _ := classifier.Classify(modelID, rules)
	return hubclient.ProtocolsForCapability(capability)
}

// handleCreateModel handles POST /api/models. Chat models are trial-probed
// on their candidate protocols first: an endpoint is created per protocol that
// answered. Image and video models get their endpoints created without trial
// (GH #100, spec 0018 T2: trial-free creation, same discipline as discovery
// sync). A chat model unreachable on all protocols is rejected with 400 and
// nothing is stored; an image/video model always succeeds (trial-free).
func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.HubID == 0 || req.ModelID == "" {
		writeError(w, http.StatusBadRequest, "hub_id and model_id are required")
		return
	}

	hub, err := s.db.GetHub(req.HubID)
	if err != nil {
		writeError(w, http.StatusNotFound, "hub not found")
		return
	}

	protocols := s.trialProtocolsFor(req.ModelID)
	working, failures := s.trialProtocols(r.Context(), *hub, req.ModelID, protocols)
	if len(working) == 0 {
		s.audit(r, "model.create", "model", req.ModelID, failures, "failed: unreachable on all trialed protocols")
		writeError(w, http.StatusBadRequest,
			"model is unreachable on all trialed protocols ("+strings.Join(protocols, ", ")+"): "+failures)
		return
	}

	model, err := s.db.CreateModel(req.HubID, req.ModelID, working)
	if err != nil {
		if isUniqueViolation(err) {
			s.audit(r, "model.create", "model", req.ModelID, "", "failed: duplicate model_id")
			writeError(w, http.StatusConflict, "model_id already exists for this hub")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create model")
		return
	}

	endpoints, err := s.db.ListEndpointsByModelID(model.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load endpoints")
		return
	}

	s.audit(r, "model.create", "model", strconv.FormatInt(model.ID, 10),
		fmt.Sprintf("model_id=%q hub_id=%d protocols=%v capability=%s family=%s",
			model.ModelID, model.HubID, working, model.Capability, model.Family), "success")
	s.InvalidateOverview()
	writeData(w, http.StatusCreated, toModelDTO(*model, endpoints))
}

// trialProtocols probes the model on the given protocols and returns the
// protocols that answered plus a human-readable summary of the failures.
// Trial-free protocols (image/video) are returned without probe calls
// (GH #100, spec 0018 T2); chat protocols go through the probe flow. Image
// protocol trials carry the rule-merged extra parameters (GH #33) via the
// single resolution entry (store.ImageParamsFor); a rules-table hiccup
// degrades the trial to the minimal request body rather than failing it.
func (s *Server) trialProtocols(ctx context.Context, hub store.Hub, modelID string, protocols []string) ([]string, string) {
	client := hubclient.New()
	working := []string{}
	failures := []string{}
	for _, protocol := range protocols {
		// Trial-free protocols (image/video) are returned without probe calls.
		if hubclient.IsTrialFreeProtocol(protocol) {
			working = append(working, protocol)
			continue
		}
		// Chat protocols: trial-probe before accepting.
		var imageParams map[string]string
		if hubclient.IsImageProtocol(protocol) {
			if params, err := s.db.ImageParamsFor(modelID); err != nil {
				slog.Warn("models: image param rules unavailable, trialing with minimal body",
					"model", modelID, "protocol", protocol, "error", err)
			} else {
				imageParams = params
			}
		}
		result := client.Probe(ctx, hub.BaseURL, hub.Token, protocol, modelID, false, imageParams)
		if result.OK {
			working = append(working, protocol)
			continue
		}
		reason := fmt.Sprintf("%s: HTTP %d", protocol, result.HTTPStatus)
		if result.ErrorSummary != nil {
			reason = fmt.Sprintf("%s: %s", protocol, *result.ErrorSummary)
		}
		failures = append(failures, reason)
	}
	return working, strings.Join(failures, "; ")
}

// handleListModels handles GET /api/models. Includes endpoints per model.
// Per the per-hub query isolation invariant (spec 0005): an anonymous caller
// is blocked at requireSession (/api/models is not in publicReadPattern), so
// a non-nil session here is guaranteed. A non-super_admin session only sees
// its own hub's models; super_admin sees all.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.listModelsForRequest(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}

	dtos := make([]modelDTO, 0, len(models))
	for _, m := range models {
		endpoints, err := s.db.ListEndpointsByModelID(m.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load endpoints")
			return
		}
		dtos = append(dtos, toModelDTO(m, endpoints))
	}

	writeData(w, http.StatusOK, dtos)
}

// listModelsForRequest selects models scoped to the request's session: all
// models for super_admin (and the defensive anonymous case, which
// requireSession only permits for public-read routes — /api/models is not one,
// so this branch is unreachable for anonymous in practice), or the session
// user's hub for hub-scoped roles.
func (s *Server) listModelsForRequest(r *http.Request) ([]store.Model, error) {
	return s.listModelsForScope(overviewScopeKey(r))
}

// listModelsForScope selects the models of one overview scope (the same
// selection listModelsForRequest makes, keyed for the snapshot cache).
func (s *Server) listModelsForScope(scope int64) ([]store.Model, error) {
	switch scope {
	case overviewScopeAll:
		return s.db.ListModelsAll()
	case overviewScopeEmpty:
		// A hub-scoped role without a hub_id is a data inconsistency; fall
		// back to an empty result rather than leaking the full set.
		return []store.Model{}, nil
	default:
		return s.db.ListModelsByHub(scope)
	}
}

// patchModelRequest is the body for PATCH /api/models/{id}. capability is
// editable per spec 0018 T7 (changing it triggers endpoint set
// reconciliation); eval_enabled is the GH #170 "join evaluations" switch.
// At least one field must be present.
type patchModelRequest struct {
	Capability  *string `json:"capability"`
	EvalEnabled *bool   `json:"eval_enabled"`
}

// validCapabilities is the set of capabilities accepted by PATCH /api/models/{id}.
var validCapabilities = map[string]bool{"chat": true, "image": true, "video": true}

// handlePatchModel handles PATCH /api/models/{id}. It accepts two
// independent fields (at least one required): eval_enabled, the GH #170
// "join evaluations" switch, a plain flip; and capability, whose change
// reconciles the endpoint set — protocols the new capability implies but
// are missing get created (chat trials, image/video trial-free), protocols
// the old capability had but the new one does not get disabled (history
// preserved). Spec 0018 T7 (GH #105).
func (s *Server) handlePatchModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid model id")
		return
	}

	model, err := s.db.GetModel(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	var req patchModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Capability == nil && req.EvalEnabled == nil {
		writeError(w, http.StatusBadRequest, "at least one of capability or eval_enabled is required")
		return
	}

	auditDetails := []string{}

	// GH #170: the eval switch is a plain flip. It only narrows the
	// candidate set read at batch creation, so it affects batches
	// triggered after the change; members of a running batch were
	// snapshotted when it was created.
	if req.EvalEnabled != nil {
		if err := s.db.SetModelEvalEnabled(model.ID, *req.EvalEnabled); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update eval_enabled")
			return
		}
		if *req.EvalEnabled != model.EvalEnabled {
			auditDetails = append(auditDetails, fmt.Sprintf("eval_enabled %t→%t", model.EvalEnabled, *req.EvalEnabled))
		}
		model.EvalEnabled = *req.EvalEnabled
	}

	if req.Capability != nil {
		newCap := strings.TrimSpace(*req.Capability)
		if !validCapabilities[newCap] {
			writeError(w, http.StatusBadRequest, "capability must be one of: chat, image, video")
			return
		}
		// Note: no early return when newCap == model.Capability. Reconciliation is
		// idempotent, and a same-capability PATCH is the repair path for legacy
		// endpoint drift — e.g. pre-GH #100 image models still holding surplus
		// chat endpoints (which keep burning probe requests). One click fixes it.

		oldCap := model.Capability
		if err := s.db.SetModelCapability(model.ID, newCap); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update capability")
			return
		}
		model.Capability = newCap

		// Reconcile endpoint set: create missing, disable surplus.
		newProtocols := hubclient.ProtocolsForCapability(newCap)

		existing, err := s.db.ListEndpointsByModelID(model.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load endpoints")
			return
		}
		have := make(map[string]bool, len(existing))
		for _, ep := range existing {
			have[ep.Protocol] = true
		}

		// Create missing endpoints for the new capability's protocols.
		missing := []string{}
		for _, protocol := range newProtocols {
			if !have[protocol] {
				missing = append(missing, protocol)
			}
		}
		created := []string{}
		if len(missing) > 0 {
			// Chat protocols need trial; image/video are trial-free.
			chatSet := make(map[string]bool)
			for _, p := range modelProtocols {
				chatSet[p] = true
			}
			trialNeeded := []string{}
			trialFree := []string{}
			for _, p := range missing {
				if chatSet[p] {
					trialNeeded = append(trialNeeded, p)
				} else {
					trialFree = append(trialFree, p)
				}
			}

			// Trial chat protocols (if any) — image/video are trial-free.
			if len(trialNeeded) > 0 {
				hub, err := s.db.GetHub(model.HubID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "failed to load hub")
					return
				}
				working, failures := s.trialProtocols(r.Context(), *hub, model.ModelID, trialNeeded)
				for _, protocol := range working {
					ep, isNew, err := s.db.CreateEndpoint(model.ID, protocol, true)
					if err != nil {
						slog.Error("model patch: create endpoint", "model_id", model.ID, "protocol", protocol, "error", err)
						continue
					}
					if isNew {
						_ = s.db.ApplyCreationDefaults(ep.ID, protocol)
						created = append(created, protocol)
					}
				}
				_ = failures // trial failures are not fatal — model still saved
			}

			// Trial-free: directly create image/video endpoints.
			for _, protocol := range trialFree {
				ep, isNew, err := s.db.CreateEndpoint(model.ID, protocol, true)
				if err != nil {
					slog.Error("model patch: create trial-free endpoint", "model_id", model.ID, "protocol", protocol, "error", err)
					continue
				}
				if isNew {
					_ = s.db.ApplyCreationDefaults(ep.ID, protocol)
					created = append(created, protocol)
				}
			}
		}

		// Disable surplus endpoints: protocols the old capability had but the
		// new one does not (history preserved, not deleted).
		disabled := []string{}
		for _, ep := range existing {
			stillNeeded := false
			for _, p := range newProtocols {
				if ep.Protocol == p {
					stillNeeded = true
					break
				}
			}
			if !stillNeeded && ep.Enabled {
				if _, err := s.db.SetEndpointEnabled(ep.ID, false); err != nil {
					slog.Error("model patch: disable surplus endpoint", "endpoint_id", ep.ID, "error", err)
					continue
				}
				disabled = append(disabled, ep.Protocol)
			}
		}

		auditDetails = append(auditDetails,
			fmt.Sprintf("capability %s→%s created=%v disabled=%v", oldCap, newCap, created, disabled))
	}

	endpoints, err := s.db.ListEndpointsByModelID(model.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load endpoints")
		return
	}

	if len(auditDetails) == 0 {
		auditDetails = append(auditDetails, "no change")
	}
	s.audit(r, "model.patch", "model", strconv.FormatInt(model.ID, 10),
		fmt.Sprintf("model_id=%q %s", model.ModelID, strings.Join(auditDetails, " ")), "success")
	s.InvalidateOverview()
	writeData(w, http.StatusOK, toModelDTO(*model, endpoints))
}

// handleDeleteModel handles DELETE /api/models/{id}. Only manual models can
// be deleted (a discovered one would be resurrected by the next sync); the
// model's endpoints and their history are removed together with it.
func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid model id")
		return
	}

	if _, err := s.db.GetModel(id); err != nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	if err := s.db.DeleteModel(id); err != nil {
		if errors.Is(err, store.ErrModelNotManual) {
			s.audit(r, "model.delete", "model", strconv.FormatInt(id, 10), "", "failed: discovered model")
			writeError(w, http.StatusConflict, "discovered models cannot be deleted; disable their endpoints instead")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete model")
		return
	}

	s.audit(r, "model.delete", "model", strconv.FormatInt(id, 10), "", "success")
	s.InvalidateOverview()
	writeNoContent(w)
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

// trialResultDTO is the response of POST /api/models/{id}/trial.
type trialResultDTO struct {
	Model            modelDTO `json:"model"`
	CreatedProtocols []string `json:"created_protocols"`
	// Failures summarizes the failed trials ("" when nothing failed).
	Failures string `json:"failures"`
}

// handleTrialModel handles POST /api/models/{id}/trial. It re-runs the
// protocol trial for the protocols the model has no endpoint for and creates
// an enabled endpoint per protocol that answered (W3: only working protocols
// get an endpoint). Protocols that already have an endpoint are not
// re-probed, so the call is idempotent for a fully-covered model.
func (s *Server) handleTrialModel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid model id")
		return
	}

	model, err := s.db.GetModel(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	hub, err := s.db.GetHub(model.HubID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hub")
		return
	}

	existing, err := s.db.ListEndpointsByModelID(model.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load endpoints")
		return
	}
	have := make(map[string]bool, len(existing))
	for _, ep := range existing {
		have[ep.Protocol] = true
	}
	missing := []string{}
	// The stored capability is authoritative here because the model already
	// exists (GH #100: shared capability→protocol mapping, incl. video).
	for _, protocol := range hubclient.ProtocolsForCapability(model.Capability) {
		if !have[protocol] {
			missing = append(missing, protocol)
		}
	}

	working, failures := s.trialProtocols(r.Context(), *hub, model.ModelID, missing)
	created := make([]string, 0, len(working))
	for _, protocol := range working {
		if ep, isNew, err := s.db.CreateEndpoint(model.ID, protocol, true); err != nil {
			// Keep going: a partially created trial must not surface as a
			// bare 500 with the already-created endpoints left invisible.
			slog.Error("model trial: create endpoint", "model_id", model.ID, "protocol", protocol, "error", err)
			if failures == "" {
				failures = fmt.Sprintf("create %s endpoint failed", protocol)
			} else {
				failures += fmt.Sprintf("; create %s endpoint failed", protocol)
			}
			continue
		} else if isNew {
			if err := s.db.ApplyCreationDefaults(ep.ID, protocol); err != nil {
				slog.Error("model trial: apply creation defaults", "model_id", model.ID, "protocol", protocol, "error", err)
			}
			created = append(created, protocol)
		}
	}

	endpoints, err := s.db.ListEndpointsByModelID(model.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load endpoints")
		return
	}

	result := "success"
	if len(created) == 0 && failures != "" {
		result = "failed: unreachable on all missing protocols"
	}
	s.audit(r, "model.trial", "model", strconv.FormatInt(model.ID, 10),
		fmt.Sprintf("model_id=%q created=%v failures=%q", model.ModelID, created, failures), result)
	if len(created) > 0 {
		s.InvalidateOverview()
	}
	writeData(w, http.StatusOK, trialResultDTO{
		Model:            toModelDTO(*model, endpoints),
		CreatedProtocols: created,
		Failures:         failures,
	})
}

// modelEvalSummaryDTO is the response of GET /api/models/{id}/eval-summary:
// the model's latest evaluation summary, or null when the model has never been
// evaluated. The total_score is the ADR-0005 weighted average of the model's
// suite scores, nadir-normalized per ADR-0009 on the 0-100 scale.
type modelEvalSummaryDTO struct {
	ModelID           int64                 `json:"model_id"`
	ModelIDStr        string                `json:"model_id_str"`
	CampaignID        int64                 `json:"campaign_id"`
	CampaignCreatedAt string                `json:"campaign_created_at"`
	TotalScore        *float64              `json:"total_score"`
	SuiteScores       []modelEvalSuiteScore `json:"suite_scores"`
}

// modelEvalSuiteScore is one suite's score within the model evaluation summary.
type modelEvalSuiteScore struct {
	SuiteID   int64    `json:"suite_id"`
	SuiteName string   `json:"suite_name"`
	Version   int      `json:"version"`
	Score     *float64 `json:"score"`
}

// handleGetModelEvalSummary handles GET /api/models/{id}/eval-summary. Returns
// the model's latest campaign evaluation summary (total score plus per-suite
// breakdown), or {"data": null} when the model has never been evaluated. A
// nonexistent model returns 404.
func (s *Server) handleGetModelEvalSummary(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid model id")
		return
	}

	model, err := s.db.GetModel(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}

	campaign, err := s.db.GetLatestCampaignForModel(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load latest campaign")
		return
	}
	if campaign == nil {
		writeData(w, http.StatusOK, nil)
		return
	}

	runs, err := s.db.ListEvalRunsByCampaign(campaign.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign runs")
		return
	}

	suites, err := s.campaignSuites(runs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load suites")
		return
	}

	configured, err := s.db.GetSuiteWeights()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read suite weights")
		return
	}
	weights := effectiveWeights(suites, configured)

	scores, err := s.db.ListCampaignSuiteScores(campaign.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load campaign scores")
		return
	}

	nadirs := nadirBySuiteKey(suites, runs)
	rows := buildReportRows(scores, weights, nadirs)

	// Find this model's row
	var modelRow *reportRowDTO
	for _, row := range rows {
		if row.ModelDBID == id {
			modelRow = &row
			break
		}
	}

	// Model participated but has no scores yet
	if modelRow == nil {
		writeData(w, http.StatusOK, nil)
		return
	}

	// Build suite scores array
	suiteScores := make([]modelEvalSuiteScore, 0, len(suites))
	for _, suite := range suites {
		score := modelRow.SuiteScores[suite.Key]
		suiteScores = append(suiteScores, modelEvalSuiteScore{
			SuiteID:   suite.ID,
			SuiteName: suite.Name,
			Version:   suite.Version,
			Score:     score,
		})
	}

	writeData(w, http.StatusOK, modelEvalSummaryDTO{
		ModelID:           model.ID,
		ModelIDStr:        model.ModelID,
		CampaignID:        campaign.ID,
		CampaignCreatedAt: campaign.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		TotalScore:        modelRow.TotalScore,
		SuiteScores:       suiteScores,
	})
}
