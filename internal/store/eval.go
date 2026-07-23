package store

import (
	"database/sql"
	"time"
)

// Capability dimensions (ADR 0010): question-bank v3 organizes suites by the
// capability they measure, replacing the old mixed basic/reasoning/coding/
// chinese split. The empty string marks pre-v3 legacy suites.
const (
	CapabilityInstruction = "instruction"
	CapabilityReasoning   = "reasoning"
	CapabilityCoding      = "coding"
	CapabilityLanguage    = "language"
	CapabilityKnowledge   = "knowledge"
)

// Suite is an evaluation suite: a named group of cases along one capability
// dimension (instruction / reasoning / coding / language / knowledge; legacy
// suites carry an empty capability). Version starts at 1 and increments on
// every case create/replace/enable-toggle (Suite Version). Nadir is the
// lower bound of the 0~1 raw-score scale used for normalized scoring (ADR
// 0009); 0 degenerates to the legacy raw-mean caliber. Enabled is false for
// retired suites: they stay readable for history and curation but leave the
// evaluation rotation (full sweeps and the weekly batch skip them).
type Suite struct {
	ID         int64
	Key        string
	Name       string
	Version    int
	Capability string
	Nadir      float64
	Enabled    bool
}

// Case is a single evaluation question with its verdict configuration.
// RuleMode/RuleExpected are set for verdict_type="rule"; Rubric for "judge".
// Difficulty is one of basic/intermediate/hard. SampleCount is nil when the
// case inherits the global default sample count.
type Case struct {
	ID           int64
	SuiteID      int64
	Prompt       string
	VerdictType  string
	RuleMode     *string
	RuleExpected *string
	Rubric       *string
	Difficulty   string
	SampleCount  *int
	Enabled      bool
	CreatedAt    time.Time
}

// suiteColumns is the canonical suites column list.
const suiteColumns = `id, key, name, version, capability, nadir, enabled`

// caseColumns is the canonical cases column list.
const caseColumns = `id, suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, difficulty, sample_count, enabled, created_at`

// scanSuite scans one suites row.
func scanSuite(s rowScanner) (Suite, error) {
	var su Suite
	var enabled int
	if err := s.Scan(&su.ID, &su.Key, &su.Name, &su.Version, &su.Capability, &su.Nadir, &enabled); err != nil {
		return Suite{}, err
	}
	su.Enabled = enabled == 1
	return su, nil
}

// scanCase scans one cases row.
func scanCase(s rowScanner) (Case, error) {
	var c Case
	var enabled int
	var createdAt string
	if err := s.Scan(&c.ID, &c.SuiteID, &c.Prompt, &c.VerdictType,
		&c.RuleMode, &c.RuleExpected, &c.Rubric, &c.Difficulty, &c.SampleCount, &enabled, &createdAt); err != nil {
		return Case{}, err
	}
	c.Enabled = enabled == 1
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return c, nil
}

// ListSuites returns all suites ordered by id, retired ones included.
func (db *DB) ListSuites() ([]Suite, error) {
	return db.listSuites("SELECT " + suiteColumns + " FROM suites ORDER BY id")
}

// ListEnabledSuites returns only the suites in the evaluation rotation
// (enabled), ordered by id. Full sweeps and the weekly batch run over this
// set; retired suites stay listed by ListSuites for history and curation.
func (db *DB) ListEnabledSuites() ([]Suite, error) {
	return db.listSuites("SELECT " + suiteColumns + " FROM suites WHERE enabled = 1 ORDER BY id")
}

// listSuites runs a suite query.
func (db *DB) listSuites(query string) ([]Suite, error) {
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suites []Suite
	for rows.Next() {
		s, err := scanSuite(rows)
		if err != nil {
			return nil, err
		}
		suites = append(suites, s)
	}
	return suites, rows.Err()
}

// GetSuite retrieves a suite by ID.
func (db *DB) GetSuite(id int64) (*Suite, error) {
	s, err := scanSuite(db.conn.QueryRow(
		"SELECT "+suiteColumns+" FROM suites WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListCases returns all cases of a suite (enabled and disabled), by id.
func (db *DB) ListCases(suiteID int64) ([]Case, error) {
	return db.listCases("SELECT "+caseColumns+" FROM cases WHERE suite_id = ? ORDER BY id", suiteID)
}

// ListEnabledCases returns only the enabled cases of a suite, for execution.
func (db *DB) ListEnabledCases(suiteID int64) ([]Case, error) {
	return db.listCases("SELECT "+caseColumns+" FROM cases WHERE suite_id = ? AND enabled = 1 ORDER BY id", suiteID)
}

// listCases runs a case query against one suite.
func (db *DB) listCases(query string, suiteID int64) ([]Case, error) {
	rows, err := db.conn.Query(query, suiteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cases []Case
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

// GetCase retrieves a case by ID.
func (db *DB) GetCase(id int64) (*Case, error) {
	c, err := scanCase(db.conn.QueryRow(
		"SELECT "+caseColumns+" FROM cases WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// bumpSuiteVersion increments a suite's version inside tx. Every case
// mutation (create/replace/enable-toggle) goes through it.
func bumpSuiteVersion(tx *sql.Tx, suiteID int64) error {
	_, err := tx.Exec("UPDATE suites SET version = version + 1 WHERE id = ?", suiteID)
	return err
}

// insertCase inserts one case row inside tx and returns its ID.
func insertCase(tx *sql.Tx, c Case, now time.Time) (int64, error) {
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	difficulty := c.Difficulty
	if difficulty == "" {
		difficulty = "basic"
	}
	result, err := tx.Exec(`
		INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, difficulty, sample_count, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.SuiteID, c.Prompt, c.VerdictType, c.RuleMode, c.RuleExpected, c.Rubric, difficulty, c.SampleCount, enabled, now.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CreateCase inserts a case, bumps the parent suite's version, and returns
// the stored copy.
func (db *DB) CreateCase(c Case) (*Case, error) {
	now := time.Now().UTC()
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	id, err := insertCase(tx, c, now)
	if err != nil {
		return nil, err
	}
	if err := bumpSuiteVersion(tx, c.SuiteID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	created, err := db.GetCase(id)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ReplaceCase is the immutable "edit": it disables the old case, inserts the
// merged fields as a brand-new case, and bumps the parent suite's version —
// all in one transaction. Historical run results keep pointing at the old
// case row, so past runs still render the old prompt.
func (db *DB) ReplaceCase(oldID int64, c Case) (*Case, error) {
	now := time.Now().UTC()
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE cases SET enabled = 0 WHERE id = ?", oldID); err != nil {
		return nil, err
	}
	id, err := insertCase(tx, c, now)
	if err != nil {
		return nil, err
	}
	if err := bumpSuiteVersion(tx, c.SuiteID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	created, err := db.GetCase(id)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// SetSuiteEnabled toggles a suite's membership in the evaluation rotation.
// It is not a content change, so the suite's version stays untouched; retired
// suites keep their cases and history either way.
func (db *DB) SetSuiteEnabled(id int64, enabled bool) error {
	flag := 0
	if enabled {
		flag = 1
	}
	_, err := db.conn.Exec("UPDATE suites SET enabled = ? WHERE id = ?", flag, id)
	return err
}

// SetCaseEnabled toggles a case in place (no content change) and bumps the
// parent suite's version.
func (db *DB) SetCaseEnabled(id int64, enabled bool) (*Case, error) {
	existing, err := db.GetCase(id)
	if err != nil {
		return nil, err
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	flag := 0
	if enabled {
		flag = 1
	}
	if _, err := tx.Exec("UPDATE cases SET enabled = ? WHERE id = ?", flag, id); err != nil {
		return nil, err
	}
	if err := bumpSuiteVersion(tx, existing.SuiteID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetCase(id)
}
