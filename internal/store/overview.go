package store

import (
	"database/sql"
	"time"
)

// ProbeSample is the minimal probe data used for window statistics.
type ProbeSample struct {
	OK        bool
	LatencyMs int
	CreatedAt time.Time
}

// CountConsecutiveFailures returns how many probes have failed in a row
// since the most recent success, ordered by creation time.
func (db *DB) CountConsecutiveFailures(endpointID int64) (int, error) {
	var count int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM probes
		WHERE endpoint_id = ? AND ok = 0
		AND created_at > COALESCE(
			(SELECT MAX(created_at) FROM probes WHERE endpoint_id = ? AND ok = 1),
			''
		)
	`, endpointID, endpointID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// LatestProbe returns the newest probe for an endpoint, or nil when the
// endpoint has never been probed.
func (db *DB) LatestProbe(endpointID int64) (*Probe, error) {
	row := db.conn.QueryRow(`
		SELECT id, endpoint_id, streaming, ok, http_status, error_summary, latency_ms, ttft_ms, input_tokens, output_tokens, created_at
		FROM probes
		WHERE endpoint_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, endpointID)

	var p Probe
	var streaming, ok int
	var createdAt string
	err := row.Scan(&p.ID, &p.EndpointID, &streaming, &ok, &p.HTTPStatus, &p.ErrorSummary, &p.LatencyMs, &p.TTFTMs, &p.InputTokens, &p.OutputTokens, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Streaming = streaming == 1
	p.OK = ok == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &p, nil
}

// CountProbes returns the total number of probes recorded for an endpoint.
func (db *DB) CountProbes(endpointID int64) (int, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM probes WHERE endpoint_id = ?",
		endpointID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountProbeSamplesBetween returns how many probes were created at or after
// from and before to, and how many of them succeeded. The upper bound is
// exclusive so adjacent windows never double-count a boundary sample.
func (db *DB) CountProbeSamplesBetween(endpointID int64, from, to time.Time) (total int, ok int, err error) {
	err = db.conn.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(ok), 0) FROM probes
		WHERE endpoint_id = ? AND created_at >= ? AND created_at < ?
	`, endpointID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)).Scan(&total, &ok)
	if err != nil {
		return 0, 0, err
	}
	return total, ok, nil
}

// ListProbeSamplesSince returns ok/latency/timestamp triples for all probes
// created at or after the given time, oldest first.
func (db *DB) ListProbeSamplesSince(endpointID int64, since time.Time) ([]ProbeSample, error) {
	rows, err := db.conn.Query(`
		SELECT ok, latency_ms, created_at FROM probes
		WHERE endpoint_id = ? AND created_at >= ?
		ORDER BY created_at
	`, endpointID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []ProbeSample
	for rows.Next() {
		var s ProbeSample
		var ok int
		var createdAt string
		if err := rows.Scan(&ok, &s.LatencyMs, &createdAt); err != nil {
			return nil, err
		}
		s.OK = ok == 1
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		samples = append(samples, s)
	}
	return samples, rows.Err()
}
