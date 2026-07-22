package store

import (
	"database/sql"
	"time"
)

// Campaign status vocabulary. A campaign starts running as soon as it is
// created (its runs execute immediately); "pending" is reserved for future
// queued campaigns. Terminal states are done (every run done) and failed
// (any run failed once no run is still active).
const (
	CampaignStatusPending = "pending"
	CampaignStatusRunning = "running"
	CampaignStatusDone    = "done"
	CampaignStatusFailed  = "failed"
)

// Campaign is one evaluation batch: the aggregate unit over a set of eval
// runs, usually one run per suite. Its status is persisted (recomputed from
// member runs when the batch settles), never derived on read.
type Campaign struct {
	ID         int64
	Trigger    string
	Status     string
	StartedAt  *time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
}

// CampaignProgress counts a campaign's member runs by status bucket.
type CampaignProgress struct {
	Total   int
	Done    int
	Failed  int
	Running int
}

// CampaignWithProgress is a campaign plus the live per-status count of its
// member runs, as served by the list endpoint.
type CampaignWithProgress struct {
	Campaign
	Progress CampaignProgress
}

// campaignColumns is the canonical campaigns column list. "trigger" is a
// reserved SQLite keyword and must stay quoted.
const campaignColumns = `id, "trigger", status, started_at, finished_at, created_at`

// scanCampaign scans one campaigns row.
func scanCampaign(s rowScanner) (Campaign, error) {
	var c Campaign
	var startedAt, finishedAt sql.NullString
	var createdAt string
	if err := s.Scan(&c.ID, &c.Trigger, &c.Status, &startedAt, &finishedAt, &createdAt); err != nil {
		return Campaign{}, err
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		c.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		c.FinishedAt = &t
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return c, nil
}

// CreateCampaign inserts a campaign in "running" status, stamped with the
// given start time, and returns the stored copy.
func (db *DB) CreateCampaign(trigger string, now time.Time) (*Campaign, error) {
	now = now.UTC()
	result, err := db.conn.Exec(`
		INSERT INTO campaigns ("trigger", status, started_at, created_at)
		VALUES (?, ?, ?, ?)
	`, trigger, CampaignStatusRunning, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetCampaign(id)
}

// GetCampaign retrieves a campaign by ID.
func (db *DB) GetCampaign(id int64) (*Campaign, error) {
	c, err := scanCampaign(db.conn.QueryRow(
		"SELECT "+campaignColumns+" FROM campaigns WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCampaigns returns all campaigns, newest first, each with the live
// per-status count of its member runs.
func (db *DB) ListCampaigns() ([]CampaignWithProgress, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c."trigger", c.status, c.started_at, c.finished_at, c.created_at,
			COUNT(r.id) AS total,
			COALESCE(SUM(CASE WHEN r.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
			COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed,
			COALESCE(SUM(CASE WHEN r.status = 'running' THEN 1 ELSE 0 END), 0) AS running
		FROM campaigns c
		LEFT JOIN eval_runs r ON r.campaign_id = c.id
		GROUP BY c.id
		ORDER BY c.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CampaignWithProgress
	for rows.Next() {
		var c CampaignWithProgress
		var startedAt, finishedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Trigger, &c.Status, &startedAt, &finishedAt, &createdAt,
			&c.Progress.Total, &c.Progress.Done, &c.Progress.Failed, &c.Progress.Running); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			c.StartedAt = &t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			c.FinishedAt = &t
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SettleCampaign recomputes the campaign's aggregate status from its member
// runs and persists terminal transitions:
//   - any run still active -> stays running (left untouched);
//   - no runs at all, or any failed run -> failed;
//   - every run done -> done.
//
// Terminal transitions stamp finished_at; an already-terminal campaign is
// never rewritten, so the first settle wins.
func (db *DB) SettleCampaign(id int64, now time.Time) error {
	var total, failed, active int
	err := db.conn.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0)
		FROM eval_runs WHERE campaign_id = ?
	`, id).Scan(&total, &failed, &active)
	if err != nil {
		return err
	}
	if active > 0 {
		return nil
	}

	status := CampaignStatusDone
	if total == 0 || failed > 0 {
		status = CampaignStatusFailed
	}
	_, err = db.conn.Exec(`
		UPDATE campaigns SET status = ?, finished_at = ?
		WHERE id = ? AND status NOT IN (?, ?)
	`, status, now.UTC().Format(time.RFC3339), id, CampaignStatusDone, CampaignStatusFailed)
	return err
}

// PreviousDoneCampaign returns the most recent done campaign strictly before
// the given one, or nil when there is none. Only settled ("done") campaigns
// serve as report baselines. Note this differs from the score-drop alert's
// per-suite PreviousDoneCampaignRun: a manual single-suite campaign between
// two full sweeps becomes the baseline and is then marked incomparable
// (suite_missing) rather than skipped — conservative, never misleading.
func (db *DB) PreviousDoneCampaign(id int64) (*Campaign, error) {
	c, err := scanCampaign(db.conn.QueryRow(
		"SELECT "+campaignColumns+" FROM campaigns WHERE status = ? AND id < ? ORDER BY id DESC LIMIT 1",
		CampaignStatusDone, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// PreviousDoneCampaignRun returns the run covering the given suite inside the
// most recent done campaign strictly before the given campaign, or nil when
// no earlier done campaign covered the suite. It is the baseline the
// campaign-level score-drop alert compares against: only settled ("done")
// campaigns serve as baselines, matching the reporting unit semantics.
func (db *DB) PreviousDoneCampaignRun(campaignID, suiteID int64) (*EvalRun, error) {
	r, err := scanEvalRun(db.conn.QueryRow(`
		SELECT r.id, r.campaign_id, r.suite_id, r.suite_version, r."trigger", r.judge_model, r.status, r.started_at, r.finished_at
		FROM eval_runs r
		JOIN campaigns c ON c.id = r.campaign_id
		WHERE c.status = ? AND r.status = 'done' AND r.suite_id = ? AND r.campaign_id < ?
		ORDER BY r.campaign_id DESC, r.id DESC
		LIMIT 1
	`, CampaignStatusDone, suiteID, campaignID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListEvalRunsByCampaign returns a campaign's member runs, oldest first (the
// execution order of a sequential sweep).
func (db *DB) ListEvalRunsByCampaign(campaignID int64) ([]EvalRun, error) {
	rows, err := db.conn.Query(
		"SELECT "+evalRunColumns+" FROM eval_runs WHERE campaign_id = ? ORDER BY id", campaignID)
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
