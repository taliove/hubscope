package store

import (
	"database/sql"
	"time"
)

// Suite is an evaluation suite: a named group of cases along one capability
// dimension (basic / reasoning / coding / chinese). Version starts at 1 and
// increments on every case create/replace/enable-toggle (Suite Version).
type Suite struct {
	ID      int64
	Key     string
	Name    string
	Version int
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

// EvalRun is one execution of a suite against a set of models. SuiteVersion
// snapshots the suite's version at creation, so a run is always attributable
// to the question-bank version it scored. CampaignID links the run to its
// evaluation batch (Eval Campaign); every run belongs to exactly one.
type EvalRun struct {
	ID           int64
	CampaignID   int64
	SuiteID      int64
	SuiteVersion int
	Trigger      string
	JudgeModel   string
	Status       string
	StartedAt    time.Time
	FinishedAt   *time.Time
}

// LatestEvalScore is the aggregate score of the most recent done run for one
// (suite, model) pair. Score is nil when that run scored nothing.
type LatestEvalScore struct {
	SuiteID    int64
	SuiteKey   string
	ModelDBID  int64
	ModelID    string
	Score      *float64
	EvalRunID  int64
	FinishedAt time.Time
}

// EvalResult is the outcome of one (model, case) pair inside a run. Score is
// nil when the case could not be judged (answer call failed, judge failed).
// ModelDeleted reports whether the model has since been removed from the
// models table; the run history keeps the row so the UI can badge it.
type EvalResult struct {
	ID            int64
	EvalRunID     int64
	ModelDBID     int64
	ModelID       string
	CaseID        int64
	AnswerText    *string
	Score         *float64
	VerdictDetail *string
	LatencyMs     int
	InputTokens   *int
	OutputTokens  *int
	CreatedAt     time.Time
	ModelDeleted  bool
}

// evalRunColumns is the canonical eval_runs column list. "trigger" is a
// reserved SQLite keyword and must stay quoted.
const evalRunColumns = `id, campaign_id, suite_id, suite_version, "trigger", judge_model, status, started_at, finished_at`

// caseColumns is the canonical cases column list.
const caseColumns = `id, suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, difficulty, sample_count, enabled, created_at`

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

// scanEvalRun scans one eval_runs row.
func scanEvalRun(s rowScanner) (EvalRun, error) {
	var r EvalRun
	var startedAt string
	var finishedAt sql.NullString
	if err := s.Scan(&r.ID, &r.CampaignID, &r.SuiteID, &r.SuiteVersion, &r.Trigger, &r.JudgeModel, &r.Status, &startedAt, &finishedAt); err != nil {
		return EvalRun{}, err
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		r.FinishedAt = &t
	}
	return r, nil
}

// ListSuites returns all suites ordered by id.
func (db *DB) ListSuites() ([]Suite, error) {
	rows, err := db.conn.Query("SELECT id, key, name, version FROM suites ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suites []Suite
	for rows.Next() {
		var s Suite
		if err := rows.Scan(&s.ID, &s.Key, &s.Name, &s.Version); err != nil {
			return nil, err
		}
		suites = append(suites, s)
	}
	return suites, rows.Err()
}

// GetSuite retrieves a suite by ID.
func (db *DB) GetSuite(id int64) (*Suite, error) {
	var s Suite
	err := db.conn.QueryRow("SELECT id, key, name, version FROM suites WHERE id = ?", id).
		Scan(&s.ID, &s.Key, &s.Name, &s.Version)
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

// CreateEvalRun inserts a run in "running" status under the given campaign
// and returns it. The suite's current version is snapshotted onto the run.
func (db *DB) CreateEvalRun(campaignID, suiteID int64, trigger, judgeModel string) (*EvalRun, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(`
		INSERT INTO eval_runs (campaign_id, suite_id, suite_version, "trigger", judge_model, status, started_at)
		VALUES (?, ?, (SELECT version FROM suites WHERE id = ?), ?, ?, 'running', ?)
	`, campaignID, suiteID, suiteID, trigger, judgeModel, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetEvalRun(id)
}

// GetEvalRun retrieves a run by ID.
func (db *DB) GetEvalRun(id int64) (*EvalRun, error) {
	r, err := scanEvalRun(db.conn.QueryRow(
		"SELECT "+evalRunColumns+" FROM eval_runs WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListEvalRuns returns all runs, newest first.
func (db *DB) ListEvalRuns() ([]EvalRun, error) {
	rows, err := db.conn.Query("SELECT " + evalRunColumns + " FROM eval_runs ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []EvalRun
	for rows.Next() {
		r, err := scanEvalRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// SetEvalRunJudgeModel records the judge model a run actually used. The
// evaluator calls this at run start when the configured judge differs from
// the value snapshot at creation time.
func (db *DB) SetEvalRunJudgeModel(id int64, judgeModel string) error {
	_, err := db.conn.Exec("UPDATE eval_runs SET judge_model = ? WHERE id = ?", judgeModel, id)
	return err
}

// ListLatestEvalScores returns, for every (suite, model) pair that has at
// least one done run, the aggregate score of the most recent such run. The
// aggregate is the average of the pair's non-null scores inside that run
// (null when nothing was scored), matching the read-time run aggregation.
// Pairs whose model no longer exists in the models table are excluded: the
// comparison view only shows live models, while run history keeps the
// deleted model's rows.
func (db *DB) ListLatestEvalScores() ([]LatestEvalScore, error) {
	rows, err := db.conn.Query(`
		SELECT suite_id, suite_key, model_db_id, model_id, eval_run_id, finished_at, score
		FROM (
			SELECT r.suite_id AS suite_id, s.key AS suite_key,
				res.model_db_id AS model_db_id, res.model_id AS model_id,
				r.id AS eval_run_id, r.finished_at AS finished_at,
				AVG(res.score) AS score,
				ROW_NUMBER() OVER (
					PARTITION BY r.suite_id, res.model_db_id ORDER BY r.id DESC
				) AS rn
			FROM eval_runs r
			JOIN eval_results res ON res.eval_run_id = r.id
			JOIN suites s ON s.id = r.suite_id
			JOIN models m ON m.id = res.model_db_id
			WHERE r.status = 'done'
			GROUP BY r.id, res.model_db_id
		)
		WHERE rn = 1
		ORDER BY suite_id, model_db_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LatestEvalScore
	for rows.Next() {
		var ls LatestEvalScore
		var finishedAt string
		if err := rows.Scan(&ls.SuiteID, &ls.SuiteKey, &ls.ModelDBID, &ls.ModelID,
			&ls.EvalRunID, &finishedAt, &ls.Score); err != nil {
			return nil, err
		}
		ls.FinishedAt, _ = time.Parse(time.RFC3339, finishedAt)
		out = append(out, ls)
	}
	return out, rows.Err()
}

// PreviousDoneScore returns the aggregate score of the latest done run for a
// (suite, model) pair strictly before the given run, or (nil, 0, nil) when no
// earlier done run covered the pair. The score-drop alert compares against
// this baseline.
func (db *DB) PreviousDoneScore(suiteID, modelDBID, beforeRunID int64) (*float64, int64, error) {
	var runID int64
	var score *float64
	err := db.conn.QueryRow(`
		SELECT r.id, AVG(res.score)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		WHERE r.status = 'done' AND r.suite_id = ? AND res.model_db_id = ? AND r.id < ?
		GROUP BY r.id
		ORDER BY r.id DESC
		LIMIT 1
	`, suiteID, modelDBID, beforeRunID).Scan(&runID, &score)
	if err == sql.ErrNoRows {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return score, runID, nil
}

// FinishEvalRun marks a run done/failed with its finish time.
func (db *DB) FinishEvalRun(id int64, status string, finishedAt time.Time) error {
	_, err := db.conn.Exec(
		"UPDATE eval_runs SET status = ?, finished_at = ? WHERE id = ?",
		status, finishedAt.UTC().Format(time.RFC3339), id)
	return err
}

// CreateEvalResult inserts one result row and returns the stored copy.
func (db *DB) CreateEvalResult(r EvalResult) (*EvalResult, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(`
		INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, answer_text, score, verdict_detail, latency_ms, input_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.EvalRunID, r.ModelDBID, r.ModelID, r.CaseID, r.AnswerText, r.Score, r.VerdictDetail,
		r.LatencyMs, r.InputTokens, r.OutputTokens, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	r.ID = id
	r.CreatedAt = now
	return &r, nil
}

// ListEvalResults returns all results of a run ordered by id. Each row is
// flagged with whether its model still exists in the models table.
func (db *DB) ListEvalResults(runID int64) ([]EvalResult, error) {
	rows, err := db.conn.Query(`
		SELECT res.id, res.eval_run_id, res.model_db_id, res.model_id, res.case_id,
			res.answer_text, res.score, res.verdict_detail, res.latency_ms,
			res.input_tokens, res.output_tokens, res.created_at,
			CASE WHEN m.id IS NULL THEN 1 ELSE 0 END AS model_deleted
		FROM eval_results res
		LEFT JOIN models m ON m.id = res.model_db_id
		WHERE res.eval_run_id = ? ORDER BY res.id
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EvalResult
	for rows.Next() {
		var r EvalResult
		var createdAt string
		var deleted int
		if err := rows.Scan(&r.ID, &r.EvalRunID, &r.ModelDBID, &r.ModelID, &r.CaseID,
			&r.AnswerText, &r.Score, &r.VerdictDetail, &r.LatencyMs,
			&r.InputTokens, &r.OutputTokens, &createdAt, &deleted); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.ModelDeleted = deleted == 1
		results = append(results, r)
	}
	return results, rows.Err()
}

// HasScheduledEvalRunSince reports whether any scheduled eval run started at
// or after the given time. The weekly worker uses it to stay idempotent
// across restarts inside the Sunday window.
func (db *DB) HasScheduledEvalRunSince(since time.Time) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM eval_runs WHERE "trigger" = 'scheduled' AND started_at >= ?`,
		since.Format(time.RFC3339),
	).Scan(&count)
	return count > 0, err
}
