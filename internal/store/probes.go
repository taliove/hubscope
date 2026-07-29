package store

import "time"

// maxProbeLimit caps how many probe records a single query may return.
const maxProbeLimit = 200

// defaultProbeLimit applies when the caller passes a non-positive limit.
const defaultProbeLimit = 50

// Probe represents a probe execution result
type Probe struct {
	ID           int64
	EndpointID   int64
	Streaming    bool
	OK           bool
	HTTPStatus   int
	ErrorSummary *string
	LatencyMs    int
	TTFTMs       *int
	InputTokens  *int
	OutputTokens *int
	CreatedAt    time.Time
}

// ProbeWatermark returns the largest probe ID (0 when the table is empty).
// It is the overview snapshot's cheap freshness sentinel (spec 0015 decision
// 3): every probe insert moves it strictly upward, so one indexed MAX query
// per request replaces the per-endpoint query fan-out on cache hits — and it
// catches probe writes from any path, including ones that bypass the HTTP
// handlers (seeded history in tests, future writers).
func (db *DB) ProbeWatermark() (int64, error) {
	var id int64
	err := db.conn.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM probes`).Scan(&id)
	return id, err
}

// CreateProbe inserts a probe record and returns the stored copy with its
// assigned ID and creation time. The input is not mutated. When p.CreatedAt
// is zero the current time is used; a non-zero CreatedAt is stored as-is so
// tests can seed history at controlled timestamps.
func (db *DB) CreateProbe(p Probe) (Probe, error) {
	now := time.Now().UTC()
	createdAt := now
	if !p.CreatedAt.IsZero() {
		createdAt = p.CreatedAt.UTC()
	}

	streaming := 0
	if p.Streaming {
		streaming = 1
	}
	ok := 0
	if p.OK {
		ok = 1
	}

	result, err := db.conn.Exec(`
		INSERT INTO probes (endpoint_id, streaming, ok, http_status, error_summary, latency_ms, ttft_ms, input_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.EndpointID, streaming, ok, p.HTTPStatus, p.ErrorSummary, p.LatencyMs, p.TTFTMs, p.InputTokens, p.OutputTokens, createdAt.Format(time.RFC3339))
	if err != nil {
		return Probe{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Probe{}, err
	}

	p.ID = id
	p.CreatedAt = createdAt
	return p, nil
}

// ListProbes returns probe records for an endpoint, newest first. okFilter
// restricts the result to successful (true) or failed (false) probes when
// non-nil; nil returns both kinds.
func (db *DB) ListProbes(endpointID int64, limit int, okFilter *bool) ([]Probe, error) {
	if limit <= 0 {
		limit = defaultProbeLimit
	}
	if limit > maxProbeLimit {
		limit = maxProbeLimit
	}

	query := `
		SELECT id, endpoint_id, streaming, ok, http_status, error_summary, latency_ms, ttft_ms, input_tokens, output_tokens, created_at
		FROM probes
		WHERE endpoint_id = ?
	`
	args := []interface{}{endpointID}
	if okFilter != nil {
		ok := 0
		if *okFilter {
			ok = 1
		}
		query += " AND ok = ?"
		args = append(args, ok)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var probes []Probe
	for rows.Next() {
		var p Probe
		var streaming, ok int
		var createdAt string
		if err := rows.Scan(&p.ID, &p.EndpointID, &streaming, &ok, &p.HTTPStatus, &p.ErrorSummary, &p.LatencyMs, &p.TTFTMs, &p.InputTokens, &p.OutputTokens, &createdAt); err != nil {
			return nil, err
		}
		p.Streaming = streaming == 1
		p.OK = ok == 1
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		probes = append(probes, p)
	}

	return probes, rows.Err()
}
