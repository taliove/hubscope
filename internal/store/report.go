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
