package store

import (
	"strings"
	"time"

	"github.com/taliove2009/hubscope/internal/classifier"
)

// ListClassificationRules returns all rules ordered per dimension by
// (priority, id) — the exact order Classify evaluates them in.
func (db *DB) ListClassificationRules() ([]classifier.Rule, error) {
	rows, err := db.conn.Query(`
		SELECT id, dimension, keyword, category, priority
		FROM classification_rules
		ORDER BY dimension, priority, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []classifier.Rule
	for rows.Next() {
		var r classifier.Rule
		if err := rows.Scan(&r.ID, &r.Dimension, &r.Keyword, &r.Category, &r.Priority); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// GetClassificationRule fetches one rule by ID.
func (db *DB) GetClassificationRule(id int64) (*classifier.Rule, error) {
	var r classifier.Rule
	err := db.conn.QueryRow(`
		SELECT id, dimension, keyword, category, priority
		FROM classification_rules WHERE id = ?
	`, id).Scan(&r.ID, &r.Dimension, &r.Keyword, &r.Category, &r.Priority)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateClassificationRule inserts a rule and returns it.
func (db *DB) CreateClassificationRule(dimension, keyword, category string, priority int) (*classifier.Rule, error) {
	result, err := db.conn.Exec(
		"INSERT INTO classification_rules (dimension, keyword, category, priority, created_at) VALUES (?, ?, ?, ?, ?)",
		dimension, keyword, category, priority, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &classifier.Rule{ID: id, Dimension: dimension, Keyword: keyword, Category: category, Priority: priority}, nil
}

// UpdateClassificationRule applies a partial update; nil fields stay unchanged.
func (db *DB) UpdateClassificationRule(id int64, keyword, category *string, priority *int) (*classifier.Rule, error) {
	updates := []string{}
	args := []interface{}{}
	if keyword != nil {
		updates = append(updates, "keyword = ?")
		args = append(args, *keyword)
	}
	if category != nil {
		updates = append(updates, "category = ?")
		args = append(args, *category)
	}
	if priority != nil {
		updates = append(updates, "priority = ?")
		args = append(args, *priority)
	}
	if len(updates) > 0 {
		query := "UPDATE classification_rules SET " + strings.Join(updates, ", ") + " WHERE id = ?"
		args = append(args, id)
		if _, err := db.conn.Exec(query, args...); err != nil {
			return nil, err
		}
	}
	return db.GetClassificationRule(id)
}

// DeleteClassificationRule removes a rule.
func (db *DB) DeleteClassificationRule(id int64) error {
	_, err := db.conn.Exec("DELETE FROM classification_rules WHERE id = ?", id)
	return err
}

// classificationRulesSeededKey is the settings flag marking that the default
// rules were seeded once. Using a flag instead of an empty-table check keeps
// a deliberately emptied rule table from being reseeded on the next boot.
const classificationRulesSeededKey = "classification_rules_seeded"

// seedClassificationRules writes the built-in default rules exactly once.
// Afterwards the database is the source of truth.
func (db *DB) seedClassificationRules() error {
	seeded, err := db.GetSettingBool(classificationRulesSeededKey, false)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	for _, r := range classifier.DefaultRules() {
		if _, err := db.CreateClassificationRule(r.Dimension, r.Keyword, r.Category, r.Priority); err != nil {
			return err
		}
	}
	return db.SetSettingBool(classificationRulesSeededKey, true)
}

// ReclassifyAll re-runs classification for every stored model against the
// current rule set, updating rows whose classification changed. Called after
// rule mutations and once at startup (covering upgrades and seed changes).
func (db *DB) ReclassifyAll() error {
	rules, err := db.ListClassificationRules()
	if err != nil {
		return err
	}
	models, err := db.ListModels()
	if err != nil {
		return err
	}
	for _, m := range models {
		capability, family := classifier.Classify(m.ModelID, rules)
		if capability == m.Capability && family == m.Family {
			continue
		}
		if err := db.SetModelClassification(m.ID, capability, family); err != nil {
			return err
		}
	}
	return nil
}
