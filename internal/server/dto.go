package server

import (
	"encoding/json"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// hubDTO is the API representation of a Hub. It never carries the raw token.
type hubDTO struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	BaseURL       string  `json:"base_url"`
	TokenHint     string  `json:"token_hint"`
	SyncStatus    string  `json:"sync_status"`
	LastSyncedAt  *string `json:"last_synced_at"`
	LastSyncError *string `json:"last_sync_error"`
	CreatedAt     string  `json:"created_at"`
}

// endpointDTO is the API representation of an Endpoint.
type endpointDTO struct {
	ID              int64  `json:"id"`
	ModelID         int64  `json:"model_id"`
	Protocol        string `json:"protocol"`
	Enabled         bool   `json:"enabled"`
	IntervalSeconds *int   `json:"interval_seconds"`
}

// modelDTO is the API representation of a Model with its endpoints.
type modelDTO struct {
	ID          int64         `json:"id"`
	HubID       int64         `json:"hub_id"`
	ModelID     string        `json:"model_id"`
	Origin      string        `json:"origin"`
	Status      string        `json:"status"`
	Capability  string        `json:"capability"`
	Family      string        `json:"family"`
	EvalEnabled bool          `json:"eval_enabled"`
	Endpoints   []endpointDTO `json:"endpoints"`
}

// probeDTO is the API representation of a ProbeRecord.
type probeDTO struct {
	ID           int64   `json:"id"`
	EndpointID   int64   `json:"endpoint_id"`
	Streaming    bool    `json:"streaming"`
	OK           bool    `json:"ok"`
	HTTPStatus   int     `json:"http_status"`
	ErrorSummary *string `json:"error_summary"`
	LatencyMs    int     `json:"latency_ms"`
	TTFTMs       *int    `json:"ttft_ms"`
	InputTokens  *int    `json:"input_tokens"`
	OutputTokens *int    `json:"output_tokens"`
	CreatedAt    string  `json:"created_at"`
}

// maskToken returns the last 4 characters of a token prefixed with an ellipsis.
// A token shorter than 4 characters yields just the ellipsis.
func maskToken(token string) string {
	runes := []rune(token)
	if len(runes) < 4 {
		return "…"
	}
	return "…" + string(runes[len(runes)-4:])
}

// toHubDTO maps a store.Hub to its API representation.
func toHubDTO(h store.Hub) hubDTO {
	d := hubDTO{
		ID:            h.ID,
		Name:          h.Name,
		BaseURL:       h.BaseURL,
		TokenHint:     maskToken(h.Token),
		SyncStatus:    h.SyncStatus,
		LastSyncError: h.LastSyncError,
		CreatedAt:     h.CreatedAt.Format(time.RFC3339),
	}
	if h.LastSyncedAt != nil {
		s := h.LastSyncedAt.Format(time.RFC3339)
		d.LastSyncedAt = &s
	}
	return d
}

// toEndpointDTO maps a store.Endpoint to its API representation.
func toEndpointDTO(e store.Endpoint) endpointDTO {
	return endpointDTO{
		ID:              e.ID,
		ModelID:         e.ModelID,
		Protocol:        e.Protocol,
		Enabled:         e.Enabled,
		IntervalSeconds: e.IntervalSeconds,
	}
}

// toModelDTO maps a store.Model plus its endpoints to the API representation.
func toModelDTO(m store.Model, endpoints []store.Endpoint) modelDTO {
	eps := make([]endpointDTO, 0, len(endpoints))
	for _, e := range endpoints {
		eps = append(eps, toEndpointDTO(e))
	}
	return modelDTO{
		ID:          m.ID,
		HubID:       m.HubID,
		ModelID:     m.ModelID,
		Origin:      m.Origin,
		Status:      m.Status,
		Capability:  m.Capability,
		Family:      m.Family,
		EvalEnabled: m.EvalEnabled,
		Endpoints:   eps,
	}
}

// toProbeDTO maps a store.Probe to the API representation.
func toProbeDTO(p store.Probe) probeDTO {
	return probeDTO{
		ID:           p.ID,
		EndpointID:   p.EndpointID,
		Streaming:    p.Streaming,
		OK:           p.OK,
		HTTPStatus:   p.HTTPStatus,
		ErrorSummary: p.ErrorSummary,
		LatencyMs:    p.LatencyMs,
		TTFTMs:       p.TTFTMs,
		InputTokens:  p.InputTokens,
		OutputTokens: p.OutputTokens,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
	}
}

// ruleConfigDTO is the API representation of a rule verdict configuration.
type ruleConfigDTO struct {
	Mode     string `json:"mode"`
	Expected string `json:"expected"`
}

// caseDTO is the API representation of a Case. rule_config is only populated
// for verdict_type="rule" and rubric only for "judge"; the other is null.
// sample_count is null when the case inherits the global default.
// check_params carries the IFEval structured check parameters (a JSON array
// of {instruction_id, kwargs}) for rule mode "ifeval" and is null otherwise
// (ticket 97); it is seed-cast data the admin API never authors, only
// preserves.
type caseDTO struct {
	ID          int64           `json:"id"`
	SuiteID     int64           `json:"suite_id"`
	Prompt      string          `json:"prompt"`
	VerdictType string          `json:"verdict_type"`
	RuleConfig  *ruleConfigDTO  `json:"rule_config"`
	Rubric      *string         `json:"rubric"`
	Difficulty  string          `json:"difficulty"`
	SampleCount *int            `json:"sample_count"`
	CheckParams json.RawMessage `json:"check_params"`
	Enabled     bool            `json:"enabled"`
}

// suiteDTO is the API representation of a Suite with its cases. Version is
// the suite's question-bank version (Suite Version). Capability names the
// ADR 0010 capability dimension ("" for pre-v3 legacy suites); nadir is the
// suite's normalization constant (ADR 0009); enabled is false for retired
// suites, which stay listed for history and curation but leave the
// evaluation rotation.
type suiteDTO struct {
	ID         int64     `json:"id"`
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	Version    int       `json:"version"`
	Capability string    `json:"capability"`
	Nadir      float64   `json:"nadir"`
	Enabled    bool      `json:"enabled"`
	Cases      []caseDTO `json:"cases"`
}

// evalRunDTO is the API representation of an EvalRun. Score is the mean of
// all non-null result scores scaled through the ADR-0009 nadir normalization
// (kept on the 0~1 wire scale), computed on read (never persisted); it is
// null when no result has been scored yet. SuiteVersion and Nadir snapshot
// the question-bank version and normalization constant the run scored
// against. CampaignID groups the run into its evaluation batch (added by
// ticket 29; additive, never changed in place).
type evalRunDTO struct {
	ID           int64   `json:"id"`
	CampaignID   int64   `json:"campaign_id"`
	SuiteID      int64   `json:"suite_id"`
	SuiteVersion int     `json:"suite_version"`
	Nadir        float64 `json:"nadir"`
	Trigger      string  `json:"trigger"`
	JudgeModel   string  `json:"judge_model"`
	// JuryModels carries the run's jury snapshot verbatim (spec 0020 /
	// ADR 0016): policy plus per-model judges; null for pre-jury runs.
	JuryModels json.RawMessage `json:"jury_models"`
	// EstimatedCost carries the run's estimated cost split
	// {"exam":x,"judge":y} (GH #178); null when unset or when some
	// component's price is not registered.
	EstimatedCost json.RawMessage `json:"estimated_cost"`
	Status        string          `json:"status"`
	StartedAt     string          `json:"started_at"`
	FinishedAt    *string         `json:"finished_at"`
	Score         *float64        `json:"score"`
}

// judgeVoteDTO is one jury vote on one sample (GH #178).
type judgeVoteDTO struct {
	SampleNo   int      `json:"sample_no"`
	Slot       int      `json:"slot"`
	JudgeModel string   `json:"judge_model"`
	Score      *float64 `json:"score"`
}

// evalResultDTO is the API representation of an EvalResult. ModelDeleted
// flags rows whose model has been removed, so history views can badge them.
// VerdictProfile names the scoring caliber the row was judged with (ADR 0008).
type evalResultDTO struct {
	ID             int64    `json:"id"`
	ModelID        string   `json:"model_id"`
	CaseID         int64    `json:"case_id"`
	AnswerText     *string  `json:"answer_text"`
	Score          *float64 `json:"score"`
	VerdictDetail  *string  `json:"verdict_detail"`
	VerdictProfile string   `json:"verdict_profile"`
	LatencyMs      int      `json:"latency_ms"`
	InputTokens    *int     `json:"input_tokens"`
	OutputTokens   *int     `json:"output_tokens"`
	ModelDeleted   bool     `json:"model_deleted"`
	// JudgeScores carries the case's per-jury-slot votes from its latest
	// answer attempts (GH #178); empty for rule verdicts. Spread is the
	// max-min disagreement across the case's non-null votes, absent when
	// fewer than two votes scored.
	JudgeScores []judgeVoteDTO `json:"judge_scores,omitempty"`
	Spread      *float64       `json:"spread,omitempty"`
}

// latestScoreDTO is the API representation of a (suite, model) pair's most
// recent done-run aggregate score.
type latestScoreDTO struct {
	SuiteID    int64    `json:"suite_id"`
	SuiteKey   string   `json:"suite_key"`
	ModelID    string   `json:"model_id"`
	ModelDBID  int64    `json:"model_db_id"`
	Score      *float64 `json:"score"`
	EvalRunID  int64    `json:"eval_run_id"`
	FinishedAt string   `json:"finished_at"`
}

// evalRunDetailDTO is an EvalRun plus its per-case results.
type evalRunDetailDTO struct {
	evalRunDTO
	Results []evalResultDTO `json:"results"`
}

// toLatestScoreDTO maps a store.LatestEvalScore to its API representation.
func toLatestScoreDTO(ls store.LatestEvalScore) latestScoreDTO {
	return latestScoreDTO{
		SuiteID:    ls.SuiteID,
		SuiteKey:   ls.SuiteKey,
		ModelID:    ls.ModelID,
		ModelDBID:  ls.ModelDBID,
		Score:      ls.Score,
		EvalRunID:  ls.EvalRunID,
		FinishedAt: ls.FinishedAt.Format(time.RFC3339),
	}
}

// toCaseDTO maps a store.Case to its API representation.
func toCaseDTO(c store.Case) caseDTO {
	dto := caseDTO{
		ID:          c.ID,
		SuiteID:     c.SuiteID,
		Prompt:      c.Prompt,
		VerdictType: c.VerdictType,
		Difficulty:  c.Difficulty,
		SampleCount: c.SampleCount,
		Enabled:     c.Enabled,
	}
	if c.VerdictType == "rule" {
		dto.RuleConfig = &ruleConfigDTO{
			Mode:     deref(c.RuleMode),
			Expected: deref(c.RuleExpected),
		}
	}
	if c.VerdictType == "judge" {
		dto.Rubric = c.Rubric
	}
	if c.CheckParams != nil {
		dto.CheckParams = json.RawMessage(*c.CheckParams)
	}
	return dto
}

// toSuiteDTO maps a store.Suite plus its cases to the API representation.
func toSuiteDTO(s store.Suite, cases []store.Case) suiteDTO {
	caseDTOs := make([]caseDTO, 0, len(cases))
	for _, c := range cases {
		caseDTOs = append(caseDTOs, toCaseDTO(c))
	}
	return suiteDTO{
		ID: s.ID, Key: s.Key, Name: s.Name, Version: s.Version,
		Capability: s.Capability, Nadir: s.Nadir, Enabled: s.Enabled,
		Cases: caseDTOs,
	}
}

// toEvalRunDTO maps a store.EvalRun to the API representation, attaching the
// aggregate score computed from the run's results.
func toEvalRunDTO(r store.EvalRun, score *float64) evalRunDTO {
	var finishedAt *string
	if r.FinishedAt != nil {
		s := r.FinishedAt.Format(time.RFC3339)
		finishedAt = &s
	}
	return evalRunDTO{
		ID:            r.ID,
		CampaignID:    r.CampaignID,
		SuiteID:       r.SuiteID,
		SuiteVersion:  r.SuiteVersion,
		Nadir:         r.Nadir,
		Trigger:       r.Trigger,
		JudgeModel:    r.JudgeModel,
		JuryModels:    juryRawMessage(r.JuryModels),
		EstimatedCost: juryRawMessage(r.EstimatedCost),
		Status:        r.Status,
		StartedAt:     r.StartedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt,
		Score:         score,
	}
}

// juryRawMessage wraps a stored jury snapshot for the API: empty stays null
// (pre-jury runs), anything else is passed through verbatim.
func juryRawMessage(snapshot string) json.RawMessage {
	if snapshot == "" {
		return nil
	}
	return json.RawMessage(snapshot)
}

// toEvalResultDTO maps a store.EvalResult to the API representation.
func toEvalResultDTO(r store.EvalResult) evalResultDTO {
	return evalResultDTO{
		ID:             r.ID,
		ModelID:        r.ModelID,
		CaseID:         r.CaseID,
		AnswerText:     r.AnswerText,
		Score:          r.Score,
		VerdictDetail:  r.VerdictDetail,
		VerdictProfile: r.VerdictProfile,
		LatencyMs:      r.LatencyMs,
		InputTokens:    r.InputTokens,
		OutputTokens:   r.OutputTokens,
		ModelDeleted:   r.ModelDeleted,
	}
}

// deref returns the string a pointer holds, or "" for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// campaignProgressDTO is the per-status run-count aggregate of a campaign,
// plus the judged-unit aggregate for active campaigns (2026-08-03 sidebar
// pill caliber: judged units, since runs settle late under model-major).
type campaignProgressDTO struct {
	Total   int `json:"total"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
	Running int `json:"running"`
	// JudgedUnits/ExpectedUnits are filled only for running/pending
	// campaigns; zero on settled ones.
	JudgedUnits   int `json:"judged_units"`
	ExpectedUnits int `json:"expected_units"`
}

// campaignDTO is the API representation of a Campaign with its run-count
// progress. StartedAt is null only for the reserved pending status.
type campaignDTO struct {
	ID         int64               `json:"id"`
	Trigger    string              `json:"trigger"`
	Status     string              `json:"status"`
	StartedAt  *string             `json:"started_at"`
	FinishedAt *string             `json:"finished_at"`
	CreatedAt  string              `json:"created_at"`
	Progress   campaignProgressDTO `json:"progress"`
}

// campaignDetailDTO is a campaign plus its member runs (each with its
// aggregate score), oldest first.
type campaignDetailDTO struct {
	campaignDTO
	Runs []evalRunDTO `json:"runs"`
}

// toCampaignDTO maps a store.CampaignWithProgress to its API representation.
func toCampaignDTO(c store.CampaignWithProgress) campaignDTO {
	return campaignDTO{
		ID:         c.ID,
		Trigger:    c.Trigger,
		Status:     c.Status,
		StartedAt:  formatTimePtr(c.StartedAt),
		FinishedAt: formatTimePtr(c.FinishedAt),
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		Progress: campaignProgressDTO{
			Total:   c.Progress.Total,
			Done:    c.Progress.Done,
			Failed:  c.Progress.Failed,
			Running: c.Progress.Running,
		},
	}
}

// formatTimePtr formats an optional timestamp as RFC3339, nil staying nil.
func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}
