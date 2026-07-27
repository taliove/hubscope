// Unit tests for the shared score-cell pure functions (ticket 78, spec 0009):
// band thresholds, coverage watermark, confidence tooltip, live-cell status
// wording and live counts. The band boundaries are the load-bearing caliber
// pinned here — they mirror the backend score badge and never move.
import { describe, it, expect } from 'vitest'
import type { ReportCell } from '@/api/types'
import { cellStatusText, liveCounts, scoreBand, tooltipOf, watermarkOf } from '@/utils/scoreTier'

function makeCell(suiteKey: string, overrides: Partial<ReportCell> = {}): ReportCell {
  return {
    suite_key: suiteKey,
    status: 'done',
    judged_cases: 10,
    expected_cases: 10,
    samples: 30,
    ...overrides,
  }
}

describe('scoreBand', () => {
  it('maps the 80 boundary to success', () => {
    expect(scoreBand(80)).toBe('success')
    expect(scoreBand(79.9)).toBe('warning')
  })
  it('maps the 50 boundary to warning', () => {
    expect(scoreBand(50)).toBe('warning')
    expect(scoreBand(49.9)).toBe('danger')
  })
})

describe('watermarkOf', () => {
  it('appends the compressed watermark when a done suite judged fewer cases than expected', () => {
    expect(watermarkOf(makeCell('a', { judged_cases: 8, expected_cases: 10 }))).toBe('·8/10')
  })

  it('shows no watermark at full coverage or for non-done cells', () => {
    expect(watermarkOf(makeCell('a'))).toBe('')
    expect(watermarkOf(makeCell('a', { status: 'running' }))).toBe('')
    expect(watermarkOf(undefined)).toBe('')
  })
})

describe('tooltipOf', () => {
  it('carries the ticket-51 confidence caliber', () => {
    expect(tooltipOf('推理', 75, makeCell('a', { judged_cases: 8, expected_cases: 10 }))).toBe(
      '推理 · 75.0 · 判分 8/10 题 · 采样 30 次',
    )
  })

  it('degrades gracefully when the cell is absent or has no judged cases', () => {
    expect(tooltipOf('推理', 75, undefined)).toBe('推理 · 75.0')
    expect(tooltipOf('推理', 75, makeCell('a', { judged_cases: 0 }))).toBe('推理 · 75.0')
  })
})

describe('cellStatusText', () => {
  it('uses the batch/run vocabulary for live cells', () => {
    expect(cellStatusText(makeCell('a', { status: 'pending' }))).toBe('等待中')
    expect(cellStatusText(makeCell('a', { status: 'running' }))).toBe('进行中')
    expect(cellStatusText(makeCell('a', { status: 'failed' }))).toBe('失败')
  })

  it('falls back to a neutral wording for done cells and absent cells', () => {
    expect(cellStatusText(makeCell('a'))).toBe('未判分')
    expect(cellStatusText(undefined)).toBe('未判分')
  })
})

describe('liveCounts', () => {
  it('counts pending/running as in flight and failed separately', () => {
    const cells = [
      makeCell('a', { status: 'pending' }),
      makeCell('b', { status: 'running' }),
      makeCell('c', { status: 'failed' }),
      makeCell('d', { status: 'done' }),
    ]
    expect(liveCounts(cells)).toEqual({ inFlight: 2, failed: 1 })
  })

  it('returns zeros for a settled batch', () => {
    expect(liveCounts([makeCell('a'), makeCell('b')])).toEqual({ inFlight: 0, failed: 0 })
  })
})
