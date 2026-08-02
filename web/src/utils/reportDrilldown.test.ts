import { describe, expect, it } from 'vitest'
import { cellDrilldownEnabled, resolveSuiteRunId, rowDrilldownEnabled } from './reportDrilldown'
import type { EvalRun, ReportSuite } from '@/api/types'

// Issue #10 (design ruling 2026-07-28, option A): the shared report page
// (/report/:token) never drills down — the trends endpoint is in the
// authenticated route group and the share token's grant is "this batch's
// report", so a row click by an anonymous visitor would only produce a 401.
describe('rowDrilldownEnabled', () => {
  it('disables row drill-down on the shared report page', () => {
    expect(rowDrilldownEnabled(true)).toBe(false)
  })

  it('keeps row drill-down on console pages (/eval, /campaigns/:id/report)', () => {
    expect(rowDrilldownEnabled(false)).toBe(true)
  })
})

// GH #156 block 4: a score cell drills into the per-case run detail of that
// model x suite. Cells follow the row's console-only caliber and add two
// exclusions of their own — live half-scored boards and non-selectable
// public boards never grow clickable cells.
describe('cellDrilldownEnabled', () => {
  it('enables cell drill-down on a settled console board', () => {
    expect(cellDrilldownEnabled(false, false, true)).toBe(true)
  })

  it('disables cell drill-down on the shared report page', () => {
    expect(cellDrilldownEnabled(true, false, true)).toBe(false)
  })

  it('disables cell drill-down in live mode (unfinished batch)', () => {
    expect(cellDrilldownEnabled(false, true, true)).toBe(false)
  })

  it('disables cell drill-down on non-selectable boards (/board)', () => {
    expect(cellDrilldownEnabled(false, false, false)).toBe(false)
  })
})

function suite(partial: Partial<ReportSuite> & { id: number; key: string }): ReportSuite {
  return { name: partial.key, version: 1, ...partial }
}

function run(partial: Partial<EvalRun> & { id: number; suite_id: number }): EvalRun {
  return {
    campaign_id: 1,
    suite_version: 1,
    nadir: 0,
    trigger: 'manual',
    judge_model: 'judge',
    status: 'done',
    started_at: '2026-08-01T00:00:00Z',
    finished_at: '2026-08-01T00:01:00Z',
    score: 0.8,
    ...partial,
  }
}

// GH #156 block 4: the report names suites by key, the campaign's runs by
// suite id — the resolver bridges the two namespaces for one clicked cell.
describe('resolveSuiteRunId', () => {
  const suites = [suite({ id: 10, key: 'gsm8k' }), suite({ id: 20, key: 'mmlu' })]

  it('resolves the run whose suite_id matches the clicked suite key', () => {
    const runs = [run({ id: 101, suite_id: 10 }), run({ id: 102, suite_id: 20 })]
    expect(resolveSuiteRunId(runs, suites, 'mmlu')).toBe(102)
  })

  it('returns null when the suite key is not in the report', () => {
    const runs = [run({ id: 101, suite_id: 10 })]
    expect(resolveSuiteRunId(runs, suites, 'unknown')).toBeNull()
  })

  it('returns null when no run covers the suite', () => {
    const runs = [run({ id: 102, suite_id: 20 })]
    expect(resolveSuiteRunId(runs, suites, 'gsm8k')).toBeNull()
  })

  it('prefers the done run when several runs cover one suite', () => {
    const runs = [
      run({ id: 101, suite_id: 10, status: 'failed' }),
      run({ id: 103, suite_id: 10, status: 'done' }),
    ]
    expect(resolveSuiteRunId(runs, suites, 'gsm8k')).toBe(103)
  })
})
