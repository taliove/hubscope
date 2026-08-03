package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// EvalAnswer is one persisted answer attempt of the decoupled eval pipeline
// (spec 0020, ADR 0016): written the moment an exam call settles, before the
// answer enters the judge queue, so a crash mid-judging never loses a paid
// completion. Status is EvalAnswerAnswered or EvalAnswerFailed; only
// answered rows enter the judge queue.
type EvalAnswer struct {
	ID           int64
	EvalRunID    int64
	ModelDBID    int64
	ModelID      string
	CaseID       int64
	SampleNo     int
	Attempt      int
	Status       string
	AnswerText   *string
	LatencyMs    int
	InputTokens  *int
	OutputTokens *int
	CreatedAt    time.Time
}

// EvalAnswer statuses.
const (
	EvalAnswerAnswered = "answered"
	EvalAnswerFailed   = "failed"
)

// EvalJudgeScore is one jury member's verdict on one answer (spec 0020):
// one row per (answer, jury slot). Score is nil when the judge call failed
// (W7 — a failed judge never counts as zero).
type EvalJudgeScore struct {
	ID         int64
	AnswerID   int64
	Slot       int
	JudgeModel string
	Score      *float64
	LatencyMs  int
	CreatedAt  time.Time
}

// CreateEvalAnswer persists one answer attempt and returns its row ID.
// Re-answering a cell (retry-failed) lands as a new attempt, not a
// duplicate-key failure.
func (db *DB) CreateEvalAnswer(a EvalAnswer) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.conn.Exec(`
		INSERT INTO eval_answers (eval_run_id, model_db_id, model_id, case_id, sample_no, attempt, status, answer_text, latency_ms, input_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(attempt), 0) + 1 FROM eval_answers WHERE eval_run_id = ? AND model_db_id = ? AND case_id = ? AND sample_no = ?), ?, ?, ?, ?, ?, ?)
	`, a.EvalRunID, a.ModelDBID, a.ModelID, a.CaseID, a.SampleNo,
		a.EvalRunID, a.ModelDBID, a.CaseID, a.SampleNo,
		a.Status, a.AnswerText, a.LatencyMs, a.InputTokens, a.OutputTokens, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateEvalJudgeScore persists one jury verdict and returns its row ID.
func (db *DB) CreateEvalJudgeScore(s EvalJudgeScore) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.conn.Exec(`
		INSERT INTO eval_judge_scores (answer_id, slot, judge_model, score, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.AnswerID, s.Slot, s.JudgeModel, s.Score, s.LatencyMs, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const evalAnswerColumns = `id, eval_run_id, model_db_id, model_id, case_id, sample_no, attempt, status, answer_text, latency_ms, input_tokens, output_tokens, created_at`

// scanEvalAnswers scans eval_answers rows.
func scanEvalAnswers(rows *sql.Rows) ([]EvalAnswer, error) {
	var out []EvalAnswer
	for rows.Next() {
		var a EvalAnswer
		var createdAt string
		if err := rows.Scan(&a.ID, &a.EvalRunID, &a.ModelDBID, &a.ModelID, &a.CaseID,
			&a.SampleNo, &a.Attempt, &a.Status, &a.AnswerText, &a.LatencyMs, &a.InputTokens, &a.OutputTokens, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, a)
	}
	return out, rows.Err()
}

// JudgeVote is one judge score joined with its answer's cell coordinates —
// the run detail's per-case jury breakdown (GH #178).
type JudgeVote struct {
	ModelDBID  int64
	CaseID     int64
	SampleNo   int
	Attempt    int
	Slot       int
	JudgeModel string
	Score      *float64
}

// ListJudgeVotesByRun returns every judge vote of one run with its cell
// coordinates, ordered for per-case assembly.
func (db *DB) ListJudgeVotesByRun(runID int64) ([]JudgeVote, error) {
	rows, err := db.conn.Query(`
		SELECT a.model_db_id, a.case_id, a.sample_no, a.attempt, js.slot, js.judge_model, js.score
		FROM eval_judge_scores js
		JOIN eval_answers a ON a.id = js.answer_id
		WHERE a.eval_run_id = ?
		ORDER BY a.model_db_id, a.case_id, a.sample_no, a.attempt, js.slot
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JudgeVote
	for rows.Next() {
		var v JudgeVote
		if err := rows.Scan(&v.ModelDBID, &v.CaseID, &v.SampleNo, &v.Attempt, &v.Slot, &v.JudgeModel, &v.Score); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListRunsWithAnswers returns the distinct runs that have eval_answers
// rows — the crash-recovery candidate set (GH #176).
func (db *DB) ListRunsWithAnswers() ([]int64, error) {
	rows, err := db.conn.Query(`SELECT DISTINCT eval_run_id FROM eval_answers ORDER BY eval_run_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListEvalAnswersByRun returns every answer row of one run, cell order.
func (db *DB) ListEvalAnswersByRun(runID int64) ([]EvalAnswer, error) {
	rows, err := db.conn.Query(`
		SELECT `+evalAnswerColumns+` FROM eval_answers
		WHERE eval_run_id = ? ORDER BY model_db_id, case_id, sample_no
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvalAnswers(rows)
}

// ListUnderJudgedAnswers returns the run's answered rows that have fewer
// than jurySize judge rows — the crash-recovery candidate set (ADR 0016):
// after a restart the judge queue is rebuilt from these, and slots already
// judged are never re-judged.
func (db *DB) ListUnderJudgedAnswers(runID int64, jurySize int) ([]EvalAnswer, error) {
	rows, err := db.conn.Query(`
		SELECT `+evalAnswerColumns+` FROM eval_answers a
		WHERE eval_run_id = ? AND status = ?
		  AND (SELECT COUNT(*) FROM eval_judge_scores js WHERE js.answer_id = a.id) < ?
		ORDER BY a.id
	`, runID, EvalAnswerAnswered, jurySize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvalAnswers(rows)
}

// ListJudgeScoresByAnswer returns every jury verdict of one answer, slot order.
func (db *DB) ListJudgeScoresByAnswer(answerID int64) ([]EvalJudgeScore, error) {
	rows, err := db.conn.Query(`
		SELECT id, answer_id, slot, judge_model, score, latency_ms, created_at
		FROM eval_judge_scores WHERE answer_id = ? ORDER BY slot
	`, answerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvalJudgeScore
	for rows.Next() {
		var s EvalJudgeScore
		var createdAt string
		if err := rows.Scan(&s.ID, &s.AnswerID, &s.Slot, &s.JudgeModel, &s.Score, &s.LatencyMs, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetEvalRunJuryModels snapshots the run's jury (JSON: policy + slot model
// IDs; ADR 0016). The snapshot makes every verdict attributable to the jury
// that produced it even after settings change.
func (db *DB) SetEvalRunJuryModels(runID int64, jury string) error {
	_, err := db.conn.Exec(`UPDATE eval_runs SET jury_models = ? WHERE id = ?`, jury, runID)
	return err
}

// GetEvalRunJuryModels returns the stored jury snapshot, "" when unset
// (pre-jury runs).
func (db *DB) GetEvalRunJuryModels(runID int64) (string, error) {
	var jury sql.NullString
	if err := db.conn.QueryRow(`SELECT jury_models FROM eval_runs WHERE id = ?`, runID).Scan(&jury); err != nil {
		return "", err
	}
	return jury.String, nil
}

// SetEvalRunEstimatedCost stores the run's estimated cost as a JSON split
// {"exam":x,"judge":y} (spec 0020, GH #178). NULL means some component's
// price or token usage is not registered — rendered as "price not
// registered", never as a partial sum.
func (db *DB) SetEvalRunCost(runID int64, exam, judge *float64) error {
	if exam == nil || judge == nil {
		_, err := db.conn.Exec(`UPDATE eval_runs SET estimated_cost = NULL WHERE id = ?`, runID)
		return err
	}
	raw, err := json.Marshal(map[string]float64{"exam": *exam, "judge": *judge})
	if err != nil {
		return err
	}
	_, err = db.conn.Exec(`UPDATE eval_runs SET estimated_cost = ? WHERE id = ?`, string(raw), runID)
	return err
}
