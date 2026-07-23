package store

// CampaignMember is one model of a campaign's creation-time membership
// snapshot (ticket 53): the population the batch set out to evaluate,
// recorded up front so the progress grid can list every member from the
// first run on — a member the sweep has not reached yet renders "pending"
// instead of staying invisible until its first result lands.
type CampaignMember struct {
	ModelDBID int64
	ModelID   string
	Family    string
}

// ListCampaignMembers returns a campaign's snapshotted model membership in
// model-id lexicographic order (the live board's convention), with the
// presentation filters applied: deleted models vanish with their models row
// (inner join), retired models drop out. The snapshot itself is a historical
// fact and is never rewritten; hiding deleted/retired models is a
// presentation concern (ticket 26 semantics), applied here at read time so
// the member-driven board matches the leaderboard's caliber.
func (db *DB) ListCampaignMembers(campaignID int64) ([]CampaignMember, error) {
	rows, err := db.conn.Query(`
		SELECT m.id, m.model_id, m.family
		FROM campaign_models cm
		JOIN models m ON m.id = cm.model_id
		WHERE cm.campaign_id = ? AND m.status != 'retired'
		ORDER BY m.model_id
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CampaignMember
	for rows.Next() {
		var m CampaignMember
		if err := rows.Scan(&m.ModelDBID, &m.ModelID, &m.Family); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// backfillCampaignMembers derives the membership of campaigns created before
// the snapshot existed (ticket 53) from their recorded results: every model
// that produced at least one result in one of the campaign's runs becomes a
// member. Models a run never reached cannot be recovered and stay absent —
// for those campaigns the member-driven board degrades to the old
// results-derived behavior. INSERT OR IGNORE over the table's primary key
// keeps the migration idempotent across restarts. Runs before the ticket 29
// backfill would leave orphan members pointing at campaign 0, so migrate()
// calls this only after backfillRunCampaigns.
func (db *DB) backfillCampaignMembers() error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO campaign_models (campaign_id, model_id)
		SELECT DISTINCT r.campaign_id, res.model_db_id
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
	`)
	return err
}
