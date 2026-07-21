package store

import (
	"database/sql"
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
