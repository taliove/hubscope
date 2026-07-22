package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/taliove2009/hubscope/internal/store"
)

// settingsDTO is the API representation of the application settings.
// lark_webhook_url is returned as-is (internal tool, no masking per the API
// contract).
type settingsDTO struct {
	LarkWebhookURL        string `json:"lark_webhook_url"`
	AlertEnabled          bool   `json:"alert_enabled"`
	ScoreDropAlertEnabled bool   `json:"score_drop_alert_enabled"`
	JudgeModel            string `json:"judge_model"`
}

// settingsPatch is the PUT body: every field is optional; a nil field leaves
// the stored value unchanged.
type settingsPatch struct {
	LarkWebhookURL        *string `json:"lark_webhook_url"`
	AlertEnabled          *bool   `json:"alert_enabled"`
	ScoreDropAlertEnabled *bool   `json:"score_drop_alert_enabled"`
	JudgeModel            *string `json:"judge_model"`
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
	}
	present := []bool{
		patch.LarkWebhookURL != nil,
		patch.AlertEnabled != nil,
		patch.ScoreDropAlertEnabled != nil,
		patch.JudgeModel != nil,
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
