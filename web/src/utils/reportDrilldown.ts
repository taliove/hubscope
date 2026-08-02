import type { EvalRun, ReportSuite } from '@/api/types'

// Row drill-down gating (issue #10, design ruling 2026-07-28, option A).
// The shared report page (/report/:token) never opens the trend dialog:
// the trends endpoint sits in the authenticated route group and the share
// token's grant is "this batch's report", not cross-batch history — an
// anonymous row click could only produce a 401. This mirrors /board's
// "rows not clickable" public-page caliber (ui-guidelines §5). Console
// pages (/eval, /campaigns/:id/report) keep drill-down untouched.
export function rowDrilldownEnabled(shared: boolean): boolean {
  return !shared
}

// Cell drill-down gating (GH #156 block 4, settle 后逐题下钻): a score cell
// on the leaderboard opens the per-case run detail filtered to that model.
// The caliber mirrors the row drill-down — console pages only, never the
// shared report page — plus two extra exclusions: live mode (an unfinished
// batch's half-scored cells point at runs still in flight; the settled
// detail is the target) and non-selectable boards (the public /board page,
// same read-only caliber as its rows).
export function cellDrilldownEnabled(shared: boolean, live: boolean, selectable: boolean): boolean {
  return rowDrilldownEnabled(shared) && !live && selectable
}

// Resolve the run behind one leaderboard cell: the report names its suites
// by key, the campaign's runs by suite id. A campaign holds one run per
// suite; if history ever yields several (e.g. a re-run), the done run wins
// because the cell's score comes from the settled verdict. Null when no run
// covers the suite (the caller surfaces an info message instead of opening
// an empty dialog).
export function resolveSuiteRunId(runs: EvalRun[], suites: ReportSuite[], suiteKey: string): number | null {
  const suite = suites.find((s) => s.key === suiteKey)
  if (!suite) return null
  const matches = runs.filter((r) => r.suite_id === suite.id)
  const run = matches.find((r) => r.status === 'done') ?? matches[0]
  return run ? run.id : null
}
