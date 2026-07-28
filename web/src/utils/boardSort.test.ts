// Unit tests for the public board's client-side ranking/filtering (ticket
// 81): the caliber must mirror the server exactly — unscored last,
// descending, model_id tie-break — or the /board page would rank by a
// second caliber.
import { describe, it, expect } from 'vitest'
import type { ReportRow } from '@/api/types'
import { familyOptionsOf, filterRowsByFamily, sortRows } from '@/utils/boardSort'

function makeRow(modelId: string, total: number | null, overrides: Partial<ReportRow> = {}): ReportRow {
  return {
    model_db_id: 1,
    model_id: modelId,
    family: 'fam-a',
    total_score: total,
    total_delta: null,
    suite_scores: { reasoning: total },
    cells: [],
    ...overrides,
  }
}

describe('sortRows', () => {
  it('ranks by the total descending', () => {
    const rows = [makeRow('b', 60), makeRow('a', 90), makeRow('c', 70)]
    expect(sortRows(rows, 'total').map((r) => r.model_id)).toEqual(['a', 'c', 'b'])
  })

  it('sinks unscored models to the bottom, scored ones still descending', () => {
    const rows = [makeRow('unscored', null), makeRow('a', 50), makeRow('b', 90)]
    expect(sortRows(rows, 'total').map((r) => r.model_id)).toEqual(['b', 'a', 'unscored'])
  })

  it('keeps unscored-to-unscored order stable and breaks score ties by model_id', () => {
    const rows = [makeRow('zeta', null), makeRow('b', 80), makeRow('a', 80), makeRow('alpha', null)]
    expect(sortRows(rows, 'total').map((r) => r.model_id)).toEqual(['a', 'b', 'zeta', 'alpha'])
  })

  it('ranks by a suite column with the same caliber', () => {
    const rows = [
      makeRow('a', 90, { suite_scores: { reasoning: 90, coding: 40 } }),
      makeRow('b', 70, { suite_scores: { reasoning: 70, coding: null } }),
      makeRow('c', 60, { suite_scores: { reasoning: 60, coding: 85 } }),
    ]
    expect(sortRows(rows, 'coding').map((r) => r.model_id)).toEqual(['c', 'a', 'b'])
  })

  it('treats a missing suite key as unscored', () => {
    const rows = [makeRow('a', 90, { suite_scores: {} }), makeRow('b', 50, { suite_scores: { reasoning: 50 } })]
    expect(sortRows(rows, 'reasoning').map((r) => r.model_id)).toEqual(['b', 'a'])
  })

  it('handles an empty board and never mutates the input', () => {
    expect(sortRows([], 'total')).toEqual([])
    const rows = [makeRow('b', 60), makeRow('a', 90)]
    sortRows(rows, 'total')
    expect(rows.map((r) => r.model_id)).toEqual(['b', 'a'])
  })

  // Coverage gate (ticket 92, spec 0014 decision A): judged-incomplete rows
  // sink below every complete row under ANY sort column — their real suite
  // scores must never buy them a mid-board spot — and order among
  // themselves by model_id. Rows without the key (live / pre-gate backend)
  // count as complete, mirroring the server's rankable().
  it('sinks judged-incomplete rows below every complete row on the total column', () => {
    const rows = [
      makeRow('inc-a', null, { complete: false, missing_suites: 1, suite_scores: { reasoning: 95 } }),
      makeRow('b', 50, { complete: true }),
      makeRow('a', 90, { complete: true }),
    ]
    expect(sortRows(rows, 'total').map((r) => r.model_id)).toEqual(['a', 'b', 'inc-a'])
  })

  it('sinks judged-incomplete rows even when they top the sorted suite column', () => {
    const rows = [
      makeRow('inc-a', null, { complete: false, missing_suites: 1, suite_scores: { reasoning: 99, coding: 99 } }),
      makeRow('a', 90, { complete: true, suite_scores: { reasoning: 90, coding: 10 } }),
      makeRow('b', 70, { complete: true, suite_scores: { reasoning: 70, coding: 50 } }),
    ]
    expect(sortRows(rows, 'coding').map((r) => r.model_id)).toEqual(['b', 'a', 'inc-a'])
    expect(sortRows(rows, 'reasoning').map((r) => r.model_id)).toEqual(['a', 'b', 'inc-a'])
  })

  it('orders the incomplete group by model_id, not by their scores or input order', () => {
    const rows = [
      makeRow('inc-z', null, { complete: false, missing_suites: 2, suite_scores: { reasoning: 10 } }),
      makeRow('a', 60, { complete: true }),
      makeRow('inc-a', null, { complete: false, missing_suites: 1, suite_scores: { reasoning: 99 } }),
    ]
    expect(sortRows(rows, 'reasoning').map((r) => r.model_id)).toEqual(['a', 'inc-a', 'inc-z'])
  })

  it('treats rows without the complete key as complete (live / pre-gate backend)', () => {
    const rows = [
      makeRow('inc', null, { complete: false, missing_suites: 1, suite_scores: { reasoning: 99 } }),
      makeRow('legacy', 40),
      makeRow('a', 90, { complete: true }),
    ]
    expect(sortRows(rows, 'total').map((r) => r.model_id)).toEqual(['a', 'legacy', 'inc'])
  })

  it('renders an all-incomplete board in model_id order without an empty state', () => {
    const rows = [
      makeRow('inc-b', null, { complete: false, missing_suites: 1 }),
      makeRow('inc-a', null, { complete: false, missing_suites: 2 }),
    ]
    expect(sortRows(rows, 'total').map((r) => r.model_id)).toEqual(['inc-a', 'inc-b'])
  })
})

describe('filterRowsByFamily', () => {
  it('keeps an exact family match only', () => {
    const rows = [makeRow('a', 90, { family: 'kimi' }), makeRow('b', 80, { family: 'claude' })]
    expect(filterRowsByFamily(rows, 'kimi').map((r) => r.model_id)).toEqual(['a'])
  })

  it('keeps every row for an empty family', () => {
    const rows = [makeRow('a', 90), makeRow('b', 80)]
    expect(filterRowsByFamily(rows, '')).toHaveLength(2)
  })
})

describe('familyOptionsOf', () => {
  it('lists distinct families sorted', () => {
    const rows = [makeRow('a', 90, { family: 'kimi' }), makeRow('b', 80, { family: 'claude' }), makeRow('c', 70, { family: 'kimi' })]
    expect(familyOptionsOf(rows)).toEqual(['claude', 'kimi'])
  })

  it('handles an empty board', () => {
    expect(familyOptionsOf([])).toEqual([])
  })
})
