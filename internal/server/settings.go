package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/taliove/hubscope/internal/store"
)

// settingsDTO is the API representation of the application settings.
// lark_webhook_url is returned as-is (internal tool, no masking per the API
// contract).
type settingsDTO struct {
	LarkWebhookURL        string             `json:"lark_webhook_url"`
	AlertEnabled          bool               `json:"alert_enabled"`
	ScoreDropAlertEnabled bool               `json:"score_drop_alert_enabled"`
	JudgeModel            string             `json:"judge_model"`
	DefaultSampleCount    int                `json:"default_sample_count"`
	SuiteWeights          map[string]float64 `json:"suite_weights"`
}

// settingsPatch is the PUT body: every field is optional; a nil field leaves
// the stored value unchanged. SuiteWeights is a map: absent (or explicit
// null) leaves it unchanged, an object replaces the whole weight map.
type settingsPatch struct {
	LarkWebhookURL        *string            `json:"lark_webhook_url"`
	AlertEnabled          *bool              `json:"alert_enabled"`
	ScoreDropAlertEnabled *bool              `json:"score_drop_alert_enabled"`
	JudgeModel            *string            `json:"judge_model"`
	DefaultSampleCount    *int               `json:"default_sample_count"`
	SuiteWeights          map[string]float64 `json:"suite_weights"`
}

// readSettings loads all settings, applying defaults for keys never written.
func (s *Server) readSettings() (settingsDTO, error) {
	var dto settingsDTO
	var err error
	if dto.LarkWebhookURL, err = s.db.GetSetting(store.SettingLarkWebhookURL, ""); err != nil {
		return dto, err
	}
	if dto.AlertEnabled, err = s.db.GetSettingBool(store.SettingAlertEnabled, store.DefaultAlertEnabled); err != nil {
		return dto, err
	}
	if dto.ScoreDropAlertEnabled, err = s.db.GetSettingBool(store.SettingScoreDropAlertEnabled, store.DefaultScoreDropAlertEnabled); err != nil {
		return dto, err
	}
	if dto.JudgeModel, err = s.db.GetSetting(store.SettingJudgeModel, store.DefaultJudgeModel); err != nil {
		return dto, err
	}
	if dto.DefaultSampleCount, err = s.db.GetSettingInt(store.SettingDefaultSampleCount, store.DefaultSampleCount); err != nil {
		return dto, err
	}
	if dto.SuiteWeights, err = s.db.GetSuiteWeights(); err != nil {
		return dto, err
	}
	return dto, nil
}

// handleGetSettings handles GET /api/settings. Public (read).
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	dto, err := s.readSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	writeData(w, http.StatusOK, dto)
}

// handlePutSettings handles PUT /api/settings with a partial update. Changes
// take effect immediately: the alert evaluator re-reads settings on every
// transition, so no restart is required.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var patch settingsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if patch.DefaultSampleCount != nil &&
		(*patch.DefaultSampleCount < 1 || *patch.DefaultSampleCount > store.MaxSampleCount) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("default_sample_count must be between 1 and %d", store.MaxSampleCount))
		return
	}

	if patch.SuiteWeights != nil {
		if err := s.validateSuiteWeights(patch.SuiteWeights); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	updates := []struct {
		key   string
		apply func() error
	}{
		{store.SettingLarkWebhookURL, func() error {
			return s.db.SetSetting(store.SettingLarkWebhookURL, *patch.LarkWebhookURL)
		}},
		{store.SettingAlertEnabled, func() error {
			return s.db.SetSettingBool(store.SettingAlertEnabled, *patch.AlertEnabled)
		}},
		{store.SettingScoreDropAlertEnabled, func() error {
			return s.db.SetSettingBool(store.SettingScoreDropAlertEnabled, *patch.ScoreDropAlertEnabled)
		}},
		{store.SettingJudgeModel, func() error {
			return s.db.SetSetting(store.SettingJudgeModel, *patch.JudgeModel)
		}},
		{store.SettingDefaultSampleCount, func() error {
			return s.db.SetSettingInt(store.SettingDefaultSampleCount, *patch.DefaultSampleCount)
		}},
		{store.SettingSuiteWeights, func() error {
			return s.db.SetSuiteWeights(patch.SuiteWeights)
		}},
	}
	present := []bool{
		patch.LarkWebhookURL != nil,
		patch.AlertEnabled != nil,
		patch.ScoreDropAlertEnabled != nil,
		patch.JudgeModel != nil,
		patch.DefaultSampleCount != nil,
		patch.SuiteWeights != nil,
	}
	for i, u := range updates {
		if !present[i] {
			continue
		}
		if err := u.apply(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update settings")
			return
		}
	}

	dto, err := s.readSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}

	// Audit key names only — setting values (webhook URLs, judge model) stay
	// out of the log.
	keys := []string{}
	for i, u := range updates {
		if present[i] {
			keys = append(keys, u.key)
		}
	}
	s.audit(r, "settings.update", "settings", "", "keys="+strings.Join(keys, ","), "success")
	writeData(w, http.StatusOK, dto)
}

// testLarkRequest is the POST /api/settings/test-lark body: the address
// under test is the one in the form, not the saved setting (ticket 100
// decision 1 — verify before saving).
type testLarkRequest struct {
	WebhookURL string `json:"webhook_url"`
}

// testLarkResult is the response data: the send outcome and, on failure, the
// reason. The error text never contains the webhook URL (LarkSender strips
// url.Error, W6).
type testLarkResult struct {
	SentOK bool    `json:"sent_ok"`
	Error  *string `json:"error"`
}

// handleTestLark handles POST /api/settings/test-lark (super_admin): it
// sends the fixed test message through the process-wide alert evaluator and
// reports the outcome. Every attempt — success or failure — is recorded as
// an alert_events row with kind="test" inside the evaluator. The manual test
// is not gated by alert_enabled (ticket 100 decision 3: the switch governs
// automatic alerts only).
func (s *Server) handleTestLark(w http.ResponseWriter, r *http.Request) {
	var req testLarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	webhookURL := strings.TrimSpace(req.WebhookURL)
	if !isAbsoluteHTTPURL(webhookURL) {
		writeError(w, http.StatusBadRequest, "webhook_url must be an absolute http(s) URL")
		return
	}

	sendErr := s.alerter.SendTest(r.Context(), webhookURL)
	result := "success"
	var errText *string
	if sendErr != nil {
		result = "failure"
		msg := sendErr.Error()
		errText = &msg
	}
	// Audit the action and outcome only — the webhook URL carries the bot
	// token and stays out of the audit log (W6).
	s.audit(r, "settings.test_lark", "settings", "", "", result)
	writeData(w, http.StatusOK, testLarkResult{SentOK: sendErr == nil, Error: errText})
}

// isAbsoluteHTTPURL accepts only absolute http/https URLs, rejecting empty
// input, relative references, and non-HTTP schemes (file://, ftp://, …).
func isAbsoluteHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// validateSuiteWeights checks a suite_weights patch: keys must name existing
// suites and every weight must be positive (zero or negative weights would
// silently distort the leaderboard total). Weights are capped so absurd
// values cannot overflow the weighted average to +Inf/NaN.
const maxSuiteWeight = 1000.0

func (s *Server) validateSuiteWeights(weights map[string]float64) error {
	suites, err := s.db.ListSuites()
	if err != nil {
		return fmt.Errorf("failed to load suites")
	}
	known := make(map[string]bool, len(suites))
	for _, suite := range suites {
		known[suite.Key] = true
	}
	for key, weight := range weights {
		if !known[key] {
			return fmt.Errorf("suite_weights key %q is not a suite", key)
		}
		if weight <= 0 {
			return fmt.Errorf("suite_weights[%q] must be positive", key)
		}
		if weight > maxSuiteWeight {
			return fmt.Errorf("suite_weights[%q] must be at most %v", key, maxSuiteWeight)
		}
	}
	return nil
}
