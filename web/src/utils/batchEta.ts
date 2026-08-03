// Batch progress and ETA math (2026-08-03 live-board batch): the running
// batch's progress is judged units, not settled runs — under model-major
// feeding (GH #169) runs all settle near the tail, so a run-count bar pins
// at 0 for most of the batch. ETAs derive from observed per-cell latency:
// a cell's remaining work is its unjudged cases times its own average unit
// latency (campaign-wide average as the fallback for unstarted cells), and
// the batch ETA spreads the remaining unit-seconds over the eval worker
// pool. Pure functions, vitest-pinned; the components only render.
import type { CampaignReport, ReportCell, ReportRow } from '@/api/types'

export interface UnitProgress {
  judged: number
  expected: number
}

// unitProgress sums judged/expected cases over every cell of the report —
// the batch progress caliber (replaces the settled-run count on running
// batches).
export function unitProgress(report: CampaignReport): UnitProgress {
  let judged = 0
  let expected = 0
  for (const row of report.rows) {
    for (const cell of row.cells) {
      judged += cell.judged_cases
      expected += cell.expected_cases
    }
  }
  return { judged, expected }
}

// avgUnitMs is the campaign-wide average latency per judged case — the
// fallback pace for cells that have not judged anything yet. Null when
// nothing has been judged (no pace information at all).
export function avgUnitMs(report: CampaignReport): number | null {
  let latency = 0
  let judged = 0
  for (const row of report.rows) {
    for (const cell of row.cells) {
      if (cell.judged_cases > 0 && cell.latency_ms !== undefined) {
        latency += cell.latency_ms
        judged += cell.judged_cases
      }
    }
  }
  return judged > 0 ? latency / judged : null
}

// cellRemainingMs estimates one cell's remaining time: unjudged cases at
// the cell's own pace (its observed average latency per judged case,
// falling back to the campaign average). Done/failed cells have no ETA
// (their state is terminal), pending/running cells estimate from coverage.
// Null when no pace is known or nothing remains.
export function cellRemainingMs(cell: ReportCell, fallbackMs: number | null): number | null {
  if (cell.status === 'done' || cell.status === 'failed') return null
  const remaining = cell.expected_cases - cell.judged_cases
  if (remaining <= 0) return null
  const pace =
    cell.judged_cases > 0 && cell.latency_ms !== undefined
      ? cell.latency_ms / cell.judged_cases
      : fallbackMs
  return pace === null ? null : remaining * pace
}

// rowRemainingMs sums a model's remaining time over its suites — its cells
// run serially within the cell pool, so the model ETA is the plain sum.
export function rowRemainingMs(row: ReportRow, fallbackMs: number | null): number | null {
  let total = 0
  let any = false
  for (const cell of row.cells) {
    const r = cellRemainingMs(cell, fallbackMs)
    if (r !== null) {
      total += r
      any = true
    }
  }
  return any ? total : null
}

// batchRemainingMs estimates the whole batch's remaining wall-clock: total
// remaining unit-seconds spread over the eval worker pool (concurrency).
// Null when no pace is known or nothing remains. concurrency comes from the
// eval_concurrency setting; clamped to >= 1.
export function batchRemainingMs(report: CampaignReport, concurrency: number): number | null {
  const fallback = avgUnitMs(report)
  if (fallback === null) return null
  let total = 0
  for (const row of report.rows) {
    for (const cell of row.cells) {
      const r = cellRemainingMs(cell, fallback)
      if (r !== null) total += r
    }
  }
  if (total <= 0) return null
  return total / Math.max(1, concurrency)
}
