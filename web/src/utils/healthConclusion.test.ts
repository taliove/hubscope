// Unit tests for the HealthBanner's abnormal chips (GH #53): the banner must
// answer "WHICH endpoints are abnormal", not just "how many" — ranked by the
// board's single severity rank (severitySort, GH #52), capped so a flood of
// abnormal endpoints collapses into a neutral "+N" overflow chip. GH #160
// adds the count-table tests (unverified key, unknown-key defense) and the
// hero unverified sub-note.
import { describe, it, expect } from 'vitest'
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'
import {
  MAX_ABNORMAL_CHIPS,
  abnormalChips,
  countByStatus,
  emptyHealthCounts,
  unverifiedNote,
  verdictUnverifiedNote,
} from '@/utils/healthConclusion'
import { SEVERITY_RANK } from '@/utils/severitySort'

function makeEntry(
  modelId: string,
  status: EndpointStatus,
  overrides: Partial<OverviewEntry> = {},
): OverviewEntry {
  return {
    endpoint_id: 1,
    model_id: modelId,
    protocol: 'openai',
    enabled: true,
    status,
    status_reason: '',
    degrade_causes: [],
    success_rate_24h: null,
    p50_ms: null,
    p95_ms: null,
    last_probe_at: null,
    family: 'fam',
    capability: 'chat',
    score: null,
    score_reasons: [],
    dots_24h: [],
    eval_score: null,
    baseline_p50_ms: null,
    ...overrides,
  }
}

describe('abnormalChips', () => {
  it('ranks failing above down above degraded, ties by model_id', () => {
    const { chips, overflow } = abnormalChips([
      makeEntry('zeta', 'degraded'),
      makeEntry('beta', 'down'),
      makeEntry('alpha', 'down'),
      makeEntry('mid', 'failing'),
      makeEntry('able', 'degraded'),
    ])
    expect(chips.map(c => `${c.status}:${c.model_id}`)).toEqual([
      'failing:mid',
      'down:alpha',
      'down:beta',
      'degraded:able',
      'degraded:zeta',
    ])
    expect(overflow).toBe(0)
  })

  it('orders chips by the shared SEVERITY_RANK (no second rank table)', () => {
    const { chips } = abnormalChips([
      makeEntry('a', 'degraded'),
      makeEntry('b', 'failing'),
      makeEntry('c', 'down'),
    ])
    const ranks = chips.map(c => SEVERITY_RANK[c.status])
    expect(ranks).toEqual([...ranks].sort((x, y) => x - y))
  })

  it('caps at MAX_ABNORMAL_CHIPS and counts the overflow (17 -> 5 + 12)', () => {
    const entries = Array.from({ length: 17 }, (_, i) =>
      makeEntry(`m${String(i).padStart(2, '0')}`, 'down', { endpoint_id: i + 1 }),
    )
    const { chips, overflow } = abnormalChips(entries)
    expect(chips).toHaveLength(MAX_ABNORMAL_CHIPS)
    expect(overflow).toBe(12)
  })

  it('returns empty for an all-healthy set', () => {
    const { chips, overflow } = abnormalChips([makeEntry('a', 'healthy'), makeEntry('b', 'healthy')])
    expect(chips).toEqual([])
    expect(overflow).toBe(0)
  })

  it('returns empty for an empty set', () => {
    const { chips, overflow } = abnormalChips([])
    expect(chips).toEqual([])
    expect(overflow).toBe(0)
  })

  it('lists degraded endpoints in a degraded-only set', () => {
    const { chips, overflow } = abnormalChips([
      makeEntry('b', 'degraded'),
      makeEntry('a', 'degraded'),
      makeEntry('ok', 'healthy'),
    ])
    expect(chips.map(c => c.model_id)).toEqual(['a', 'b'])
    expect(chips.every(c => c.status === 'degraded')).toBe(true)
    expect(overflow).toBe(0)
  })

  it('emits no overflow when exactly at the cap (no "+0")', () => {
    const entries = Array.from({ length: MAX_ABNORMAL_CHIPS }, (_, i) =>
      makeEntry(`m${i}`, 'failing', { endpoint_id: i + 1 }),
    )
    const { chips, overflow } = abnormalChips(entries)
    expect(chips).toHaveLength(MAX_ABNORMAL_CHIPS)
    expect(overflow).toBe(0)
  })

  it('breaks same-model ties by protocol then endpoint_id (dual protocol)', () => {
    const { chips } = abnormalChips([
      makeEntry('m1', 'down', { protocol: 'openai' as Protocol, endpoint_id: 2 }),
      makeEntry('m1', 'down', { protocol: 'anthropic' as Protocol, endpoint_id: 1 }),
    ])
    expect(chips.map(c => c.protocol)).toEqual(['anthropic', 'openai'])
  })

  it('carries identity fields only — no copy literals', () => {
    const { chips } = abnormalChips([makeEntry('a', 'failing')])
    expect(Object.keys(chips[0]).sort()).toEqual(['endpoint_id', 'model_id', 'protocol', 'status'])
  })

  it('never mutates the input array', () => {
    const entries = [makeEntry('b', 'down'), makeEntry('a', 'failing')]
    const snapshot = [...entries]
    abnormalChips(entries)
    expect(entries).toEqual(snapshot)
  })
})

describe('countByStatus / emptyHealthCounts (GH #160)', () => {
  it('covers all five domain values including unverified', () => {
    expect(emptyHealthCounts()).toEqual({ healthy: 0, degraded: 0, down: 0, failing: 0, unverified: 0 })
  })

  it('counts unverified entries under their own key', () => {
    const counts = countByStatus([
      makeEntry('a', 'healthy'),
      makeEntry('b', 'unverified'),
      makeEntry('c', 'unverified'),
    ])
    expect(counts.unverified).toBe(2)
    expect(counts.healthy).toBe(1)
  })

  // GH #160 L1: a runtime status outside the domain union (the wire is
  // untyped) must not turn the table into NaN — the unknown key is ignored.
  it('ignores out-of-domain statuses instead of producing NaN', () => {
    const counts = countByStatus([
      makeEntry('a', 'healthy'),
      makeEntry('b', 'mystery' as EndpointStatus),
    ])
    expect(counts).toEqual({ healthy: 1, degraded: 0, down: 0, failing: 0, unverified: 0 })
  })
})

describe('unverifiedNote (GH #160 ruling ⑦)', () => {
  // The hero conclusion word never changes for unverified (「全部稳定」
  // stays), but the unverified dimension must not be swallowed — a neutral
  // sub-note discloses it.
  it('returns the neutral sub-note when unverified endpoints exist', () => {
    expect(unverifiedNote({ ...emptyHealthCounts(), unverified: 3 })).toBe('3 个未验证')
    expect(unverifiedNote({ ...emptyHealthCounts(), down: 1, unverified: 1 })).toBe('1 个未验证')
  })

  it('returns null when nothing is unverified', () => {
    expect(unverifiedNote(emptyHealthCounts())).toBeNull()
    expect(unverifiedNote({ ...emptyHealthCounts(), down: 2 })).toBeNull()
  })
})

describe('verdictUnverifiedNote (GH #160, main ruling 2026-08-03)', () => {
  // The material verdict mirrors the hero: only the STABLE line carries the
  // note — an abnormal verdict already points at the worst problem, and the
  // distribution string's fourth segment discloses unverified there.
  it('appends the note on the stable line when unverified exists', () => {
    expect(verdictUnverifiedNote('healthy', { ...emptyHealthCounts(), unverified: 2 })).toBe('2 个未验证')
  })

  it('stays silent on abnormal and degraded lines (distribution discloses)', () => {
    const counts = { ...emptyHealthCounts(), down: 1, unverified: 2 }
    expect(verdictUnverifiedNote('abnormal', counts)).toBeNull()
    expect(verdictUnverifiedNote('degraded', { ...emptyHealthCounts(), degraded: 1, unverified: 2 })).toBeNull()
  })

  it('stays silent when nothing is unverified', () => {
    expect(verdictUnverifiedNote('healthy', emptyHealthCounts())).toBeNull()
  })
})
