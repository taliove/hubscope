package server

import (
	"sort"

	"github.com/taliove/hubscope/internal/store"
)

// normalizeScore01 converts a 0~1 raw mean into the ADR-0009 nadir-
// normalized caliber on the same 0~1 scale: (raw - nadir) / (1 - nadir),
// clamped to [0, 1]. A nadir of 0 degenerates to the legacy raw-mean
// caliber, and any nadir outside [0, 1) falls back to that same raw scale,
// so a misconfigured constant can never produce NaN or negative scores. The
// clamp is what makes "scored below the random-guess floor" read as 0
// rather than a negative number.
func normalizeScore01(raw, nadir float64) float64 {
	if nadir <= 0 || nadir >= 1 {
		return raw
	}
	v := (raw - nadir) / (1 - nadir)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// normalizeScore is normalizeScore01 on the report's 0-100 scale — the
// leaderboard's scoring caliber (ADR 0009).
func normalizeScore(raw, nadir float64) float64 {
	return normalizeScore01(raw, nadir) * 100
}

// nadirBySuiteKey resolves each covered suite's normalization constant from
// the runs' snapshots (ADR 0009): like the suite version, the nadir comes
// from the eval_runs snapshot rather than the suites table, so a historical
// report keeps the constant its runs were actually scored with even after
// the suite is recalibrated. Later runs win, mirroring suiteVersionSnapshot.
func nadirBySuiteKey(suites []reportSuiteDTO, runs []store.EvalRun) map[string]float64 {
	byID := make(map[int64]float64, len(runs))
	for _, run := range runs {
		byID[run.SuiteID] = run.Nadir
	}
	out := make(map[string]float64, len(suites))
	for _, suite := range suites {
		out[suite.Key] = byID[suite.ID]
	}
	return out
}

// buildReportRows groups per-(model, suite) aggregates into leaderboard rows,
// scaling raw means through the ADR-0009 nadir normalization and attaching
// the ADR-0005 weighted total.
func buildReportRows(scores []store.CampaignSuiteScore, weights map[string]float64, nadirs map[string]float64) []reportRowDTO {
	rows := []reportRowDTO{}
	indexByModel := map[int64]int{}
	for _, sc := range scores {
		idx, ok := indexByModel[sc.ModelDBID]
		if !ok {
			idx = len(rows)
			indexByModel[sc.ModelDBID] = idx
			rows = append(rows, reportRowDTO{
				ModelDBID:   sc.ModelDBID,
				ModelID:     sc.ModelID,
				Family:      sc.Family,
				SuiteScores: map[string]*float64{},
			})
		}
		var scaled *float64
		if sc.Score != nil {
			v := normalizeScore(*sc.Score, nadirs[sc.SuiteKey])
			scaled = &v
		}
		rows[idx].SuiteScores[sc.SuiteKey] = scaled
	}
	for i := range rows {
		rows[i].TotalScore = totalScore(rows[i].SuiteScores, weights)
	}
	return rows
}

// totalScore is the ADR-0005 total: the weighted mean of the model's scored
// suites on the 0-100 scale. Unscored suites drop out of both numerator and
// denominator; nil when the model scored nothing at all.
func totalScore(suiteScores map[string]*float64, weights map[string]float64) *float64 {
	var sum, wsum float64
	for key, score := range suiteScores {
		if score == nil {
			continue
		}
		w := weights[key]
		if w <= 0 {
			w = 1
		}
		sum += w * *score
		wsum += w
	}
	if wsum == 0 {
		return nil
	}
	total := sum / wsum
	return &total
}

// filterReportRowsByFamily returns a new slice with only the rows of the
// given model family; the input slice is left untouched.
func filterReportRowsByFamily(rows []reportRowDTO, family string) []reportRowDTO {
	filtered := make([]reportRowDTO, 0, len(rows))
	for _, row := range rows {
		if row.Family == family {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// sortReportRows ranks rows descending by the chosen column ("total" or a
// suite key). Models without a score in that column rank last; ties break by
// model id for a stable, deterministic board.
func sortReportRows(rows []reportRowDTO, sortKey string) {
	scoreOf := func(row reportRowDTO) *float64 {
		if sortKey == "total" {
			return row.TotalScore
		}
		return row.SuiteScores[sortKey]
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := scoreOf(rows[i]), scoreOf(rows[j])
		if a == nil || b == nil {
			return a != nil
		}
		if *a != *b {
			return *a > *b
		}
		return rows[i].ModelID < rows[j].ModelID
	})
}
