package store

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strconv"
)

// Setting keys persisted in the settings table. All settings are stored as
// strings; booleans use "true"/"false".
const (
	// SettingLarkWebhookURL is the Lark group-bot webhook endpoint.
	SettingLarkWebhookURL = "lark_webhook_url"
	// SettingAlertEnabled gates down/recovered probe alerts.
	SettingAlertEnabled = "alert_enabled"
	// SettingScoreDropAlertEnabled gates score-drop alerts (ticket 09).
	SettingScoreDropAlertEnabled = "score_drop_alert_enabled"
	// SettingJudgeModel is the LLM judge model used by eval verdicts.
	SettingJudgeModel = "judge_model"
	// SettingDefaultSampleCount is how many times a case is answered per run
	// when the case does not override it.
	SettingDefaultSampleCount = "default_sample_count"
	// SettingEvalConcurrency bounds how many (suite × model) cells of an
	// eval campaign execute at once (GH #26).
	SettingEvalConcurrency = "eval_concurrency"
	// SettingEvalCampaignBudgetMin is the campaign-level wall-clock budget
	// in minutes (GH #153): once a batch outlives it, unstarted cells are
	// dropped and their runs fail with the budget reason. 0 disables the
	// budget.
	SettingEvalCampaignBudgetMin = "eval_campaign_budget_minutes"
	// SettingSuiteWeights maps suite keys to leaderboard total-score weights
	// (JSON object, e.g. {"basic":2}); suites absent from the map weigh 1, so
	// the default is equal weighting (ADR 0005).
	SettingSuiteWeights = "suite_weights"
	// SettingSessionSecret is the HMAC signing key for session cookies. When
	// the SESSION_SECRET env var is unset, the server generates a random
	// 32-byte hex value here on first start and reuses it on restart.
	SettingSessionSecret = "session_secret"
	// SettingQuietHoursEnabled gates the daily quiet-hours window (spec 0017
	// ticket 4): inside the window alert sends are held and a summary is
	// delivered when the window ends.
	SettingQuietHoursEnabled = "quiet_hours_enabled"
	// SettingQuietHoursStart / SettingQuietHoursEnd are the window bounds:
	// integer hours 0–23 in the server's local timezone; cross-midnight
	// windows (e.g. 23→7) are supported, and start == end means "not
	// enabled" even when the switch is on.
	SettingQuietHoursStart = "quiet_hours_start"
	SettingQuietHoursEnd   = "quiet_hours_end"
	// SettingModelRegistryOverrides holds administrator corrections to the
	// built-in model registry (spec 0020 ticket 1) as a JSON array of
	// {match, iq_tier?, price_in?, price_out?} entries; overrides merge
	// field-by-field over the shipped table.
	SettingModelRegistryOverrides = "model_registry_overrides"
)

// Default setting values applied when a key has never been written.
const (
	// DefaultAlertEnabled enables probe alerts out of the box; they are
	// still skipped until a webhook URL is configured.
	DefaultAlertEnabled = true
	// DefaultScoreDropAlertEnabled enables score-drop alerts out of the box.
	DefaultScoreDropAlertEnabled = true
	// DefaultJudgeModel matches the eval contract default judge.
	DefaultJudgeModel = "claude-opus-4-8"
	// DefaultSampleCount answers each case once unless configured otherwise.
	DefaultSampleCount = 1
	// MaxSampleCount bounds per-case sampling to keep run cost predictable.
	MaxSampleCount = 10
	// DefaultEvalConcurrency runs four (suite × model) cells at once out of
	// the box — enough to shorten wall-clock batch time without hammering
	// hubs (GH #26).
	DefaultEvalConcurrency = 4
	// MaxEvalConcurrency caps the eval worker pool; beyond this hub load and
	// latency distortion grow without meaningful speedup.
	MaxEvalConcurrency = 16
	// DefaultEvalCampaignBudgetMin bounds a batch to one hour out of the
	// box (GH #153 introduced the budget; GH #171 converges the default to
	// the user's "every batch inside an hour" target): a half-dead Hub can
	// no longer hold a campaign for hours through 120s-per-request stalls.
	DefaultEvalCampaignBudgetMin = 60
	// MaxEvalCampaignBudgetMin caps the campaign budget at one week.
	MaxEvalCampaignBudgetMin = 10080
	// DefaultQuietHoursEnabled keeps alerting loud out of the box: quiet
	// hours are opt-in (spec 0017 story 26).
	DefaultQuietHoursEnabled = false
	// DefaultQuietHoursStart / DefaultQuietHoursEnd are the placeholder
	// window shown in the settings form (23:00–07:00); they only take effect
	// once the switch is turned on.
	DefaultQuietHoursStart = 23
	DefaultQuietHoursEnd   = 7
)

// GetSetting returns the stored value for key, or def when the key has never
// been set (or the read fails softly with no row). Read errors other than
// "no rows" are returned.
func (db *DB) GetSetting(key, def string) (string, error) {
	var value string
	err := db.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return def, nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// GetSettingBool is GetSetting for boolean settings stored as "true"/"false".
func (db *DB) GetSettingBool(key string, def bool) (bool, error) {
	defStr := "false"
	if def {
		defStr = "true"
	}
	value, err := db.GetSetting(key, defStr)
	if err != nil {
		return false, err
	}
	return value == "true", nil
}

// GetSettingInt is GetSetting for integer settings stored as decimal strings.
// An unparsable stored value falls back to the default.
func (db *DB) GetSettingInt(key string, def int) (int, error) {
	value, err := db.GetSetting(key, strconv.Itoa(def))
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return def, nil
	}
	return n, nil
}

// SetSetting upserts a setting value.
func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// SetSettingBool is SetSetting for boolean settings.
func (db *DB) SetSettingBool(key string, value bool) error {
	str := "false"
	if value {
		str = "true"
	}
	return db.SetSetting(key, str)
}

// SetSettingInt is SetSetting for integer settings.
func (db *DB) SetSettingInt(key string, value int) error {
	return db.SetSetting(key, strconv.Itoa(value))
}

// GetSuiteWeights reads the configured leaderboard suite weights. An unset or
// unparsable value yields an empty (non-nil) map, i.e. equal weighting — a
// corrupted value must never break report reads.
func (db *DB) GetSuiteWeights() (map[string]float64, error) {
	raw, err := db.GetSetting(SettingSuiteWeights, "")
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return map[string]float64{}, nil
	}
	weights := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &weights); err != nil {
		slog.Warn("settings: unparsable suite_weights, falling back to equal weighting", "error", err)
		return map[string]float64{}, nil
	}
	return weights, nil
}

// SetSuiteWeights persists the leaderboard suite weights as a JSON object.
func (db *DB) SetSuiteWeights(weights map[string]float64) error {
	data, err := json.Marshal(weights)
	if err != nil {
		return err
	}
	return db.SetSetting(SettingSuiteWeights, string(data))
}
