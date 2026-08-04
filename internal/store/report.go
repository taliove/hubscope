package store

// CampaignSuiteScore is one model's aggregate score in one suite within a
// campaign: the average of the model's non-null case scores across the
// campaign's done runs for that suite. Score is null when nothing was judged
// (every answer or judge call failed).
type CampaignSuiteScore struct {
	ModelDBID int64
	ModelID   string
	Family    string
	SuiteKey  string
	Score     *float64
}

// CampaignCellProgress is one model's recorded progress inside one run of a
// campaign: how many of the run's cases already produced a judged score and
// how many result rows exist at all. One result row per (model, case) —
// samples are averaged into a single stored result, so case-level coverage
// already accounts for sampling (ticket 52). Samples is the number of judged
// answer attempts behind the scored cases (the per-case sample count summed
// over judged cases, defaulting to 1 for cases without an explicit count);
// it is the second half of the score's confidence marker (ticket 51).
// LatencyMs/InputTokens/OutputTokens are the cell's cost sums (GH #42):
// Σ latency and Σ tokens over the model's results in the run, null tokens
// counted as 0.
type CampaignCellProgress struct {
	RunID        int64
	SuiteID      int64
	ModelDBID    int64
	ModelID      string
	Family       string
	JudgedCases  int
	TotalCases   int
	Samples      int
	LatencyMs    int64
	InputTokens  int64
	OutputTokens int64
}

// ListCampaignCellProgress aggregates a campaign's results per (run, model),
// the raw material of the progress grid and the live half-scored board
// (ticket 52). Unlike ListCampaignSuiteScores it covers runs of every
// status, so in-flight runs report their partial coverage. Deleted models
// stay invisible, matching leaderboard semantics (ticket 26).
func (db *DB) ListCampaignCellProgress(campaignID int64) ([]CampaignCellProgress, error) {
	rows, err := db.conn.Query(`
		SELECT r.id, r.suite_id, res.model_db_id, res.model_id, m.family,
			COALESCE(SUM(CASE WHEN res.score IS NOT NULL THEN 1 ELSE 0 END), 0),
			COUNT(res.id),
			COALESCE(SUM(CASE WHEN res.score IS NOT NULL THEN COALESCE(c.sample_count, 1) ELSE 0 END), 0),
			COALESCE(SUM(res.latency_ms), 0),
			COALESCE(SUM(res.input_tokens), 0),
			COALESCE(SUM(res.output_tokens), 0)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		JOIN models m ON m.id = res.model_db_id
		JOIN cases c ON c.id = res.case_id
		WHERE r.campaign_id = ? AND m.status != 'retired'
		GROUP BY r.id, res.model_db_id
		ORDER BY res.model_id, r.id
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CampaignCellProgress
	for rows.Next() {
		var p CampaignCellProgress
		if err := rows.Scan(&p.RunID, &p.SuiteID, &p.ModelDBID, &p.ModelID, &p.Family,
			&p.JudgedCases, &p.TotalCases, &p.Samples,
			&p.LatencyMs, &p.InputTokens, &p.OutputTokens); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CampaignCostTotals is the batch-level cost summary (GH #42): Σ latency
// and Σ input/output tokens over every result of the campaign, null tokens
// counted as 0. Retired models are excluded, the same filter the cells and
// the leaderboard apply, so the batch total always equals the sum of the
// visible cells.
type CampaignCostTotals struct {
	LatencyMs    int64
	InputTokens  int64
	OutputTokens int64
}

// CampaignCostTotals aggregates the campaign-wide cost summary.
func (db *DB) CampaignCostTotals(campaignID int64) (CampaignCostTotals, error) {
	var out CampaignCostTotals
	err := db.conn.QueryRow(`
		SELECT COALESCE(SUM(res.latency_ms), 0),
			COALESCE(SUM(res.input_tokens), 0),
			COALESCE(SUM(res.output_tokens), 0)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		JOIN models m ON m.id = res.model_db_id
		WHERE r.campaign_id = ? AND m.status != 'retired'
	`, campaignID).Scan(&out.LatencyMs, &out.InputTokens, &out.OutputTokens)
	return out, err
}

// CampaignCostRow is one model's cost inside one run of a campaign (GH #42)
// — the report page's per-(model, suite-run) detail table. InputTokens and
// OutputTokens stay nil when the model recorded no token at all in the run
// (the detail table renders a dash), while the batch and cell sums count
// those same nulls as 0. AvgTPS is the run's mean output speed over
// answered samples (GH #178); nil when the model produced no answer.
type CampaignCostRow struct {
	ModelID      string
	SuiteKey     string
	SuiteName    string
	RunStatus    string
	LatencyMs    int64
	InputTokens  *int64
	OutputTokens *int64
	AvgTPS       *float64
}

// ListCampaignCostRows aggregates a campaign's cost per (run, model), the
// raw material of the report page's cost detail table. Runs of every status
// contribute (a failed run still spent latency and tokens); retired models
// are excluded, matching leaderboard semantics.
func (db *DB) ListCampaignCostRows(campaignID int64) ([]CampaignCostRow, error) {
	rows, err := db.conn.Query(`
		SELECT res.model_id, s.key, s.name, r.status,
			COALESCE(SUM(res.latency_ms), 0),
			SUM(res.input_tokens), SUM(res.output_tokens),
			(SELECT CAST(SUM(a.output_tokens) AS REAL) * 1000 / NULLIF(SUM(a.latency_ms), 0)
			 FROM eval_answers a
			 WHERE a.eval_run_id = r.id AND a.model_db_id = res.model_db_id AND a.status = 'answered')
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		JOIN suites s ON s.id = r.suite_id
		JOIN models m ON m.id = res.model_db_id
		WHERE r.campaign_id = ? AND m.status != 'retired'
		GROUP BY r.id, res.model_db_id
		ORDER BY res.model_id, s.key
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CampaignCostRow{}
	for rows.Next() {
		var cr CampaignCostRow
		if err := rows.Scan(&cr.ModelID, &cr.SuiteKey, &cr.SuiteName, &cr.RunStatus,
			&cr.LatencyMs, &cr.InputTokens, &cr.OutputTokens, &cr.AvgTPS); err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return out, rows.Err()
}

// CampaignVerdictProfiles maps each suite covered by a campaign's done runs
// to the newest verdict profile its results were scored with (ADR 0008 —
// "newest" is the lexicographic max, the same convention as ListModelTrend).
// The report baseline uses it to detect a scoring-caliber break between
// adjacent batches: different profiles mean the two sides are not comparable
// even when the question-bank version never moved. Suites without results
// are absent from the map.
func (db *DB) CampaignVerdictProfiles(campaignID int64) (map[int64]string, error) {
	rows, err := db.conn.Query(`
		SELECT r.suite_id, MAX(res.verdict_profile)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		WHERE r.campaign_id = ? AND r.status = 'done'
		GROUP BY r.suite_id
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]string{}
	for rows.Next() {
		var suiteID int64
		var profile string
		if err := rows.Scan(&suiteID, &profile); err != nil {
			return nil, err
		}
		out[suiteID] = profile
	}
	return out, rows.Err()
}

// ListCampaignSuiteScores aggregates a campaign's done runs into per-(model,
// suite) scores, the raw material of the leaderboard (ticket 31). Deleted
// models never rank (ticket 26 semantics): rows whose model vanished from
// the models table or carries status "retired" are excluded. NULL scores
// (unjudged cases) never enter the average — SQLite AVG skips NULLs, the
// same convention as the read-time run aggregation.
func (db *DB) ListCampaignSuiteScores(campaignID int64) ([]CampaignSuiteScore, error) {
	return db.listCampaignSuiteScores(campaignID, "r.status = 'done'")
}

// ListCampaignLiveSuiteScores additionally counts running runs (GH #179,
// 2026-08-04 UX ruling): a model's partial suite score is visible as soon
// as its first case settles — "跑了一题就算分". The cells' coverage markers
// already name the judging denominator, so the partial mean never poses as
// complete.
func (db *DB) ListCampaignLiveSuiteScores(campaignID int64) ([]CampaignSuiteScore, error) {
	return db.listCampaignSuiteScores(campaignID, "r.status IN ('done', 'running')")
}

func (db *DB) listCampaignSuiteScores(campaignID int64, statusFilter string) ([]CampaignSuiteScore, error) {
	rows, err := db.conn.Query(`
		SELECT res.model_db_id, res.model_id, m.family, s.key, AVG(res.score)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		JOIN suites s ON s.id = r.suite_id
		JOIN models m ON m.id = res.model_db_id
		WHERE r.campaign_id = ? AND `+statusFilter+` AND m.status != 'retired'
		GROUP BY res.model_db_id, r.suite_id
		ORDER BY res.model_db_id, r.suite_id
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CampaignSuiteScore
	for rows.Next() {
		var cs CampaignSuiteScore
		if err := rows.Scan(&cs.ModelDBID, &cs.ModelID, &cs.Family, &cs.SuiteKey, &cs.Score); err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}
