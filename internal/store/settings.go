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
	// SettingSuiteWeights maps suite keys to leaderboard total-score weights
	// (JSON object, e.g. {"basic":2}); suites absent from the map weigh 1, so
	// the default is equal weighting (ADR 0005).
	SettingSuiteWeights = "suite_weights"
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
