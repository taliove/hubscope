// Unit tests for the status board's severity ordering (GH #52): the rank
// table is the single source for the whole board — a second caliber
// anywhere (group header, flat matrix, card detail) would re-scatter the
// severe endpoints the first screen is meant to surface. The sharpest
// regression: a DISABLED down endpoint must never lift its group's rank.
import { describe, it, expect } from 'vitest'
import type { EndpointStatus, OverviewEntry, OverviewGroup } from '@/api/types'
import {
  DISABLED_RANK,
  SEVERITY_ORDER,
  SEVERITY_RANK,
  entryRank,
  groupRank,
  sortEntriesByAvailability,
  sortEntriesBySeverity,
  sortGroupSections,
  type GroupSection,
} from '@/utils/severitySort'

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

function makeSection(key: string, entries: OverviewEntry[]): GroupSection {
  const group: OverviewGroup = {
    key,
    endpoint_count: entries.length,
    status_counts: {},
    availability_24h: null,
    avg_latency_ms: null,
  }
  return { group, entries }
}

describe('SEVERITY_RANK / entryRank', () => {
  it('ranks failing above down above degraded above healthy', () => {
    expect(SEVERITY_RANK.failing).toBeLessThan(SEVERITY_RANK.down)
    expect(SEVERITY_RANK.down).toBeLessThan(SEVERITY_RANK.degraded)
    expect(SEVERITY_RANK.degraded).toBeLessThan(SEVERITY_RANK.healthy)
    expect(entryRank(makeEntry('a', 'failing'))).toBe(SEVERITY_RANK.failing)
    expect(entryRank(makeEntry('b', 'healthy'))).toBe(SEVERITY_RANK.healthy)
  })

  // GH #55: SEVERITY_ORDER is the list-form single source consumed by the
  // stats strip and the group header — it must never drift from the rank
  // table, or the board grows a second severity caliber again.
  it('SEVERITY_ORDER covers every status in the same order as SEVERITY_RANK', () => {
    expect(SEVERITY_ORDER).toHaveLength(Object.keys(SEVERITY_RANK).length)
    const ranks = SEVERITY_ORDER.map((s) => SEVERITY_RANK[s])
    expect(ranks).toEqual([...ranks].sort((a, b) => a - b))
  })

  it('ranks a disabled endpoint DISABLED_RANK whatever its status', () => {
    expect(entryRank(makeEntry('a', 'down', { enabled: false }))).toBe(DISABLED_RANK)
    expect(entryRank(makeEntry('b', 'failing', { enabled: false }))).toBe(DISABLED_RANK)
    expect(DISABLED_RANK).toBeGreaterThan(SEVERITY_RANK.healthy)
  })
})

describe('groupRank', () => {
  it('takes the best (smallest) rank among enabled entries', () => {
    const entries = [makeEntry('a', 'healthy'), makeEntry('b', 'degraded'), makeEntry('c', 'failing')]
    expect(groupRank(entries)).toBe(SEVERITY_RANK.failing)
  })

  it('is never lifted by a disabled down endpoint', () => {
    const entries = [
      makeEntry('a', 'healthy'),
      makeEntry('b', 'down', { enabled: false }),
      makeEntry('c', 'failing', { enabled: false }),
    ]
    expect(groupRank(entries)).toBe(SEVERITY_RANK.healthy)
  })

  it('ranks DISABLED_RANK when the group has no enabled entries', () => {
    expect(groupRank([])).toBe(DISABLED_RANK)
    expect(groupRank([makeEntry('a', 'down', { enabled: false })])).toBe(DISABLED_RANK)
  })
})

describe('sortEntriesBySeverity', () => {
  it('ranks by severity ascending and sinks disabled entries to the bottom', () => {
    const entries = [
      makeEntry('c', 'healthy'),
      makeEntry('a', 'failing'),
      makeEntry('d', 'down', { enabled: false }),
      makeEntry('b', 'down'),
    ]
    expect(sortEntriesBySeverity(entries).map((e) => e.model_id)).toEqual(['a', 'b', 'c', 'd'])
  })

  it('breaks ties by model_id, then protocol, then endpoint_id', () => {
    const entries = [
      makeEntry('b', 'degraded', { protocol: 'openai', endpoint_id: 2 }),
      makeEntry('a', 'degraded', { protocol: 'openai', endpoint_id: 9 }),
      makeEntry('b', 'degraded', { protocol: 'openai', endpoint_id: 1 }),
      makeEntry('b', 'degraded', { protocol: 'anthropic', endpoint_id: 5 }),
    ]
    expect(sortEntriesBySeverity(entries).map((e) => e.endpoint_id)).toEqual([9, 5, 1, 2])
  })

  it('handles an empty board and never mutates the input', () => {
    expect(sortEntriesBySeverity([])).toEqual([])
    const entries = [makeEntry('b', 'healthy'), makeEntry('a', 'failing')]
    sortEntriesBySeverity(entries)
    expect(entries.map((e) => e.model_id)).toEqual(['b', 'a'])
  })
})

describe('sortEntriesByAvailability', () => {
  it('ranks the lowest 24h availability first', () => {
    const sorted = sortEntriesByAvailability([
      makeEntry('good', 'healthy', { success_rate_24h: 0.999 }),
      makeEntry('bad', 'down', { success_rate_24h: 0.4 }),
      makeEntry('mid', 'degraded', { success_rate_24h: 0.93 }),
    ])
    expect(sorted.map(e => e.model_id)).toEqual(['bad', 'mid', 'good'])
  })

  it('sinks a null rate below every rated row and disabled rows last', () => {
    const sorted = sortEntriesByAvailability([
      makeEntry('disabled', 'down', { enabled: false, success_rate_24h: 0 }),
      makeEntry('nodata', 'healthy', { success_rate_24h: null }),
      makeEntry('perfect', 'healthy', { success_rate_24h: 1 }),
      makeEntry('zero', 'down', { success_rate_24h: 0 }),
    ])
    expect(sorted.map(e => e.model_id)).toEqual(['zero', 'perfect', 'nodata', 'disabled'])
  })

  it('breaks rate ties by the severity rank, then model_id', () => {
    const sorted = sortEntriesByAvailability([
      makeEntry('b-down', 'down', { success_rate_24h: 0 }),
      makeEntry('a-failing', 'failing', { success_rate_24h: 0 }),
      makeEntry('a-down', 'down', { success_rate_24h: 0 }),
    ])
    expect(sorted.map(e => e.model_id)).toEqual(['a-failing', 'a-down', 'b-down'])
  })

  it('never mutates the input array', () => {
    const input = [
      makeEntry('good', 'healthy', { success_rate_24h: 1 }),
      makeEntry('bad', 'down', { success_rate_24h: 0.5 }),
    ]
    sortEntriesByAvailability(input)
    expect(input.map(e => e.model_id)).toEqual(['good', 'bad'])
  })
})

describe('sortGroupSections', () => {
  it('ranks groups by their most severe enabled entry, ties by group key', () => {
    const sections = [
      makeSection('zeta', [makeEntry('a', 'healthy')]),
      makeSection('beta', [makeEntry('b', 'degraded')]),
      makeSection('alpha', [makeEntry('c', 'down'), makeEntry('d', 'healthy')]),
    ]
    expect(sortGroupSections(sections).map((s) => s.group.key)).toEqual(['alpha', 'beta', 'zeta'])
  })

  it('sinks empty-after-filter groups to the bottom, ordered by key', () => {
    const sections = [
      makeSection('z-empty', []),
      makeSection('a-empty', []),
      makeSection('mid', [makeEntry('a', 'healthy')]),
    ]
    expect(sortGroupSections(sections).map((s) => s.group.key)).toEqual(['mid', 'a-empty', 'z-empty'])
  })

  it('sinks groups whose only entries are disabled, however severe', () => {
    const sections = [
      makeSection('disabled-down', [makeEntry('a', 'down', { enabled: false })]),
      makeSection('enabled-healthy', [makeEntry('b', 'healthy')]),
    ]
    expect(sortGroupSections(sections).map((s) => s.group.key)).toEqual([
      'enabled-healthy',
      'disabled-down',
    ])
  })

  it('ranks entries inside each section and never mutates the input', () => {
    const sections = [
      makeSection('g', [makeEntry('b', 'healthy'), makeEntry('a', 'failing')]),
    ]
    const sorted = sortGroupSections(sections)
    expect(sorted[0]!.entries.map((e) => e.model_id)).toEqual(['a', 'b'])
    expect(sections[0]!.entries.map((e) => e.model_id)).toEqual(['b', 'a'])
  })
})
