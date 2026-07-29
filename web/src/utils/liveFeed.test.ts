import { describe, expect, it } from 'vitest'
import { isJudgeFailure, LIVE_FEED_CAP, liveFeedCursor, liveFeedDisplay, mergeLiveFeed, verdictTypeLabel } from './liveFeed'
import type { LiveFeedEntry } from '@/api/types'

function entry(id: number): LiveFeedEntry {
  return {
    id,
    model_id: `m-${id}`,
    suite_key: 'gsm8k',
    suite_name: 'GSM8K',
    case_id: id * 10,
    case_prompt: `prompt ${id}`,
    verdict_type: 'rule',
    score: 1,
    latency_ms: 100,
    created_at: '2026-07-29T00:00:00Z',
  }
}

describe('liveFeedCursor', () => {
  it('is 0 before the first pull (empty state)', () => {
    expect(liveFeedCursor([])).toBe(0)
  })

  it('is the last (largest) accumulated id', () => {
    expect(liveFeedCursor([entry(3), entry(7), entry(9)])).toBe(9)
  })
})

describe('mergeLiveFeed', () => {
  it('keeps the empty feed empty on an empty increment', () => {
    expect(mergeLiveFeed([], [])).toEqual([])
  })

  it('appends the increment after the existing entries', () => {
    const merged = mergeLiveFeed([entry(1), entry(2)], [entry(3), entry(4)])
    expect(merged.map((e) => e.id)).toEqual([1, 2, 3, 4])
  })

  it('returns the existing array untouched when the increment is empty', () => {
    const existing = [entry(1)]
    expect(mergeLiveFeed(existing, [])).toBe(existing)
  })

  it('dedupes an overlapping refetch by id', () => {
    const merged = mergeLiveFeed([entry(1), entry(2)], [entry(2), entry(3)])
    expect(merged.map((e) => e.id)).toEqual([1, 2, 3])
  })

  it('caps to the newest entries once the bound is exceeded', () => {
    const existing = Array.from({ length: LIVE_FEED_CAP }, (_, i) => entry(i + 1))
    const merged = mergeLiveFeed(existing, [entry(LIVE_FEED_CAP + 1), entry(LIVE_FEED_CAP + 2)])
    expect(merged).toHaveLength(LIVE_FEED_CAP)
    expect(merged[0].id).toBe(3)
    expect(merged[merged.length - 1].id).toBe(LIVE_FEED_CAP + 2)
  })
})

describe('liveFeedDisplay', () => {
  it('renders newest first without mutating the accumulator', () => {
    const entries = [entry(1), entry(2), entry(3)]
    expect(liveFeedDisplay(entries).map((e) => e.id)).toEqual([3, 2, 1])
    expect(entries.map((e) => e.id)).toEqual([1, 2, 3])
  })
})

describe('verdictTypeLabel', () => {
  it('maps rule and judge to the shared vocabulary', () => {
    expect(verdictTypeLabel('rule')).toBe('规则')
    expect(verdictTypeLabel('judge')).toBe('裁判')
  })

  it('renders a neutral dash for an empty (purged case) or unknown type', () => {
    expect(verdictTypeLabel('')).toBe('-')
    expect(verdictTypeLabel('something-else')).toBe('-')
  })
})

describe('isJudgeFailure', () => {
  it('is true only for an unscored judge verdict (GH #29 deep-link row)', () => {
    expect(isJudgeFailure({ ...entry(1), verdict_type: 'judge', score: null })).toBe(true)
  })

  it('is false for a scored judge verdict', () => {
    expect(isJudgeFailure({ ...entry(1), verdict_type: 'judge', score: 0.5 })).toBe(false)
  })

  it('is false for a rule failure — the link must not appear there', () => {
    expect(isJudgeFailure({ ...entry(1), verdict_type: 'rule', score: null })).toBe(false)
  })

  it('is false for a purged case (empty verdict type)', () => {
    expect(isJudgeFailure({ ...entry(1), verdict_type: '', score: null })).toBe(false)
  })
})
