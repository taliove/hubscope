package store

import "strings"

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
// means delete" — so any suite retired later is purged by this same
// migration with no second code path; the v3 capability suites went this way
// at the ticket-99 benchmark cutover. The single exemption is suites the
// bank seeds disabled BY DESIGN (retireAtGen == 1): deleting them at Open
// would erase them before they ever enter the rotation. No bank entry
// carries retireAtGen 1 since the cutover (the exemption existed for the
// benchmark suites pending it, ADR 0013); the mechanism stays for any future
// pre-seeded bank. An admin-disabled suite is NOT exempt: an admin disabling
// any enabled suite (a benchmark suite included) hands it to this purge at
// the next Open, tombstoned against resurrection (ticket 93 semantics:
// disabled means gone). Enabled suites and individually disabled cases
// inside them are never touched.
//
// The seed-generation records of purged suites are replaced by tombstones
// (purged_suite_<key>): the seed bank skips tombstoned suites, so a suite
// still present in the bank (the v3 capability suites, whose bank entries
// remain so existing databases learn their retirement) can never be re-seeded
// back to life after the purge.
//
// Idempotent: once nothing is disabled, every statement matches zero rows.
func (db *DB) purgeDisabledSuites() error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Exempt only suites the bank seeds disabled by design (retireAtGen == 1;
	// empty since the ticket-99 cutover enabled the benchmark bank, kept for
	// any future pre-seeded bank). Keys are compile-time constants from our
	// own bank, never user input.
	exemptKeys := make([]string, 0, len(builtinSuites))
	for _, s := range builtinSuites {
		if s.retireAtGen == 1 {
			exemptKeys = append(exemptKeys, "'"+s.key+"'")
		}
	}
	doomed := "enabled = 0"
	if len(exemptKeys) > 0 {
		doomed += " AND key NOT IN (" + strings.Join(exemptKeys, ", ") + ")"
	}

	statements := []string{
		// Pre-v3 legacy suites (empty capability) are dead weight by
		// definition: retire them first so upgrades that skip the
		// generation-tracked retirement (a pre-v3 binary jumping straight
		// to this version) are purged too. Capability suites are only
		// purged once something else disabled them.
		`UPDATE suites SET enabled = 0 WHERE capability = ''`,
		`INSERT OR REPLACE INTO settings (key, value)
			SELECT 'purged_suite_' || key, '1' FROM suites WHERE ` + doomed,
		`DELETE FROM eval_results WHERE eval_run_id IN (
			SELECT id FROM eval_runs WHERE suite_id IN (
				SELECT id FROM suites WHERE ` + doomed + `))`,
		`DELETE FROM eval_runs WHERE suite_id IN (
			SELECT id FROM suites WHERE ` + doomed + `)`,
		`DELETE FROM cases WHERE suite_id IN (
			SELECT id FROM suites WHERE ` + doomed + `)`,
		`DELETE FROM settings WHERE key IN (
			SELECT 'seed_gen_' || key FROM suites WHERE ` + doomed + `)`,
		`DELETE FROM suites WHERE ` + doomed,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
