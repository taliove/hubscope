package store

// purgeDisabledSuites hard-deletes every disabled suite together with its
// cases, the eval runs that referenced it, and those runs' results — leaf to
// root (results → runs → cases → suites) inside a single transaction, so a
// crash can never leave a half-deleted suite behind.
//
// This is spec 0014 decision B (ADR 0012): the one sanctioned exception to
// W2's "additive-only" data rule. Retired suites used to stay readable
// forever; the operator decision is that a disabled suite is dead weight and
// must disappear completely, historical reports recomputing over the
// remaining dimensions at read time. The rule is generic — "enabled = 0
// means delete" — so any suite retired later (e.g. the v3 capability suites
// at the benchmark cutover) is purged by this same migration with no second
// code path. Enabled suites and individually disabled cases inside them are
// never touched.
//
// The seed-generation records of purged suites are replaced by tombstones
// (purged_suite_<key>): the seed bank skips tombstoned suites, so a suite
// still present in the bank (e.g. the capability suites until the benchmark
// cutover) can never be re-seeded back to life after the purge.
//
// Idempotent: once nothing is disabled, every statement matches zero rows.
func (db *DB) purgeDisabledSuites() error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		// Pre-v3 legacy suites (empty capability) are dead weight by
		// definition: retire them first so upgrades that skip the
		// generation-tracked retirement (a pre-v3 binary jumping straight
		// to this version) are purged too. Capability suites are only
		// purged once something else disabled them.
		`UPDATE suites SET enabled = 0 WHERE capability = ''`,
		`INSERT OR REPLACE INTO settings (key, value)
			SELECT 'purged_suite_' || key, '1' FROM suites WHERE enabled = 0`,
		`DELETE FROM eval_results WHERE eval_run_id IN (
			SELECT id FROM eval_runs WHERE suite_id IN (
				SELECT id FROM suites WHERE enabled = 0))`,
		`DELETE FROM eval_runs WHERE suite_id IN (
			SELECT id FROM suites WHERE enabled = 0)`,
		`DELETE FROM cases WHERE suite_id IN (
			SELECT id FROM suites WHERE enabled = 0)`,
		`DELETE FROM settings WHERE key IN (
			SELECT 'seed_gen_' || key FROM suites WHERE enabled = 0)`,
		`DELETE FROM suites WHERE enabled = 0`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
