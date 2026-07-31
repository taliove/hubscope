// Unit tests for the alert-timeline derivations (GH #117, spec 0018 §12):
// time-range filtering, down↔recovered duration pairing, and local-date
// grouping. All timestamps are built through local-time Date constructors so
// the suites pass in any host timezone.
import { describe, it, expect } from 'vitest'
import {
  filterEventsByTimeRange,
  pairIncidentDurations,
  groupEventsByDate,
  type AlertTimeRange,
} from '@/utils/alertTimeline'
import type { AlertEvent, AlertKind } from '@/api/settings'

// Fixed "now" for every suite: 2026-07-31 15:00 local.
function makeNow(): Date {
  return new Date(2026, 6, 31, 15, 0, 0)
}

let nextId = 1
// Local-time constructor + toISOString keeps comparisons ms-exact while the
// stored string stays RFC3339 like the API payload.
function event(
  kind: AlertKind,
  at: Date,
  opts: { endpointId?: number | null; groupKey?: string | null } = {},
): AlertEvent {
  return {
    id: nextId++,
    endpoint_id: opts.endpointId ?? null,
    kind,
    message: `${kind} message`,
    sent_ok: true,
    created_at: at.toISOString(),
    group_key: opts.groupKey ?? null,
  }
}

beforeEachReset()
function beforeEachReset() {
  nextId = 1
}

describe('filterEventsByTimeRange', () => {
  const now = makeNow()
  const cases: Array<{ range: AlertTimeRange; at: Date; kept: boolean; note: string }> = [
    { range: 'today', at: new Date(2026, 6, 31, 0, 0, 0), kept: true, note: 'today at local midnight is kept' },
    { range: 'today', at: new Date(2026, 6, 30, 23, 59, 59), kept: false, note: 'yesterday 23:59 is outside today' },
    { range: '24h', at: new Date(2026, 6, 30, 15, 0, 0), kept: true, note: 'exactly 24h ago is kept' },
    { range: '24h', at: new Date(2026, 6, 30, 14, 59, 59), kept: false, note: '24h + 1s ago is outside' },
    { range: '7d', at: new Date(2026, 6, 24, 15, 0, 0), kept: true, note: 'exactly 7 days ago is kept' },
    { range: '7d', at: new Date(2026, 6, 23, 15, 0, 0), kept: false, note: '8 days ago is outside 7d' },
    { range: '30d', at: new Date(2026, 6, 1, 15, 0, 0), kept: true, note: 'exactly 30 days ago is kept' },
    { range: '30d', at: new Date(2026, 5, 30, 15, 0, 0), kept: false, note: '31 days ago is outside 30d' },
  ]

  for (const c of cases) {
    it(`${c.range}: ${c.note}`, () => {
      const ev = event('down', c.at, { endpointId: 1 })
      const got = filterEventsByTimeRange([ev], c.range, now)
      expect(got.length).toBe(c.kept ? 1 : 0)
    })
  }

  it('drops future events and unparseable timestamps defensively', () => {
    const future = event('down', new Date(2026, 7, 1, 0, 0, 0), { endpointId: 1 })
    const broken = { ...event('down', now, { endpointId: 1 }), created_at: 'not-a-date' }
    expect(filterEventsByTimeRange([future, broken], '30d', now)).toEqual([])
  })
})

describe('pairIncidentDurations', () => {
  it('pairs a down with the next recovery of the same endpoint', () => {
    const down = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const rec = event('recovered', new Date(2026, 6, 31, 10, 5, 30), { endpointId: 7 })
    const got = pairIncidentDurations([rec, down]) // input order must not matter
    expect(got.get(down.id)).toEqual({ state: 'paired', ms: 5 * 60 * 1000 + 30 * 1000 })
    expect(got.has(rec.id)).toBe(false)
  })

  it('marks a down without recovery as ongoing', () => {
    const down = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const got = pairIncidentDurations([down])
    expect(got.get(down.id)).toEqual({ state: 'ongoing' })
  })

  it('pairs alternating cycles independently (down1→rec1, down2→rec2)', () => {
    const d1 = event('down', new Date(2026, 6, 30, 8, 0, 0), { endpointId: 7 })
    const r1 = event('recovered', new Date(2026, 6, 30, 9, 0, 0), { endpointId: 7 })
    const d2 = event('down', new Date(2026, 6, 31, 8, 0, 0), { endpointId: 7 })
    const r2 = event('recovered', new Date(2026, 6, 31, 8, 30, 0), { endpointId: 7 })
    const got = pairIncidentDurations([d1, r1, d2, r2])
    expect(got.get(d1.id)).toEqual({ state: 'paired', ms: 60 * 60 * 1000 })
    expect(got.get(d2.id)).toEqual({ state: 'paired', ms: 30 * 60 * 1000 })
  })

  it('pairs group alerts by group_key, isolated from endpoint scopes', () => {
    const gDown = event('group_down', new Date(2026, 6, 31, 10, 0, 0), { groupKey: 'openai' })
    const gRec = event('group_recovered', new Date(2026, 6, 31, 11, 0, 0), { groupKey: 'openai' })
    // An endpoint recovery for endpoint 3 must not close the group incident.
    const epRec = event('recovered', new Date(2026, 6, 31, 10, 30, 0), { endpointId: 3 })
    const got = pairIncidentDurations([gDown, epRec, gRec])
    expect(got.get(gDown.id)).toEqual({ state: 'paired', ms: 60 * 60 * 1000 })
    expect(got.has(epRec.id)).toBe(false)
  })

  it('keeps different endpoints and different groups isolated', () => {
    const d1 = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 1 })
    const d2 = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 2 })
    const r2 = event('recovered', new Date(2026, 6, 31, 10, 10, 0), { endpointId: 2 })
    const got = pairIncidentDurations([d1, d2, r2])
    expect(got.get(d1.id)).toEqual({ state: 'ongoing' })
    expect(got.get(d2.id)).toEqual({ state: 'paired', ms: 10 * 60 * 1000 })
  })

  it('closes the earliest open down on duplicate downs (true outage window)', () => {
    const d1 = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const d2 = event('down', new Date(2026, 6, 31, 10, 5, 0), { endpointId: 7 })
    const rec = event('recovered', new Date(2026, 6, 31, 12, 0, 0), { endpointId: 7 })
    const got = pairIncidentDurations([d1, d2, rec])
    expect(got.get(d1.id)).toEqual({ state: 'paired', ms: 2 * 60 * 60 * 1000 })
    expect(got.get(d2.id)).toEqual({ state: 'ongoing' })
  })

  it('ignores dangling recoveries and non-incident kinds', () => {
    const rec = event('recovered', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const test = event('test', new Date(2026, 6, 31, 10, 1, 0))
    const batch = event('batch', new Date(2026, 6, 31, 10, 2, 0))
    const got = pairIncidentDurations([rec, test, batch])
    expect(got.size).toBe(0)
  })

  it('never pairs endpoint events without an endpoint_id', () => {
    const down = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: null })
    const rec = event('recovered', new Date(2026, 6, 31, 11, 0, 0), { endpointId: null })
    expect(pairIncidentDurations([down, rec]).size).toBe(0)
  })
})

describe('groupEventsByDate', () => {
  const now = makeNow()

  it('anchors today and yesterday with the date spelled out', () => {
    const today = event('down', new Date(2026, 6, 31, 9, 0, 0), { endpointId: 1 })
    const yesterday = event('down', new Date(2026, 6, 30, 9, 0, 0), { endpointId: 1 })
    const groups = groupEventsByDate([today, yesterday], now)
    expect(groups.map((g) => g.label)).toEqual(['今天 · 7 月 31 日', '昨天 · 7 月 30 日'])
    expect(groups[0].events).toEqual([today])
    expect(groups[1].events).toEqual([yesterday])
  })

  it('omits the year inside the current year and spells it out across years', () => {
    const older = event('down', new Date(2026, 6, 3, 9, 0, 0), { endpointId: 1 })
    const lastYear = event('down', new Date(2025, 11, 3, 9, 0, 0), { endpointId: 1 })
    const groups = groupEventsByDate([older, lastYear], now)
    expect(groups.map((g) => g.label)).toEqual(['7 月 3 日', '2025 年 12 月 3 日'])
  })

  it('keeps input order inside a day and orders sections newest first even when shuffled', () => {
    const a = event('down', new Date(2026, 6, 30, 9, 0, 0), { endpointId: 1 })
    const b = event('recovered', new Date(2026, 6, 30, 8, 0, 0), { endpointId: 1 })
    const c = event('down', new Date(2026, 6, 31, 9, 0, 0), { endpointId: 1 })
    const groups = groupEventsByDate([a, b, c], now)
    expect(groups.map((g) => g.key)).toEqual(['2026-07-31', '2026-07-30'])
    expect(groups[1].events).toEqual([a, b])
  })

  it('skips unparseable timestamps', () => {
    const broken = { ...event('down', now, { endpointId: 1 }), created_at: 'garbage' }
    expect(groupEventsByDate([broken], now)).toEqual([])
  })
})
