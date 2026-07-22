package store

import (
	"database/sql"
	"sort"
	"time"
)

// TrendPoint is one (campaign, suite) score point of a model's cross-campaign
// trend. Score is nil when the campaign's done runs judged nothing for the
// model (every answer or judge call failed) — the point stays visible so an
// unjudged batch never reads as a real zero. VerdictProfile is the newest
// scoring caliber found among the point's results (ADR 0008), so the reader
// can mark a break where the caliber changes between adjacent points. Nadir
// is the run's normalization-constant snapshot (ADR 0009), so each point is
// scaled with the constant it was actually scored under.
type TrendPoint struct {
	CampaignID     int64
	SuiteID        int64
	SuiteKey       string
	SuiteName      string
	SuiteVersion   int
	VerdictProfile string
	Nadir          float64
	Score          *float64
}

// ListModelTrend returns the model's per-(campaign, suite) aggregate scores
// across every settled campaign (done or failed) up to and including the
// given campaign, ordered by campaign then suite. Deleted models keep their
// history: eval_results denormalize model identity, so no models join is
// needed (ticket 26 semantics, ADR 0007). NULL scores never enter the
// average — SQLite AVG skips NULLs, the same convention as the leaderboard.
func (db *DB) ListModelTrend(modelDBID, campaignID int64) ([]TrendPoint, error) {
	rows, err := db.conn.Query(`
		SELECT r.campaign_id, r.suite_id, s.key, s.name, r.suite_version,
			MAX(res.verdict_profile), r.nadir, AVG(res.score)
		FROM eval_runs r
		JOIN eval_results res ON res.eval_run_id = r.id
		JOIN campaigns c ON c.id = r.campaign_id
		JOIN suites s ON s.id = r.suite_id
		WHERE res.model_db_id = ? AND r.status = 'done'
			AND c.status IN (?, ?) AND c.id <= ?
		GROUP BY r.campaign_id, r.suite_id
		ORDER BY r.campaign_id, r.suite_id
	`, modelDBID, CampaignStatusDone, CampaignStatusFailed, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []TrendPoint{}
	for rows.Next() {
		var p TrendPoint
		if err := rows.Scan(&p.CampaignID, &p.SuiteID, &p.SuiteKey, &p.SuiteName,
			&p.SuiteVersion, &p.VerdictProfile, &p.Nadir, &p.Score); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// ModelIdentity is the display identity of a model for history views:
// resolved from the models table while it lives, from the denormalized
// eval_results text after deletion. Deleted covers both ticket-26 senses:
// the row is gone (manual delete) or retired (discovered model dropped by
// its hub).
type ModelIdentity struct {
	ModelID string
	Family  string
	Deleted bool
}

// GetModelIdentity resolves a model's trend-view identity. found=false means
// the model neither exists nor has any eval history.
func (db *DB) GetModelIdentity(modelDBID int64) (identity ModelIdentity, found bool, err error) {
	model, err := db.GetModel(modelDBID)
	if err == nil {
		return ModelIdentity{ModelID: model.ModelID, Family: model.Family, Deleted: model.Status != "active"}, true, nil
	}
	if err != sql.ErrNoRows {
		return ModelIdentity{}, false, err
	}
	var modelID string
	err = db.conn.QueryRow(
		"SELECT model_id FROM eval_results WHERE model_db_id = ? ORDER BY id DESC LIMIT 1", modelDBID,
	).Scan(&modelID)
	if err == sql.ErrNoRows {
		return ModelIdentity{}, false, nil
	}
	if err != nil {
		return ModelIdentity{}, false, err
	}
	return ModelIdentity{ModelID: modelID, Deleted: true}, true, nil
}

// EarliestSettledCampaignStart returns the start time of the earliest settled
// campaign (done or failed) up to and including the given one. ok=false when
// no settled campaign exists in that range (e.g. the campaign is still
// running), in which case the caller picks its own window start.
func (db *DB) EarliestSettledCampaignStart(campaignID int64) (start time.Time, ok bool, err error) {
	var started sql.NullString
	err = db.conn.QueryRow(`
		SELECT MIN(started_at) FROM campaigns
		WHERE status IN (?, ?) AND id <= ? AND started_at IS NOT NULL
	`, CampaignStatusDone, CampaignStatusFailed, campaignID).Scan(&started)
	if err != nil {
		return time.Time{}, false, err
	}
	if !started.Valid {
		return time.Time{}, false, nil
	}
	t, perr := time.Parse(time.RFC3339, started.String)
	if perr != nil {
		return time.Time{}, false, perr
	}
	return t, true, nil
}

// ModelProbeBucket is one hourly probe aggregate across all of a model's
// enabled endpoints. Percentiles are probe-count-weighted means of the
// per-endpoint bucket percentiles: exact merging is impossible once raw
// probes are rolled up, and for a trend view the weighted mean is the
// faithful-enough summary.
type ModelProbeBucket struct {
	BucketStart time.Time
	Total       int
	Failures    int
	P50Ms       *float64
	P95Ms       *float64
}

// probeMerge accumulates one hourly bucket across endpoints.
type probeMerge struct {
	total    int
	failures int
	p50Sum   float64
	p50N     int
	p95Sum   float64
	p95N     int
}

// add folds one endpoint bucket into the accumulator, weighting percentiles
// by the bucket's probe count.
func (m *probeMerge) add(b SeriesBucket) {
	m.total += b.Total
	m.failures += b.Failures
	if b.P50Ms != nil {
		m.p50Sum += *b.P50Ms * float64(b.Total)
		m.p50N += b.Total
	}
	if b.P95Ms != nil {
		m.p95Sum += *b.P95Ms * float64(b.Total)
		m.p95N += b.Total
	}
}

// build converts the accumulator into an immutable ModelProbeBucket.
func (m *probeMerge) build(start time.Time) ModelProbeBucket {
	b := ModelProbeBucket{BucketStart: start, Total: m.total, Failures: m.failures}
	if m.p50N > 0 {
		p50 := m.p50Sum / float64(m.p50N)
		b.P50Ms = &p50
	}
	if m.p95N > 0 {
		p95 := m.p95Sum / float64(m.p95N)
		b.P95Ms = &p95
	}
	return b
}

// GetModelProbeSeries returns hourly probe buckets aggregated over the
// model's currently enabled endpoints since the given time, oldest first. A
// deleted model has no endpoints and yields an empty series; a retired model
// keeps its endpoints, so its probe history stays visible next to its score
// trend. Per-endpoint history comes from GetSeries, so rolled-up hours and
// the raw tail are merged exactly once each.
func (db *DB) GetModelProbeSeries(modelDBID int64, since, until time.Time) ([]ModelProbeBucket, error) {
	endpoints, err := db.ListEndpointsByModelID(modelDBID)
	if err != nil {
		return nil, err
	}

	merged := make(map[time.Time]*probeMerge)
	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		buckets, err := db.GetSeries(ep.ID, since, SeriesAll)
		if err != nil {
			return nil, err
		}
		for _, b := range buckets {
			// The probe side shares the score trend's timeline: buckets past
			// the campaign's end are excluded (spec 0002: same time axis).
			if b.BucketStart.After(until) {
				continue
			}
			acc, exists := merged[b.BucketStart]
			if !exists {
				acc = &probeMerge{}
				merged[b.BucketStart] = acc
			}
			acc.add(b)
		}
	}

	out := make([]ModelProbeBucket, 0, len(merged))
	for start, acc := range merged {
		out = append(out, acc.build(start))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BucketStart.Before(out[j].BucketStart) })
	return out, nil
}
