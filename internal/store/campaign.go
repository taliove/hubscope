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
// given start time, and snapshots its model membership (ticket 53) in the
// same transaction: the campaign and the population it set out to evaluate
// always agree, so the progress grid can show every member — including
// models with no results yet — from the first run on. Duplicate model IDs
// collapse onto the membership primary key. Returns the stored copy.
func (db *DB) CreateCampaign(trigger string, modelIDs []int64, now time.Time) (*Campaign, error) {
	now = now.UTC()
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
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
	for _, modelID := range modelIDs {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO campaign_models (campaign_id, model_id) VALUES (?, ?)",
			id, modelID,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
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

// ListCampaignsAll returns every campaign, newest first, each with the live
// per-status count of its member runs. It is the super_admin / store-internal
// counterpart of ListCampaignsByHub; HTTP handlers must pick the form based
// on the session's hub scope.
func (db *DB) ListCampaignsAll() ([]CampaignWithProgress, error) {
	return db.listCampaigns(0)
}

// ListCampaignsByHub returns the campaigns reachable from a single hub —
// campaigns whose member set (campaign_models) includes at least one model
// belonging to hubID — newest first, each with the live per-status count of
// its member runs. It is the hub-scoped form HTTP handlers must use for
// non-super_admin sessions.
func (db *DB) ListCampaignsByHub(hubID int64) ([]CampaignWithProgress, error) {
	return db.listCampaigns(hubID)
}

// listCampaigns is the shared implementation. hubID is the empty string for
// the unscoped (all) variant, or a parameter placeholder for the hub-scoped
// variant. The progress aggregation is identical to the old ListCampaigns.
func (db *DB) listCampaigns(hubID int64) ([]CampaignWithProgress, error) {
	hubFilter := ""
	var args []interface{}
	if hubID != 0 {
		hubFilter = `WHERE EXISTS (
			SELECT 1 FROM campaign_models cm
			JOIN models m ON m.id = cm.model_id
			WHERE cm.campaign_id = c.id AND m.hub_id = ?
		)`
		args = append(args, hubID)
	}
	rows, err := db.conn.Query(`
		SELECT c.id, c."trigger", c.status, c.started_at, c.finished_at, c.created_at,
			COUNT(r.id) AS total,
			COALESCE(SUM(CASE WHEN r.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
			COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed,
			COALESCE(SUM(CASE WHEN r.status = 'running' THEN 1 ELSE 0 END), 0) AS running
		FROM campaigns c
		LEFT JOIN eval_runs r ON r.campaign_id = c.id
		`+hubFilter+`
		GROUP BY c.id
		ORDER BY c.id DESC
	`, args...)
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

// CampaignVisibleToHub reports whether the campaign is reachable from the
// given hub — its snapshotted membership includes at least one model
// belonging to hubID. It is the single-campaign form of the
// ListCampaignsByHub reachability rule, for access checks on
// campaign-scoped reads (issue #17's live feed).
func (db *DB) CampaignVisibleToHub(campaignID, hubID int64) (bool, error) {
	var n int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM campaign_models cm
		JOIN models m ON m.id = cm.model_id
		WHERE cm.campaign_id = ? AND m.hub_id = ?
	`, campaignID, hubID).Scan(&n)
	return n > 0, err
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

// LatestSettledCampaign returns the newest settled (done or failed)
// campaign, or nil when there is none. Failed campaigns count as settled:
// their board is a legitimate, complete evaluation outcome (spec 0010 — the
// public eval board shows the newest settled batch, never a running one).
func (db *DB) LatestSettledCampaign() (*Campaign, error) {
	c, err := scanCampaign(db.conn.QueryRow(
		"SELECT "+campaignColumns+" FROM campaigns WHERE status IN (?, ?) ORDER BY id DESC LIMIT 1",
		CampaignStatusDone, CampaignStatusFailed))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// HasUnfinishedCampaign reports whether any campaign is still pending or
// running (spec 0010 — the public board's neutral "a new batch is in
// flight" hint; the flag is operational metadata, not an evaluation
// conclusion).
func (db *DB) HasUnfinishedCampaign() (bool, error) {
	var n int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM campaigns WHERE status IN (?, ?)",
		CampaignStatusPending, CampaignStatusRunning).Scan(&n)
	return n > 0, err
}

// PreviousDoneCampaignRun returns the run covering the given suite inside the
// most recent done campaign strictly before the given campaign, or nil when
// no earlier done campaign covered the suite. It is the baseline the
// campaign-level score-drop alert compares against: only settled ("done")
// campaigns serve as baselines, matching the reporting unit semantics.
func (db *DB) PreviousDoneCampaignRun(campaignID, suiteID int64) (*EvalRun, error) {
	r, err := scanEvalRun(db.conn.QueryRow(`
		SELECT r.id, r.campaign_id, r.suite_id, r.suite_version, r.nadir, r."trigger", r.judge_model, r.status, r.started_at, r.finished_at
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

// GetLatestCampaignForModel returns the most recent campaign the model
// participated in (via campaign_models membership), or nil when the model has
// never been evaluated. The returned campaign may be unfinished; callers must
// check its status before assuming scores are available.
func (db *DB) GetLatestCampaignForModel(modelID int64) (*Campaign, error) {
	c, err := scanCampaign(db.conn.QueryRow(`
		SELECT `+campaignColumns+`
		FROM campaigns c
		JOIN campaign_models cm ON cm.campaign_id = c.id
		WHERE cm.model_id = ?
		ORDER BY c.id DESC
		LIMIT 1
	`, modelID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetLatestCampaignsForModels returns a map of model_id → latest campaign for
// all provided model IDs in a single query. Models that have never been
// evaluated are absent from the result map. This is the batch version of
// GetLatestCampaignForModel, used to avoid N+1 queries when loading eval
// scores for the overview API.
func (db *DB) GetLatestCampaignsForModels(modelIDs []int64) (map[int64]*Campaign, error) {
	if len(modelIDs) == 0 {
		return map[int64]*Campaign{}, nil
	}

	// Build the IN clause with placeholders
	placeholders := make([]string, len(modelIDs))
	args := make([]interface{}, len(modelIDs))
	for i, id := range modelIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// Find the latest campaign per model using a subquery to get max campaign_id
	query := `
		SELECT cm.model_id, ` + campaignColumns + `
		FROM campaign_models cm
		JOIN campaigns c ON c.id = cm.campaign_id
		WHERE cm.model_id IN (` + string(placeholders[0])
	for i := 1; i < len(placeholders); i++ {
		query += ", " + placeholders[i]
	}
	query += `)
		AND cm.campaign_id = (
			SELECT MAX(cm2.campaign_id)
			FROM campaign_models cm2
			WHERE cm2.model_id = cm.model_id
		)
	`

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[int64]*Campaign{}
	for rows.Next() {
		var modelID int64
		var c Campaign
		var startedAt, finishedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&modelID, &c.ID, &c.Trigger, &c.Status, &startedAt, &finishedAt, &createdAt); err != nil {
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
		result[modelID] = &c
	}
	return result, rows.Err()
}
