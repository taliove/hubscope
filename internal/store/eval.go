package store

import (
	"database/sql"
	"time"
)

// Suite is an evaluation suite: a named group of cases along one capability
// dimension (basic / reasoning / coding / chinese).
type Suite struct {
	ID   int64
	Key  string
	Name string
}

// Case is a single evaluation question with its verdict configuration.
// RuleMode/RuleExpected are set for verdict_type="rule"; Rubric for "judge".
type Case struct {
	ID           int64
	SuiteID      int64
	Prompt       string
	VerdictType  string
	RuleMode     *string
	RuleExpected *string
	Rubric       *string
	Enabled      bool
	CreatedAt    time.Time
}

// EvalRun is one execution of a suite against a set of models.
type EvalRun struct {
	ID         int64
	SuiteID    int64
	Trigger    string
	JudgeModel string
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
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
}

// evalRunColumns is the canonical eval_runs column list. "trigger" is a
// reserved SQLite keyword and must stay quoted.
const evalRunColumns = `id, suite_id, "trigger", judge_model, status, started_at, finished_at`

// scanCase scans one cases row.
func scanCase(s rowScanner) (Case, error) {
	var c Case
	var enabled int
	var createdAt string
	if err := s.Scan(&c.ID, &c.SuiteID, &c.Prompt, &c.VerdictType,
		&c.RuleMode, &c.RuleExpected, &c.Rubric, &enabled, &createdAt); err != nil {
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
	if err := s.Scan(&r.ID, &r.SuiteID, &r.Trigger, &r.JudgeModel, &r.Status, &startedAt, &finishedAt); err != nil {
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
	rows, err := db.conn.Query("SELECT id, key, name FROM suites ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suites []Suite
	for rows.Next() {
		var s Suite
		if err := rows.Scan(&s.ID, &s.Key, &s.Name); err != nil {
			return nil, err
		}
		suites = append(suites, s)
	}
	return suites, rows.Err()
}

// GetSuite retrieves a suite by ID.
func (db *DB) GetSuite(id int64) (*Suite, error) {
	var s Suite
	err := db.conn.QueryRow("SELECT id, key, name FROM suites WHERE id = ?", id).
		Scan(&s.ID, &s.Key, &s.Name)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListCases returns all cases of a suite (enabled and disabled), by id.
func (db *DB) ListCases(suiteID int64) ([]Case, error) {
	return db.listCases("SELECT id, suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, enabled, created_at FROM cases WHERE suite_id = ? ORDER BY id", suiteID)
}

// ListEnabledCases returns only the enabled cases of a suite, for execution.
func (db *DB) ListEnabledCases(suiteID int64) ([]Case, error) {
	return db.listCases("SELECT id, suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, enabled, created_at FROM cases WHERE suite_id = ? AND enabled = 1 ORDER BY id", suiteID)
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
		"SELECT id, suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, enabled, created_at FROM cases WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateCase inserts a case and returns the stored copy.
func (db *DB) CreateCase(c Case) (*Case, error) {
	now := time.Now().UTC()
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	result, err := db.conn.Exec(`
		INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, c.SuiteID, c.Prompt, c.VerdictType, c.RuleMode, c.RuleExpected, c.Rubric, enabled, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	c.ID = id
	c.CreatedAt = now
	return &c, nil
}

// UpdateCase replaces a case's editable fields (matched by c.ID) and returns
// the stored copy. SuiteID and CreatedAt are preserved from the input.
func (db *DB) UpdateCase(c Case) (*Case, error) {
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	if _, err := db.conn.Exec(`
		UPDATE cases SET prompt = ?, verdict_type = ?, rule_mode = ?, rule_expected = ?, rubric = ?, enabled = ?
		WHERE id = ?
	`, c.Prompt, c.VerdictType, c.RuleMode, c.RuleExpected, c.Rubric, enabled, c.ID); err != nil {
		return nil, err
	}
	return db.GetCase(c.ID)
}

// CreateEvalRun inserts a run in "running" status and returns it.
func (db *DB) CreateEvalRun(suiteID int64, trigger, judgeModel string) (*EvalRun, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(`
		INSERT INTO eval_runs (suite_id, "trigger", judge_model, status, started_at)
		VALUES (?, ?, ?, 'running', ?)
	`, suiteID, trigger, judgeModel, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &EvalRun{
		ID:         id,
		SuiteID:    suiteID,
		Trigger:    trigger,
		JudgeModel: judgeModel,
		Status:     "running",
		StartedAt:  now,
	}, nil
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

// ListEvalResults returns all results of a run ordered by id.
func (db *DB) ListEvalResults(runID int64) ([]EvalResult, error) {
	rows, err := db.conn.Query(`
		SELECT id, eval_run_id, model_db_id, model_id, case_id, answer_text, score, verdict_detail, latency_ms, input_tokens, output_tokens, created_at
		FROM eval_results WHERE eval_run_id = ? ORDER BY id
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EvalResult
	for rows.Next() {
		var r EvalResult
		var createdAt string
		if err := rows.Scan(&r.ID, &r.EvalRunID, &r.ModelDBID, &r.ModelID, &r.CaseID,
			&r.AnswerText, &r.Score, &r.VerdictDetail, &r.LatencyMs,
			&r.InputTokens, &r.OutputTokens, &createdAt); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
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
