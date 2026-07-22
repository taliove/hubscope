package server

import (
	"time"

	"github.com/taliove2009/hubscope/internal/store"
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
	ID         int64         `json:"id"`
	HubID      int64         `json:"hub_id"`
	ModelID    string        `json:"model_id"`
	Origin     string        `json:"origin"`
	Status     string        `json:"status"`
	Capability string        `json:"capability"`
	Family     string        `json:"family"`
	Endpoints  []endpointDTO `json:"endpoints"`
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
		ID:         m.ID,
		HubID:      m.HubID,
		ModelID:    m.ModelID,
		Origin:     m.Origin,
		Status:     m.Status,
		Capability: m.Capability,
		Family:     m.Family,
		Endpoints:  eps,
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
type caseDTO struct {
	ID          int64          `json:"id"`
	SuiteID     int64          `json:"suite_id"`
	Prompt      string         `json:"prompt"`
	VerdictType string         `json:"verdict_type"`
	RuleConfig  *ruleConfigDTO `json:"rule_config"`
	Rubric      *string        `json:"rubric"`
	Enabled     bool           `json:"enabled"`
}

// suiteDTO is the API representation of a Suite with its cases.
type suiteDTO struct {
	ID    int64     `json:"id"`
	Key   string    `json:"key"`
	Name  string    `json:"name"`
	Cases []caseDTO `json:"cases"`
}

// evalRunDTO is the API representation of an EvalRun. Score is the average
// of all non-null result scores, computed on read (never persisted); it is
// null when no result has been scored yet.
type evalRunDTO struct {
	ID         int64    `json:"id"`
	SuiteID    int64    `json:"suite_id"`
	Trigger    string   `json:"trigger"`
	JudgeModel string   `json:"judge_model"`
	Status     string   `json:"status"`
	StartedAt  string   `json:"started_at"`
	FinishedAt *string  `json:"finished_at"`
	Score      *float64 `json:"score"`
}

// evalResultDTO is the API representation of an EvalResult. ModelDeleted
// flags rows whose model has been removed, so history views can badge them.
type evalResultDTO struct {
	ID            int64    `json:"id"`
	ModelID       string   `json:"model_id"`
	CaseID        int64    `json:"case_id"`
	AnswerText    *string  `json:"answer_text"`
	Score         *float64 `json:"score"`
	VerdictDetail *string  `json:"verdict_detail"`
	LatencyMs     int      `json:"latency_ms"`
	InputTokens   *int     `json:"input_tokens"`
	OutputTokens  *int     `json:"output_tokens"`
	ModelDeleted  bool     `json:"model_deleted"`
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
	return dto
}

// toSuiteDTO maps a store.Suite plus its cases to the API representation.
func toSuiteDTO(s store.Suite, cases []store.Case) suiteDTO {
	caseDTOs := make([]caseDTO, 0, len(cases))
	for _, c := range cases {
		caseDTOs = append(caseDTOs, toCaseDTO(c))
	}
	return suiteDTO{ID: s.ID, Key: s.Key, Name: s.Name, Cases: caseDTOs}
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
		ID:         r.ID,
		SuiteID:    r.SuiteID,
		Trigger:    r.Trigger,
		JudgeModel: r.JudgeModel,
		Status:     r.Status,
		StartedAt:  r.StartedAt.Format(time.RFC3339),
		FinishedAt: finishedAt,
		Score:      score,
	}
}

// toEvalResultDTO maps a store.EvalResult to the API representation.
func toEvalResultDTO(r store.EvalResult) evalResultDTO {
	return evalResultDTO{
		ID:            r.ID,
		ModelID:       r.ModelID,
		CaseID:        r.CaseID,
		AnswerText:    r.AnswerText,
		Score:         r.Score,
		VerdictDetail: r.VerdictDetail,
		LatencyMs:     r.LatencyMs,
		InputTokens:   r.InputTokens,
		OutputTokens:  r.OutputTokens,
		ModelDeleted:  r.ModelDeleted,
	}
}

// deref returns the string a pointer holds, or "" for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
