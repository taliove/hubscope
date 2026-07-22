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
type CampaignCellProgress struct {
	RunID       int64
	SuiteID     int64
	ModelDBID   int64
	ModelID     string
	Family      string
	JudgedCases int
	TotalCases  int
	Samples     int
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
			COALESCE(SUM(CASE WHEN res.score IS NOT NULL THEN COALESCE(c.sample_count, 1) ELSE 0 END), 0)
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
			&p.JudgedCases, &p.TotalCases, &p.Samples); err != nil {
			return nil, err
		}
		out = append(out, p)
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
	rows, err := db.conn.Query(`
		SELECT res.model_db_id, res.model_id, m.family, s.key, AVG(res.score)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		JOIN suites s ON s.id = r.suite_id
		JOIN models m ON m.id = res.model_db_id
		WHERE r.campaign_id = ? AND r.status = 'done' AND m.status != 'retired'
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
