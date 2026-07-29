package store

import "strings"

// benchmarkCutoverKey is the settings key recording that the ticket-99
// benchmark cutover has run on this database.
const benchmarkCutoverKey = "benchmark_cutover"

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
