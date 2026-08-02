package store

import (
	"database/sql"
	"sort"
	"time"

	"github.com/taliove/hubscope/internal/status"
)

// Streaming mode identifiers stored in probe_rollups.streaming. The combined
// row is pre-computed at rollup time (not derived at query time) because two
// percentile values cannot be merged exactly once the raw probes are gone.
const (
	rollupNonStreaming = 0
	rollupStreaming    = 1
	rollupCombined     = 2
)

// SeriesMode selects which probe stream a series query aggregates.
type SeriesMode int

const (
	// SeriesAll merges streaming and non-streaming probes (default).
	SeriesAll SeriesMode = iota
	// SeriesStreaming restricts the series to streaming probes.
	SeriesStreaming
	// SeriesNonStreaming restricts the series to non-streaming probes.
	SeriesNonStreaming
)

// rollupMode maps a SeriesMode onto the stored streaming identifiers.
func (m SeriesMode) rollupMode() int {
	switch m {
	case SeriesStreaming:
		return rollupStreaming
	case SeriesNonStreaming:
		return rollupNonStreaming
	default:
		return rollupCombined
	}
}

// SeriesBucket is one hourly aggregation point of the series API. Percentile
// and TTFT fields are nil when the bucket has no usable values.
type SeriesBucket struct {
	BucketStart time.Time
	Total       int
	Failures    int
	P50Ms       *float64
	P95Ms       *float64
	AvgTTFTMs   *float64
}

// bucketAgg accumulates the raw probes of one hour bucket.
type bucketAgg struct {
	total     int
	failures  int
	latencies []int
	ttftSum   int
	ttftCount int
}

// add folds one probe into the accumulator. Latency enters only for
// successful probes (GH #160, appendix 17③): a failed probe's latency is
// time-to-failure and must never enter a presented percentile. The count
// fields still track every probe. Historical probe_rollups rows written under
// the old all-sample caliber are NOT backfilled (registered mixed period).
func (a *bucketAgg) add(ok bool, latencyMs int, ttftMs *int) {
	a.total++
	if !ok {
		a.failures++
	} else {
		a.latencies = append(a.latencies, latencyMs)
	}
	if ttftMs != nil {
		a.ttftSum += *ttftMs
		a.ttftCount++
	}
}

// build converts the accumulator into an immutable SeriesBucket. Percentiles
// are null when the bucket has no SUCCESSFUL probe — an all-failed bucket
// keeps its counts but presents no latency.
func (a *bucketAgg) build(start time.Time) SeriesBucket {
	b := SeriesBucket{BucketStart: start, Total: a.total, Failures: a.failures}
	if len(a.latencies) > 0 {
		p50 := status.Percentile(a.latencies, 50)
		p95 := status.Percentile(a.latencies, 95)
		b.P50Ms = &p50
		b.P95Ms = &p95
	}
	if a.ttftCount > 0 {
		avg := float64(a.ttftSum) / float64(a.ttftCount)
		b.AvgTTFTMs = &avg
	}
	return b
}

// bucketKey identifies one (endpoint, hour) aggregation group.
type bucketKey struct {
	endpointID int64
	start      time.Time
}

// bucketPair holds the per-streaming-mode accumulators of one group.
type bucketPair struct {
	streaming    bucketAgg
	nonStreaming bucketAgg
}

// RollupProbesBefore aggregates every probe created before cutoff (truncated
// to the hour) into hourly probe_rollups rows and advances each endpoint's
// rollup watermark to the cutoff. Only probes at or after the previous
// watermark are aggregated, so a bucket is never recomputed from raw rows
// that retention may have partially deleted. Upserts make the operation
// idempotent. It returns how many raw probe rows were aggregated.
func (db *DB) RollupProbesBefore(cutoff time.Time) (int, error) {
	cutoff = cutoff.UTC().Truncate(time.Hour)

	rows, err := db.conn.Query(`
		SELECT p.endpoint_id, p.streaming, p.ok, p.latency_ms, p.ttft_ms, p.created_at
		FROM probes p
		LEFT JOIN rollup_watermarks w ON w.endpoint_id = p.endpoint_id
		WHERE p.created_at < ? AND p.created_at >= COALESCE(w.rolled_up_to, '')
	`, cutoff.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	aggregated := 0
	groups := make(map[bucketKey]*bucketPair)
	for rows.Next() {
		var endpointID, streaming, ok, latencyMs int
		var ttftMs *int
		var createdAt string
		if err := rows.Scan(&endpointID, &streaming, &ok, &latencyMs, &ttftMs, &createdAt); err != nil {
			return 0, err
		}
		ts, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return 0, err
		}
		aggregated++
		key := bucketKey{endpointID: int64(endpointID), start: ts.Truncate(time.Hour)}
		pair, exists := groups[key]
		if !exists {
			pair = &bucketPair{}
			groups[key] = pair
		}
		if streaming == 1 {
			pair.streaming.add(ok == 1, latencyMs, ttftMs)
		} else {
			pair.nonStreaming.add(ok == 1, latencyMs, ttftMs)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	for key, pair := range groups {
		modes := []struct {
			mode int
			agg  bucketAgg
		}{
			{rollupStreaming, pair.streaming},
			{rollupNonStreaming, pair.nonStreaming},
			{rollupCombined, combineAggs(pair.streaming, pair.nonStreaming)},
		}
		for _, m := range modes {
			if m.agg.total == 0 {
				continue
			}
			b := m.agg.build(key.start)
			if _, err := tx.Exec(`
				INSERT OR REPLACE INTO probe_rollups
					(endpoint_id, streaming, bucket_start, total, failures, p50_ms, p95_ms, avg_ttft_ms)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, key.endpointID, m.mode, key.start.Format(time.RFC3339), b.Total, b.Failures, b.P50Ms, b.P95Ms, b.AvgTTFTMs); err != nil {
				return 0, err
			}
		}
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO rollup_watermarks (endpoint_id, rolled_up_to)
			VALUES (?, ?)
		`, key.endpointID, cutoff.Format(time.RFC3339)); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return aggregated, nil
}

// combineAggs merges two accumulators into the combined-mode aggregate.
func combineAggs(a, b bucketAgg) bucketAgg {
	return bucketAgg{
		total:     a.total + b.total,
		failures:  a.failures + b.failures,
		latencies: append(append([]int{}, a.latencies...), b.latencies...),
		ttftSum:   a.ttftSum + b.ttftSum,
		ttftCount: a.ttftCount + b.ttftCount,
	}
}

// DeleteProbesBefore removes raw probe rows created before cutoff and returns
// how many rows were deleted. Rolled-up history is unaffected.
func (db *DB) DeleteProbesBefore(cutoff time.Time) (int64, error) {
	res, err := db.conn.Exec(
		"DELETE FROM probes WHERE created_at < ?",
		cutoff.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetSeries returns hourly buckets for an endpoint since the given time,
// oldest first. Rolled-up buckets cover the past; raw probes at or after the
// endpoint's watermark fill the not-yet-aggregated tail, so the series stays
// complete both before and after retention deletes old raw rows.
func (db *DB) GetSeries(endpointID int64, since time.Time, mode SeriesMode) ([]SeriesBucket, error) {
	since = since.UTC()
	buckets := make(map[time.Time]SeriesBucket)

	rollupRows, err := db.conn.Query(`
		SELECT bucket_start, total, failures, p50_ms, p95_ms, avg_ttft_ms
		FROM probe_rollups
		WHERE endpoint_id = ? AND streaming = ? AND bucket_start >= ?
	`, endpointID, mode.rollupMode(), since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rollupRows.Close()

	for rollupRows.Next() {
		var start string
		var b SeriesBucket
		var p50, p95, avgTTFT sql.NullFloat64
		if err := rollupRows.Scan(&start, &b.Total, &b.Failures, &p50, &p95, &avgTTFT); err != nil {
			return nil, err
		}
		b.BucketStart, err = time.Parse(time.RFC3339, start)
		if err != nil {
			return nil, err
		}
		b.P50Ms = nullFloat(p50)
		b.P95Ms = nullFloat(p95)
		b.AvgTTFTMs = nullFloat(avgTTFT)
		buckets[b.BucketStart] = b
	}
	if err := rollupRows.Err(); err != nil {
		return nil, err
	}

	// The raw tail starts at the later of the query window and the watermark,
	// so probes already represented by rollups are never counted twice.
	tail := since
	var watermark string
	err = db.conn.QueryRow(
		"SELECT rolled_up_to FROM rollup_watermarks WHERE endpoint_id = ?",
		endpointID,
	).Scan(&watermark)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if wm, perr := time.Parse(time.RFC3339, watermark); perr == nil && wm.After(tail) {
		tail = wm
	}

	rawQuery := `
		SELECT ok, latency_ms, ttft_ms, created_at
		FROM probes
		WHERE endpoint_id = ? AND created_at >= ?
	`
	args := []interface{}{endpointID, tail.Format(time.RFC3339)}
	if mode == SeriesStreaming {
		rawQuery += " AND streaming = 1"
	} else if mode == SeriesNonStreaming {
		rawQuery += " AND streaming = 0"
	}

	rawRows, err := db.conn.Query(rawQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rawRows.Close()

	aggs := make(map[time.Time]*bucketAgg)
	for rawRows.Next() {
		var ok, latencyMs int
		var ttftMs *int
		var createdAt string
		if err := rawRows.Scan(&ok, &latencyMs, &ttftMs, &createdAt); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		start := ts.Truncate(time.Hour)
		agg, exists := aggs[start]
		if !exists {
			agg = &bucketAgg{}
			aggs[start] = agg
		}
		agg.add(ok == 1, latencyMs, ttftMs)
	}
	if err := rawRows.Err(); err != nil {
		return nil, err
	}

	for start, agg := range aggs {
		if _, covered := buckets[start]; !covered {
			buckets[start] = agg.build(start)
		}
	}

	out := make([]SeriesBucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BucketStart.Before(out[j].BucketStart) })
	return out, nil
}

// nullFloat converts a nullable SQL float into a pointer.
func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}
