package store

import (
	"database/sql"
	"strings"
	"time"
)

// EvalRun is one execution of a suite against a set of models. SuiteVersion
// snapshots the suite's version at creation, so a run is always attributable
// to the question-bank version it scored. Nadir snapshots the suite's nadir
// the same way, so historical runs keep the normalization constant they were
// scored with even after the suite is recalibrated (ADR 0009/0010).
// CampaignID links the run to its evaluation batch (Eval Campaign); every
// run belongs to exactly one.
type EvalRun struct {
	ID           int64
	CampaignID   int64
	SuiteID      int64
	SuiteVersion int
	Nadir        float64
	Trigger      string
	JudgeModel   string
	Status       string
	StartedAt    time.Time
	FinishedAt   *time.Time
	// JuryModels snapshots the run's jury selection (JSON: policy + per-model
	// judges; spec 0020 / ADR 0016). Empty for pre-jury single-judge runs.
	JuryModels string
	// EstimatedCost holds the run's estimated cost as a JSON split
	// {"exam":x,"judge":y}; empty when unset or when some component's price
	// is not registered (GH #178).
	EstimatedCost string
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

// Verdict profile versions (ADR 0008): the normalization pipeline a result's
// rule verdict was scored with. V1 is the legacy TrimSpace-only caliber every
// pre-migration row belongs to; V2 is the full pipeline (trim, paired-quote
// stripping, NFKC, whitespace collapse). The pipeline itself lives in the
// evaluator package; the constants live here because the eval_results column
// default and the migration backfill speak the same vocabulary.
//
// Version strings must stay lexicographically ordered by version: the store
// resolves "the newest profile" as MAX(verdict_profile) in SQL (see
// CampaignVerdictProfiles and ListModelTrend), so lexicographic order is the
// version order. A future v10 would sort before v2 — either zero-pad (v09,
// v10) or switch those queries to a numeric comparison when the time comes.
const (
	VerdictProfileV1 = "v1"
	VerdictProfileV2 = "v2"
	// VerdictProfileV3 is the jury-median caliber (spec 0020, ADR 0016):
	// judge cases are scored by up to three judges and the sample score is
	// their median. Rule verdicts stay V2 — the pipeline did not change.
	VerdictProfileV3 = "v3"
)

// EvalResult is the outcome of one (model, case) pair inside a run. Score is
// nil when the case could not be judged (answer call failed, judge failed).
// ModelDeleted reports whether the model has since been removed from the
// models table; the run history keeps the row so the UI can badge it.
// VerdictProfile records the scoring caliber (ADR 0008); rows predating the
// column are backfilled to VerdictProfileV1.
type EvalResult struct {
	ID             int64
	EvalRunID      int64
	ModelDBID      int64
	ModelID        string
	CaseID         int64
	AnswerText     *string
	Score          *float64
	VerdictDetail  *string
	VerdictProfile string
	LatencyMs      int
	InputTokens    *int
	OutputTokens   *int
	CreatedAt      time.Time
	ModelDeleted   bool
}

// evalRunColumns is the canonical eval_runs column list. "trigger" is a
// reserved SQLite keyword and must stay quoted.
const evalRunColumns = `id, campaign_id, suite_id, suite_version, nadir, "trigger", judge_model, status, started_at, finished_at, jury_models, estimated_cost`

// scanEvalRun scans one eval_runs row.
func scanEvalRun(s rowScanner) (EvalRun, error) {
	var r EvalRun
	var startedAt string
	var finishedAt, juryModels, estimatedCost sql.NullString
	if err := s.Scan(&r.ID, &r.CampaignID, &r.SuiteID, &r.SuiteVersion, &r.Nadir, &r.Trigger, &r.JudgeModel, &r.Status, &startedAt, &finishedAt, &juryModels, &estimatedCost); err != nil {
		return EvalRun{}, err
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		r.FinishedAt = &t
	}
	r.JuryModels = juryModels.String
	r.EstimatedCost = estimatedCost.String
	return r, nil
}

// CreateEvalRun inserts a run in "running" status under the given campaign
// and returns it. The suite's current version and nadir are snapshotted onto
// the run, so later question-bank rotations or recalibrations never rewrite
// what a historical run scored against (ADR 0007/0009).
func (db *DB) CreateEvalRun(campaignID, suiteID int64, trigger, judgeModel string) (*EvalRun, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(`
		INSERT INTO eval_runs (campaign_id, suite_id, suite_version, nadir, "trigger", judge_model, status, started_at)
		VALUES (?, ?, (SELECT version FROM suites WHERE id = ?), (SELECT nadir FROM suites WHERE id = ?), ?, ?, 'running', ?)
	`, campaignID, suiteID, suiteID, suiteID, trigger, judgeModel, now.Format(time.RFC3339))
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

// RunUnitProgress pairs a run's campaign linkage with its (model, case)
// unit completion, powering the task center's batch deep link and live
// progress (GH #156): DoneUnits counts the units with at least one result
// row, TotalUnits is the campaign's member models x the suite's enabled
// cases.
type RunUnitProgress struct {
	CampaignID int64
	DoneUnits  int
	TotalUnits int
}

// GetRunUnitProgress resolves campaign linkage and unit completion for a
// batch of run ids in one aggregate query — the task center calls it once
// per listed page, never per row (no N+1). Unknown ids are simply absent
// from the result.
func (db *DB) GetRunUnitProgress(ids []int64) (map[int64]RunUnitProgress, error) {
	out := make(map[int64]RunUnitProgress, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.Query(`
		SELECT r.id, r.campaign_id,
			(SELECT COUNT(*) FROM (
				SELECT DISTINCT model_db_id, case_id FROM eval_results WHERE eval_run_id = r.id
			)) AS done_units,
			(SELECT COUNT(*) FROM campaign_models cm WHERE cm.campaign_id = r.campaign_id)
				* (SELECT COUNT(*) FROM cases c WHERE c.suite_id = r.suite_id AND c.enabled = 1) AS total_units
		FROM eval_runs r
		WHERE r.id IN (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p RunUnitProgress
		if err := rows.Scan(&id, &p.CampaignID, &p.DoneUnits, &p.TotalUnits); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// ListEvalRunsAll returns every run, newest first. It is the super_admin /
// store-internal counterpart of ListEvalRunsByHub; HTTP handlers must pick
// the form based on the session's hub scope.
func (db *DB) ListEvalRunsAll() ([]EvalRun, error) {
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

// ListEvalRunsByHub returns the runs whose campaign membership includes at
// least one model belonging to hubID, newest first. The join chain is
// eval_runs -> campaign_models -> models.hub_id (the shortest hub-reaching
// path; eval_results is deliberately avoided to avoid dropping in-flight
// runs that have no results yet). HTTP handlers must use this form for
// non-super_admin sessions.
func (db *DB) ListEvalRunsByHub(hubID int64) ([]EvalRun, error) {
	rows, err := db.conn.Query(`
		SELECT `+evalRunColumns+` FROM eval_runs r
		WHERE EXISTS (
			SELECT 1 FROM campaign_models cm
			JOIN models m ON m.id = cm.model_id
			WHERE cm.campaign_id = r.campaign_id AND m.hub_id = ?
		)
		ORDER BY r.id DESC
	`, hubID)
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
	return db.listLatestEvalScores(0)
}

// ListLatestEvalScoresByHub is the hub-scoped counterpart of
// ListLatestEvalScores: it restricts the (suite, model) pairs to models
// belonging to hubID. HTTP handlers must use this form for non-super_admin
// sessions.
func (db *DB) ListLatestEvalScoresByHub(hubID int64) ([]LatestEvalScore, error) {
	return db.listLatestEvalScores(hubID)
}

// listLatestEvalScores is the shared implementation. hubID is 0 for the
// unscoped (all) variant — hub IDs are AUTOINCREMENT from 1, so 0 never
// matches a real hub — or the hubID parameter for the hub-scoped variant.
func (db *DB) listLatestEvalScores(hubID int64) ([]LatestEvalScore, error) {
	hubFilter := ""
	var args []interface{}
	if hubID != 0 {
		hubFilter = "AND m.hub_id = ?"
		args = append(args, hubID)
	}
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
			WHERE r.status = 'done' `+hubFilter+`
			GROUP BY r.id, res.model_db_id
		)
		WHERE rn = 1
		ORDER BY suite_id, model_db_id
	`, args...)
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

// FinishEvalRun marks a run done/failed with its finish time.
func (db *DB) FinishEvalRun(id int64, status string, finishedAt time.Time) error {
	_, err := db.conn.Exec(
		"UPDATE eval_runs SET status = ?, finished_at = ? WHERE id = ?",
		status, finishedAt.UTC().Format(time.RFC3339), id)
	return err
}

// CreateEvalResult inserts one result row and returns the stored copy. An
// empty VerdictProfile falls back to V1, the same default the column
// migration backfills legacy rows with; the evaluator always tags explicitly.
func (db *DB) CreateEvalResult(r EvalResult) (*EvalResult, error) {
	now := time.Now().UTC()
	profile := r.VerdictProfile
	if profile == "" {
		profile = VerdictProfileV1
	}
	result, err := db.conn.Exec(`
		INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, answer_text, score, verdict_detail, verdict_profile, latency_ms, input_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.EvalRunID, r.ModelDBID, r.ModelID, r.CaseID, r.AnswerText, r.Score, r.VerdictDetail,
		profile, r.LatencyMs, r.InputTokens, r.OutputTokens, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	r.ID = id
	r.VerdictProfile = profile
	r.CreatedAt = now
	return &r, nil
}

// CreateEvalResultsBatch inserts result rows in a single transaction
// (GH #150): the failure-stamp paths (setup failure, circuit breaker)
// write whole case-set rows at once, and a prepared statement inside one
// tx is far cheaper than per-row implicit transactions. Live per-case
// scoring keeps CreateEvalResult so the progress grid and live feed
// update case by case. Profiles fall back to V1 like the single-row path.
func (db *DB) CreateEvalResultsBatch(rs []EvalResult) error {
	if len(rs) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO eval_results (eval_run_id, model_db_id, model_id, case_id, answer_text, score, verdict_detail, verdict_profile, latency_ms, input_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range rs {
		profile := r.VerdictProfile
		if profile == "" {
			profile = VerdictProfileV1
		}
		if _, err := stmt.Exec(r.EvalRunID, r.ModelDBID, r.ModelID, r.CaseID, r.AnswerText,
			r.Score, r.VerdictDetail, profile, r.LatencyMs, r.InputTokens, r.OutputTokens, now); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ListEvalRunAverageScores returns each requested run's mean score in one
// aggregate query (GH #151): SQLite AVG skips NULL scores, so unjudged
// cases never drag the mean — the same convention as averageScore, whose
// nadir normalization stays a read-side concern of the caller. Runs with
// no scored result are absent from the map (nil mean).
func (db *DB) ListEvalRunAverageScores(runIDs []int64) (map[int64]float64, error) {
	out := map[int64]float64{}
	if len(runIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(runIDs))
	args := make([]interface{}, len(runIDs))
	for i, id := range runIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.Query(
		"SELECT eval_run_id, AVG(score) FROM eval_results WHERE eval_run_id IN ("+
			strings.Join(placeholders, ",")+") GROUP BY eval_run_id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		var avg *float64
		if err := rows.Scan(&runID, &avg); err != nil {
			return nil, err
		}
		if avg != nil {
			out[runID] = *avg
		}
	}
	return out, rows.Err()
}

// ListEvalResults returns all results of a run ordered by id. Each row is
// flagged with whether its model still exists in the models table.
func (db *DB) ListEvalResults(runID int64) ([]EvalResult, error) {
	rows, err := db.conn.Query(`
		SELECT res.id, res.eval_run_id, res.model_db_id, res.model_id, res.case_id,
			res.answer_text, res.score, res.verdict_detail, res.verdict_profile, res.latency_ms,
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
			&r.AnswerText, &r.Score, &r.VerdictDetail, &r.VerdictProfile, &r.LatencyMs,
			&r.InputTokens, &r.OutputTokens, &createdAt, &deleted); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.ModelDeleted = deleted == 1
		results = append(results, r)
	}
	return results, rows.Err()
}

// LiveFeedEntry is one judged-case event of a campaign (issue #17): the
// unit of the console live feed's cursor-pulled stream — model, suite and
// case identity, verdict method, the raw 0~1 per-case score (nil when the
// case could not be judged — W7's "judge failure is null, not zero"), the
// answer latency and the verdict time. VerdictType/CasePrompt come from the
// case row; a purged case leaves them empty rather than dropping the event.
type LiveFeedEntry struct {
	ID          int64
	ModelID     string
	SuiteKey    string
	SuiteName   string
	CaseID      int64
	CasePrompt  string
	VerdictType string
	Score       *float64
	LatencyMs   int
	CreatedAt   time.Time
}

// ListCampaignLiveFeed returns up to limit judged-case events of the
// campaign whose result id is strictly greater than sinceID, ascending by
// id — the cursor increment behind GET /api/campaigns/{id}/live-feed. The
// case join is LEFT so an event survives its case being purged (empty
// prompt/verdict), matching the run-history retention rule.
func (db *DB) ListCampaignLiveFeed(campaignID, sinceID int64, limit int) ([]LiveFeedEntry, error) {
	rows, err := db.conn.Query(`
		SELECT res.id, res.model_id, s.key, s.name, res.case_id,
			COALESCE(c.prompt, ''), COALESCE(c.verdict_type, ''),
			res.score, res.latency_ms, res.created_at
		FROM eval_results res
		JOIN eval_runs r ON r.id = res.eval_run_id
		JOIN suites s ON s.id = r.suite_id
		LEFT JOIN cases c ON c.id = res.case_id
		WHERE r.campaign_id = ? AND res.id > ?
		ORDER BY res.id
		LIMIT ?
	`, campaignID, sinceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []LiveFeedEntry{}
	for rows.Next() {
		var e LiveFeedEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ModelID, &e.SuiteKey, &e.SuiteName, &e.CaseID,
			&e.CasePrompt, &e.VerdictType, &e.Score, &e.LatencyMs, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// LiveFeedResultDetail is the on-demand expansion of one live-feed event
// (GH #41): the case's full prompt, the expectation by verdict method (the
// rule standard answer for rule cases, the rubric scoring points for judge
// cases), the model's answer text, the score and the verdict detail. The
// case join is LEFT so a purged case leaves prompt/expectation empty rather
// than dropping the result, matching the feed's retention rule.
type LiveFeedResultDetail struct {
	ID            int64
	CasePrompt    string
	VerdictType   string
	Expected      *string
	AnswerText    *string
	Score         *float64
	VerdictDetail *string
}

// GetCampaignLiveFeedResult returns the detail of one result of the
// campaign, or sql.ErrNoRows when the result does not exist or belongs to
// another campaign (the handler folds both into the same 404 — no cross-
// campaign oracle).
func (db *DB) GetCampaignLiveFeedResult(campaignID, resultID int64) (*LiveFeedResultDetail, error) {
	var d LiveFeedResultDetail
	err := db.conn.QueryRow(`
		SELECT res.id, COALESCE(c.prompt, ''), COALESCE(c.verdict_type, ''),
			CASE WHEN c.verdict_type = 'judge' THEN c.rubric ELSE c.rule_expected END,
			res.answer_text, res.score, res.verdict_detail
		FROM eval_results res
		JOIN eval_runs r ON r.id = res.eval_run_id
		LEFT JOIN cases c ON c.id = res.case_id
		WHERE r.campaign_id = ? AND res.id = ?
	`, campaignID, resultID).Scan(
		&d.ID, &d.CasePrompt, &d.VerdictType, &d.Expected, &d.AnswerText, &d.Score, &d.VerdictDetail)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// SetEvalRunVerdictProfile re-tags every result of a run with the given
// verdict profile. It exists so caliber migrations and tests can stage a
// run as scored under an older profile (ADR 0008); production scoring always
// writes the current profile at insert time and never calls this.
func (db *DB) SetEvalRunVerdictProfile(runID int64, profile string) error {
	_, err := db.conn.Exec(
		"UPDATE eval_results SET verdict_profile = ? WHERE eval_run_id = ?", profile, runID)
	return err
}

// NullScoreCell identifies one failed result of a run — a (model, case) unit
// whose stored score IS NULL (answer or judge failure, W7: never zero). It
// is the retry unit of GH #28's retry-failed path.
type NullScoreCell struct {
	ModelDBID int64
	ModelID   string
	CaseID    int64
}

// ListNullScoreCells returns every failed (null-score) result of the run,
// ordered by model then case, as retry units.
func (db *DB) ListNullScoreCells(runID int64) ([]NullScoreCell, error) {
	rows, err := db.conn.Query(`
		SELECT model_db_id, model_id, case_id FROM eval_results
		WHERE eval_run_id = ? AND score IS NULL
		ORDER BY model_db_id, case_id
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NullScoreCell
	for rows.Next() {
		var c NullScoreCell
		if err := rows.Scan(&c.ModelDBID, &c.ModelID, &c.CaseID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RetryUnit identifies one (model, case) unit a targeted retry is asked to
// re-evaluate (retry-units, the single-unit sibling of GH #28's batch
// retry-failed). Unlike NullScoreCell it carries no model string: it is a
// request-side selector, not a stored row.
type RetryUnit struct {
	ModelDBID int64
	CaseID    int64
}

// retryUnitWhere builds an OR-of-ANDs WHERE fragment over the (model_db_id,
// case_id) pairs plus its args, against the aliased eval_results columns.
// Callers pass 1..N units (bounded at the handler boundary); an empty slice
// would produce an empty clause and is a programming error.
func retryUnitWhere(alias string, units []RetryUnit) (string, []interface{}) {
	parts := make([]string, 0, len(units))
	args := make([]interface{}, 0, 2*len(units))
	for _, u := range units {
		parts = append(parts, "("+alias+".model_db_id = ? AND "+alias+".case_id = ?)")
		args = append(args, u.ModelDBID, u.CaseID)
	}
	return strings.Join(parts, " OR "), args
}

// CampaignNullScoreUnits returns the subset of the requested units that
// currently hold a failed (null-score) result in the campaign — exactly the
// units a targeted retry will re-evaluate. Units with a judged (non-null)
// score and units the campaign never recorded are both absent (W7: only
// null scores are retryable); the handler reports them as skipped.
func (db *DB) CampaignNullScoreUnits(campaignID int64, units []RetryUnit) (map[RetryUnit]struct{}, error) {
	where, args := retryUnitWhere("res", units)
	rows, err := db.conn.Query(`
		SELECT res.model_db_id, res.case_id FROM eval_results res
		JOIN eval_runs r ON r.id = res.eval_run_id
		WHERE r.campaign_id = ? AND res.score IS NULL AND (`+where+`)
	`, append([]interface{}{campaignID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[RetryUnit]struct{}, len(units))
	for rows.Next() {
		var u RetryUnit
		if err := rows.Scan(&u.ModelDBID, &u.CaseID); err != nil {
			return nil, err
		}
		out[u] = struct{}{}
	}
	return out, rows.Err()
}

// DeleteNullScoreResult removes the failed (null-score) result row of one
// (run, model, case) unit, immediately before its re-evaluation inserts the
// fresh row (GH #28). The score IS NULL clause is deliberately hardcoded —
// never a parameter — so no caller, present or future, can widen this into
// deleting a judged result (W7: scored rows are immutable).
func (db *DB) DeleteNullScoreResult(runID, modelDBID, caseID int64) error {
	_, err := db.conn.Exec(`
		DELETE FROM eval_results
		WHERE eval_run_id = ? AND model_db_id = ? AND case_id = ? AND score IS NULL
	`, runID, modelDBID, caseID)
	return err
}

// CampaignAnsweredUnits returns the subset of the requested units that
// already hold a result row of ANY score (2026-08-04 retry-any ruling):
// only those are re-answerable on demand — a unit without a row is
// mid-flight or pending and the running batch will answer it anyway.
func (db *DB) CampaignAnsweredUnits(campaignID int64, units []RetryUnit) (map[RetryUnit]struct{}, error) {
	where, args := retryUnitWhere("res", units)
	rows, err := db.conn.Query(`
		SELECT res.model_db_id, res.case_id FROM eval_results res
		JOIN eval_runs r ON r.id = res.eval_run_id
		WHERE r.campaign_id = ? AND (`+where+`)
	`, append([]interface{}{campaignID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[RetryUnit]struct{}, len(units))
	for rows.Next() {
		var u RetryUnit
		if err := rows.Scan(&u.ModelDBID, &u.CaseID); err != nil {
			return nil, err
		}
		out[u] = struct{}{}
	}
	return out, rows.Err()
}

// DeleteUnitResult removes a unit's result row regardless of score —
// exclusively for the operator-initiated re-answer flow (2026-08-04
// ruling), which re-answers and re-judges on explicit request. Contrast
// DeleteNullScoreResult (W7): the null-only guard there stays hardcoded;
// this method must never be called from any path but the retry-any one.
func (db *DB) DeleteUnitResult(runID, modelDBID, caseID int64) error {
	_, err := db.conn.Exec(`
		DELETE FROM eval_results
		WHERE eval_run_id = ? AND model_db_id = ? AND case_id = ?
	`, runID, modelDBID, caseID)
	return err
}

// HasResult reports whether the unit holds any result row.
func (db *DB) HasResult(runID, modelDBID, caseID int64) (bool, error) {
	var n int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM eval_results
		WHERE eval_run_id = ? AND model_db_id = ? AND case_id = ?
	`, runID, modelDBID, caseID).Scan(&n)
	return n > 0, err
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
