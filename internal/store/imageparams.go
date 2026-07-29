package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/taliove/hubscope/internal/imageparams"
)

// ListImageParamRules returns all rules ordered by (priority, id) — the exact
// order imageparams.Merge evaluates them in.
func (db *DB) ListImageParamRules() ([]imageparams.Rule, error) {
	rows, err := db.conn.Query(`
		SELECT id, keyword, params, priority
		FROM image_param_rules
		ORDER BY priority, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []imageparams.Rule
	for rows.Next() {
		var r imageparams.Rule
		var paramsJSON string
		if err := rows.Scan(&r.ID, &r.Keyword, &paramsJSON, &r.Priority); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(paramsJSON), &r.Params); err != nil {
			return nil, fmt.Errorf("image_param_rules row %d: corrupt params JSON: %w", r.ID, err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// GetImageParamRule fetches one rule by ID.
func (db *DB) GetImageParamRule(id int64) (*imageparams.Rule, error) {
	var r imageparams.Rule
	var paramsJSON string
	err := db.conn.QueryRow(`
		SELECT id, keyword, params, priority
		FROM image_param_rules WHERE id = ?
	`, id).Scan(&r.ID, &r.Keyword, &paramsJSON, &r.Priority)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(paramsJSON), &r.Params); err != nil {
		return nil, fmt.Errorf("image_param_rules row %d: corrupt params JSON: %w", r.ID, err)
	}
	return &r, nil
}

// CreateImageParamRule inserts a rule and returns it.
func (db *DB) CreateImageParamRule(keyword string, params map[string]string, priority int) (*imageparams.Rule, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	result, err := db.conn.Exec(
		"INSERT INTO image_param_rules (keyword, params, priority, created_at) VALUES (?, ?, ?, ?)",
		keyword, string(paramsJSON), priority, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &imageparams.Rule{ID: id, Keyword: keyword, Params: params, Priority: priority}, nil
}

// UpdateImageParamRule applies a partial update; nil fields stay unchanged.
func (db *DB) UpdateImageParamRule(id int64, keyword *string, params map[string]string, priority *int) (*imageparams.Rule, error) {
	updates := []string{}
	args := []interface{}{}
	if keyword != nil {
		updates = append(updates, "keyword = ?")
		args = append(args, *keyword)
	}
	if params != nil {
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		updates = append(updates, "params = ?")
		args = append(args, string(paramsJSON))
	}
	if priority != nil {
		updates = append(updates, "priority = ?")
		args = append(args, *priority)
	}
	if len(updates) > 0 {
		query := "UPDATE image_param_rules SET " + strings.Join(updates, ", ") + " WHERE id = ?"
		args = append(args, id)
		if _, err := db.conn.Exec(query, args...); err != nil {
			return nil, err
		}
	}
	return db.GetImageParamRule(id)
}

// DeleteImageParamRule removes a rule.
func (db *DB) DeleteImageParamRule(id int64) error {
	_, err := db.conn.Exec("DELETE FROM image_param_rules WHERE id = ?", id)
	return err
}

// ImageParamsFor is the single resolution entry every probe call site must
// use (GH #33): load the rule set and merge it for the model. Keeping rule
// loading in the store (never in hubclient) makes every rule mutation
// effective on the very next probe — there is no client-side cache to
// invalidate.
func (db *DB) ImageParamsFor(modelID string) (map[string]string, error) {
	rules, err := db.ListImageParamRules()
	if err != nil {
		return nil, err
	}
	return imageparams.Merge(modelID, rules), nil
}

// imageParamRulesSeededKey is the settings flag marking that the default
// rules were seeded once. Using a flag instead of an empty-table check keeps
// a deliberately emptied rule table from being reseeded on the next boot
// (same policy as classificationRulesSeededKey; the two flags are
// independent).
const imageParamRulesSeededKey = "image_param_rules_seeded"

// seedImageParamRules writes the built-in default rules exactly once.
// Afterwards the database is the source of truth.
func (db *DB) seedImageParamRules() error {
	seeded, err := db.GetSettingBool(imageParamRulesSeededKey, false)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	for _, r := range imageparams.DefaultRules() {
		if _, err := db.CreateImageParamRule(r.Keyword, r.Params, r.Priority); err != nil {
			return err
		}
	}
	return db.SetSettingBool(imageParamRulesSeededKey, true)
}
