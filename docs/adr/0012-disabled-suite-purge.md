# ADR 0012 — Disabled-suite hard delete (W2 additive-only exception)

**Date:** 2026-07-28
**Status:** Accepted (landed in ticket 93)
**Spec:** [docs/specs/0014-authoritative-benchmark-eval.md](../specs/0014-authoritative-benchmark-eval.md) (decision B)
**Revises load-bearing wall:** W2 (存储层与 Schema) — first sanctioned exception to "data is only ever added, never deleted".

## Context

The /admin question-bank tab listed four pre-v3 legacy suites (`basic` /
`reasoning` / `coding` / `chinese`) as retired-but-still-listed rows
("旧版套件" tag + "已停用" badge). They were dead weight: out of the
evaluation rotation since question-bank v3 (ADR 0010), never edited again,
and their presence diluted the curation view. The operator decision (spec
0014, explicitly confirmed by the user) is that a disabled suite is deleted
outright — including its data — with the accepted consequence that
historical campaign reports lose the corresponding dimension column and
recompute totals over the remaining dimensions at read time.

W2's invariant "schema migrations are additive-only; data is never deleted"
exists because the single-binary deployment has exactly one copy of the
data. This ADR registers the one sanctioned exception and its guardrails.

## Decision

1. **Generic rule: `enabled = 0` means delete.** A migration running inside
   `store.Open` (after `seedSuites`) hard-deletes every disabled suite in a
   single transaction, leaf to root: `eval_results` → `eval_runs` →
   `cases` → `suites`. The single transaction is a hard requirement — a
   half-deleted suite (suite gone, runs orphaned) would produce ghost data.
   The migration is idempotent: once nothing is disabled, every statement
   matches zero rows, so repeated `Open` calls are no-ops. The same code
   path purges the v3 capability suites when the benchmark cutover
   (ticket 99) retires them — no second deletion mechanism.
2. **Pre-v3 suites are always retired first.** The migration first runs
   `UPDATE suites SET enabled = 0 WHERE capability = ''`, so a database
   upgrading from a pre-v3 binary (skipping the generation-tracked
   retirement of ADR 0010) is purged too. This overrides the old "an admin
   re-enable sticks" semantics for legacy suites — they are gone by
   decision, not by accident.
3. **Seed-bank interplay.** (a) The four legacy suites are removed from the
   seed bank (`builtinSuites` = capability suites only); keeping them would
   re-seed what the purge deletes on every boot. (b) The purge writes a
   tombstone setting `purged_suite_<key>` for every deleted suite, and
   `seedSuites` skips tombstoned bank entries — a suite still present in
   the bank (the capability suites until ticket 99) can never resurrect as
   an enabled empty shell after its rows are deleted. The suite's
   `seed_gen_<key>` generation record is deleted with it.
4. **Historical reports recompute, no compatibility shim.** Reports are
   computed at read time from `eval_runs` JOIN `suites`; purged runs simply
   vanish, so old campaigns lose the deleted dimension and totals
   recompute over the remaining suites (weights unchanged, denominator
   shrinks). Campaigns themselves, their membership snapshots, share links
   and audit history are preserved — only suite/run/result rows are
   deleted. A campaign whose runs all belonged to purged suites renders an
   empty board, never a 500.
5. **Scope boundary.** Only disabled suites are touched. An individually
   disabled case inside an enabled suite stays (question-bank daily editing
   semantics, out of scope per spec 0014).
6. **Frontend.** The "旧版套件" capability filter option and the "已停用"
   suite badge are removed from the case library — disabled suites no
   longer exist to be filtered or badged.

## Consequences

**Positive:**
- The question bank lists only suites that actually exist in the rotation;
  curation is no longer diluted by history.
- No orphan rows: suite deletion is total and transactional.
- One generic mechanism covers the legacy purge now and the v3 purge at the
  benchmark cutover.

**Negative (accepted by the operator):**
- **Irreversible.** Historical campaign reports permanently lose the purged
  dimension columns; totals shift (recomputed over fewer dimensions). This
  is a rewriting of presented history, deliberately accepted in spec 0014.
- Delta baselines across the purge boundary may compare campaigns whose
  suite sets differ; the existing comparability machinery (suite_missing /
  suite_changed) marks them incomparable rather than misleading.

**Neutral:**
- The W2 wall text stands; this ADR is the registered exception, not a
  rewrite. Any future row-level deletion requires its own ADR.

## W2 four questions (this ADR revises W2 by exception)

1. **Why must W2 change?** The retired legacy suites are permanent dead
   weight in every curation view, and the upcoming authoritative-benchmark
   cutover (spec 0014) would add five more retired suites to the list.
   "Never delete" forces the UI to carry an ever-growing graveyard; the
   operator explicitly chose deletion with its consequences over perpetual
   retention.
2. **Which callers are affected?** Direct: `internal/store` (migration
   path, seed bank), report/read paths (`internal/server` campaign report,
   trends, latest scores, shared/public reports — all read-time computed,
   no code change needed), `web/src/components/CaseLibrary.vue` (filter
   option and badge removed). Indirect: historical campaign reports and
   their share links (dimension loss, total recompute — accepted);
   `internal/server` test fixtures that seeded or referenced the legacy
   suites (rewritten to the capability bank in ticket 93).
3. **Alternatives?** (a) Keep retired suites listed (status quo) — rejected
   by the operator. (b) Soft-delete with a `deleted_at` flag — same storage
   cost, same graveyard, plus a second visibility rule everywhere; rejected.
   (c) One-off manual SQL cleanup outside the migration mechanism — leaves
   every deployment to fend for itself and cannot cover the ticket-99
   cutover; rejected in favor of the generic idempotent migration.
4. **Regression testing?** L1: `internal/server/suite_purge_test.go`
   (W1 black-box: cascade deletion observed through the admin API, ghost-run
   404, historical report recompute, empty-board edge, idempotent re-Open,
   enabled-suite regression) and the rewritten pre-v3 upgrade test in
   `eval_versioning_test.go`. L2: the full `internal/server` suite —
   every fixture that referenced the legacy bank was updated and passes.
   L3: `make test` (backend + frontend typecheck + build).

## Out of scope

Per spec 0014: deleting individually disabled cases inside enabled suites;
the benchmark cutover itself (ticket 99, reuses this mechanism); any other
row-level deletion (requires its own ADR).

## Addendum (2026-07-28, ticket 94 merge integration)

The generic rule "enabled = 0 means delete" collided with ADR 0013: benchmark
suites are seeded DISABLED by design pending the ticket-99 cutover, and an
unguarded purge erased them at the same Open that seeded them. Resolution:
the purge exempts exactly the suites the bank seeds disabled by design
(`retireAtGen == 1`). Admin-disabled suites are NOT exempt — an admin
disabling any enabled suite (including a benchmark suite after the cutover)
hands it to this purge at the next Open, tombstoned against resurrection;
that is the ticket-93 semantics the operator approved ("disabled means
gone"). The v3 capability suites at the cutover need no second code path:
disable them and this purge removes them.
