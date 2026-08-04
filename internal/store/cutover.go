package store

import (
	"database/sql"
	"strings"
)

// benchmarkCutoverKey is the settings key recording that the ticket-99
// benchmark cutover has run on this database.
const benchmarkCutoverKey = "benchmark_cutover"

// benchmarkTrimKey is the settings key recording that the 100→20 case trim
// has run on this database.
const benchmarkTrimKey = "benchmark_trim_20"

// trimBenchmarkSuitesToTwenty converges databases that already cast the
// 100-row benchmark bank onto the 20-case subsets. The trimmed files keep
// every 5th row of the four stratified banks and a coverage-first
// selection of ifeval (every ported instruction type stays represented);
// the migration therefore cannot use a position grid — it keeps the
// cases whose prompt is in the embedded subset, inside the suite's first
// 100 enabled cases (seed order). Custom cases sit past the seed window
// and stay untouched (seed discipline: admin edits are never reverted).
// The suite version bumps once per trimmed suite (the trend breakpoint
// caliber — a changed question bank never reads as a model change, ADR
// 0007). One-time, settings-tracked, idempotent.
func (db *DB) trimBenchmarkSuitesToTwenty() error {
	done, err := db.GetSetting(benchmarkTrimKey, "")
	if err != nil {
		return err
	}
	if done != "" {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, suite := range benchmarkSuites {
		var suiteID int64
		err := tx.QueryRow("SELECT id FROM suites WHERE key = ?", suite.key).Scan(&suiteID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(
			"SELECT id, prompt FROM cases WHERE suite_id = ? AND enabled = 1 ORDER BY id LIMIT 100", suiteID)
		if err != nil {
			return err
		}
		type caseRow struct {
			id     int64
			prompt string
		}
		var bank []caseRow
		for rows.Next() {
			var cr caseRow
			if err := rows.Scan(&cr.id, &cr.prompt); err != nil {
				rows.Close()
				return err
			}
			bank = append(bank, cr)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(bank) < 100 {
			continue // fresh or already-trimmed bank: nothing to retire
		}
		keep := make(map[string]bool, len(suite.cases))
		for _, c := range suite.cases {
			keep[c.prompt] = true
		}
		var retire []int64
		for _, cr := range bank {
			if !keep[cr.prompt] {
				retire = append(retire, cr.id)
			}
		}
		if len(retire) == 0 {
			continue
		}
		placeholders := make([]string, len(retire))
		args := make([]interface{}, len(retire))
		for i, id := range retire {
			placeholders[i] = "?"
			args[i] = id
		}
		if _, err := tx.Exec(
			"UPDATE cases SET enabled = 0 WHERE id IN ("+strings.Join(placeholders, ",")+")", args...,
		); err != nil {
			return err
		}
		if err := bumpSuiteVersion(tx, suiteID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, '1')", benchmarkTrimKey,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// decision C (ticket 99): the five authoritative-benchmark suites join the
// evaluation rotation. The generation-tracked seed mechanism has only a
// retirement path (seeds.go clears enabled when retireAtGen passes); it has
// no enable path, so the switch is this one-time, settings-tracked
// migration:
// enableBenchmarkSuitesAtCutover performs the deliberate switch of spec 0014
// decision C (ticket 99): the five authoritative-benchmark suites join the
// evaluation rotation. The generation-tracked seed mechanism has only a
// retirement path (seeds.go clears enabled when retireAtGen passes); it has
// no enable path, so the switch is this one-time, settings-tracked
// migration:
//
//   - Fresh databases seed the benchmark suites enabled (retireAtGen 0) and
//     merely record the key here.
//   - Databases that seeded the suites disabled under tickets 94-98
//     (retireAtGen 1, seed_gen 1 received) are flipped to enabled.
//
// It runs at Open between seedSuites and purgeDisabledSuites: any later and
// the purge — whose seeds-disabled-by-design exemption ended with the
// cutover — would delete the still-disabled benchmark suites before they
// could be flipped.
//
// One-shot on purpose (ADR 0012 Addendum): an admin disabling a benchmark
// suite after the cutover hands it to the disabled-suite purge at the next
// Open, tombstoned against re-seeding, and this migration must never
// resurrect it — "disabled means gone".
func (db *DB) enableBenchmarkSuitesAtCutover() error {
	done, err := db.GetSetting(benchmarkCutoverKey, "")
	if err != nil {
		return err
	}
	if done != "" {
		return nil
	}

	// Keys are compile-time constants from our own bank, never user input.
	keys := make([]string, 0, len(benchmarkSuites))
	for _, s := range benchmarkSuites {
		keys = append(keys, "'"+s.key+"'")
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		"UPDATE suites SET enabled = 1 WHERE key IN (" + strings.Join(keys, ", ") + ")",
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, '1')", benchmarkCutoverKey,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// retireV3SuitesAtOpen unconditionally retires the v3 capability suites on
// every Open, immediately before the disabled-suite purge. It is the
// mid-state fallback of the ticket-99 cutover (GH #15): a database that
// reached the cutover through a half-applied path — v3 purge tombstones
// already written, so seedSuites skips the v3 bank and the
// generation-tracked retirement (retireAtGen 4) never fires, while the v3
// rows themselves survived still enabled — would otherwise keep them
// forever, because the purge deletes only enabled=0 suites. Flipping them
// off here hands them to the purge in the same Open.
//
// Deliberately NOT gated on the one-shot benchmark_cutover settings key:
// that key records the enable migration, not the v3 retirement, and the
// mid-state has it already written. Unconditional is safe: the UPDATE is
// idempotent (steady state matches zero rows), the keys come from the
// capabilitySuites bank constants (never literals, never user input), and
// no path can legitimately re-enable a v3 suite — the purge tombstones
// already make "disabled means gone" irreversible for them.
func (db *DB) retireV3SuitesAtOpen() error {
	keys := make([]string, 0, len(capabilitySuites))
	for _, s := range capabilitySuites {
		keys = append(keys, "'"+s.key+"'")
	}
	_, err := db.conn.Exec(
		"UPDATE suites SET enabled = 0 WHERE key IN (" + strings.Join(keys, ", ") + ")",
	)
	return err
}
