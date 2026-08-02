// Unit tests for the alert-timeline derivations (GH #117, spec 0018 §12):
// time-range filtering, down↔recovered duration pairing, and local-date
// grouping. All timestamps are built through local-time Date constructors so
// the suites pass in any host timezone.
import { describe, it, expect } from 'vitest'
import {
  filterEventsByTimeRange,
  pairIncidentDurations,
  groupEventsByDate,
  groupEventsByMonthWeekDay,
  alertsFilterToQuery,
  parseAlertsFilterQuery,
  alertSentLabel,
  buildEventDetail,
  ALERTS_FILTER_DEFAULT,
  type AlertTimeRange,
} from '@/utils/alertTimeline'
import { formatTime } from '@/utils/format'
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
    expect(got.get(down.id)).toEqual({ state: 'paired', ms: 5 * 60 * 1000 + 30 * 1000, closerId: rec.id })
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
    expect(got.get(d1.id)).toEqual({ state: 'paired', ms: 60 * 60 * 1000, closerId: r1.id })
    expect(got.get(d2.id)).toEqual({ state: 'paired', ms: 30 * 60 * 1000, closerId: r2.id })
  })

  it('pairs group alerts by group_key, isolated from endpoint scopes', () => {
    const gDown = event('group_down', new Date(2026, 6, 31, 10, 0, 0), { groupKey: 'openai' })
    const gRec = event('group_recovered', new Date(2026, 6, 31, 11, 0, 0), { groupKey: 'openai' })
    // An endpoint recovery for endpoint 3 must not close the group incident.
    const epRec = event('recovered', new Date(2026, 6, 31, 10, 30, 0), { endpointId: 3 })
    const got = pairIncidentDurations([gDown, epRec, gRec])
    expect(got.get(gDown.id)).toEqual({ state: 'paired', ms: 60 * 60 * 1000, closerId: gRec.id })
    expect(got.has(epRec.id)).toBe(false)
  })

  it('keeps different endpoints and different groups isolated', () => {
    const d1 = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 1 })
    const d2 = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 2 })
    const r2 = event('recovered', new Date(2026, 6, 31, 10, 10, 0), { endpointId: 2 })
    const got = pairIncidentDurations([d1, d2, r2])
    expect(got.get(d1.id)).toEqual({ state: 'ongoing' })
    expect(got.get(d2.id)).toEqual({ state: 'paired', ms: 10 * 60 * 1000, closerId: r2.id })
  })

  it('closes the earliest open down on duplicate downs (true outage window)', () => {
    const d1 = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const d2 = event('down', new Date(2026, 6, 31, 10, 5, 0), { endpointId: 7 })
    const rec = event('recovered', new Date(2026, 6, 31, 12, 0, 0), { endpointId: 7 })
    const got = pairIncidentDurations([d1, d2, rec])
    expect(got.get(d1.id)).toEqual({ state: 'paired', ms: 2 * 60 * 60 * 1000, closerId: rec.id })
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

describe('groupEventsByMonthWeekDay (GH #145, spec 0019 T5 — month>week>day nesting)', () => {
  // Fixed "now" for this suite: 2026-08-02 15:00 local — a Sunday, so the
  // current Monday-start week is 7/27–8/2 and straddles the month boundary.
  const now = new Date(2026, 7, 2, 15, 0, 0)

  it('nests events month > week > day, newest first at every level, splitting a straddling week across months', () => {
    const a = event('down', new Date(2026, 7, 2, 10, 0, 0), { endpointId: 1 }) // Sun — today
    const b = event('down', new Date(2026, 6, 30, 9, 0, 0), { endpointId: 1 }) // Thu
    const c = event('down', new Date(2026, 6, 26, 9, 0, 0), { endpointId: 1 }) // Sun — prior week
    const d = event('down', new Date(2026, 6, 20, 9, 0, 0), { endpointId: 1 }) // Mon — prior week
    const months = groupEventsByMonthWeekDay([d, c, b, a], now) // shuffled input

    expect(months.map((m) => m.key)).toEqual(['2026-08', '2026-07'])
    expect(months.map((m) => m.label)).toEqual(['8 月', '7 月'])

    // August holds only Sunday 8/2 of the straddling week 7/27–8/2; July
    // holds its 7/30 day under the SAME Monday key — the week is split, not
    // duplicated into a second label caliber.
    expect(months[0].weeks.map((w) => w.key)).toEqual(['2026-07-27'])
    expect(months[0].weeks[0].label).toBe('7/27–8/2')
    expect(months[0].weeks[0].days.map((g) => g.key)).toEqual(['2026-08-02'])
    expect(months[0].weeks[0].days[0].events).toEqual([a])

    expect(months[1].weeks.map((w) => w.key)).toEqual(['2026-07-27', '2026-07-20'])
    expect(months[1].weeks[0].label).toBe('7/27–8/2')
    expect(months[1].weeks[0].days.map((g) => g.key)).toEqual(['2026-07-30'])
    expect(months[1].weeks[1].label).toBe('7/20–7/26')
    expect(months[1].weeks[1].days.map((g) => g.key)).toEqual(['2026-07-26', '2026-07-20'])
  })

  it('weeks are Monday-start: a Sunday belongs to the week that began the prior Monday', () => {
    // 2026-07-26 is a Sunday; a Sunday-start convention would open a new
    // week 7/26–8/1 instead.
    const sunday = event('down', new Date(2026, 6, 26, 9, 0, 0), { endpointId: 1 })
    const months = groupEventsByMonthWeekDay([sunday], now)
    expect(months[0].weeks[0].key).toBe('2026-07-20')
    expect(months[0].weeks[0].label).toBe('7/20–7/26')
  })

  it('spells the year in the month label only across years', () => {
    const old = event('down', new Date(2025, 11, 30, 9, 0, 0), { endpointId: 1 })
    const sameYear = event('down', new Date(2026, 6, 15, 9, 0, 0), { endpointId: 1 })
    const months = groupEventsByMonthWeekDay([sameYear, old], now)
    expect(months.map((m) => [m.key, m.label])).toEqual([
      ['2026-07', '7 月'],
      ['2025-12', '2025 年 12 月'],
    ])
  })

  it('adds the year to both ends of the week label when the range crosses a year', () => {
    // Monday 2025-12-29 → Sunday 2026-01-04.
    const ev = event('down', new Date(2025, 11, 30, 9, 0, 0), { endpointId: 1 })
    const months = groupEventsByMonthWeekDay([ev], now)
    expect(months[0].weeks[0].key).toBe('2025-12-29')
    expect(months[0].weeks[0].label).toBe('2025/12/29–2026/1/4')
  })

  it('keeps the 今天/昨天 day anchors inside the nested structure (v1 label caliber)', () => {
    const today = event('down', new Date(2026, 7, 2, 9, 0, 0), { endpointId: 1 })
    const yesterday = event('down', new Date(2026, 7, 1, 9, 0, 0), { endpointId: 1 })
    const months = groupEventsByMonthWeekDay([today, yesterday], now)
    const week = months[0].weeks[0]
    expect(week.days.map((g) => g.label)).toEqual(['今天 · 8 月 2 日', '昨天 · 8 月 1 日'])
  })

  it('a single event produces exactly one group at each level', () => {
    const ev = event('down', new Date(2026, 6, 29, 9, 0, 0), { endpointId: 1 })
    const months = groupEventsByMonthWeekDay([ev], now)
    expect(months.length).toBe(1)
    expect(months[0].weeks.length).toBe(1)
    expect(months[0].weeks[0].days.length).toBe(1)
    expect(months[0].weeks[0].days[0].events).toEqual([ev])
  })

  it('returns [] for empty input', () => {
    expect(groupEventsByMonthWeekDay([], now)).toEqual([])
  })
})

describe('buildEventDetail (GH #144, spec 0019 T4 — inline expansion)', () => {
  it('paired down carries the recovery event time and duration from the same pairing map', () => {
    const down = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const rec = event('recovered', new Date(2026, 6, 31, 10, 5, 30), { endpointId: 7 })
    const events = [rec, down]
    const detail = buildEventDetail(down, events, pairIncidentDurations(events))
    expect(detail.pairing).toEqual({
      state: 'paired',
      text: `恢复于 ${formatTime(rec.created_at)} · 持续 5 分 30 秒`,
    })
  })

  it('unpaired down reads 进行中', () => {
    const down = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 7 })
    const detail = buildEventDetail(down, [down], pairIncidentDurations([down]))
    expect(detail.pairing).toEqual({ state: 'ongoing', text: '进行中' })
  })

  it('hub-less non-incident event: no endpoint id, no link target, no pairing row', () => {
    const test = event('test', new Date(2026, 6, 31, 10, 0, 0))
    const detail = buildEventDetail(test, [test], pairIncidentDurations([test]))
    expect(detail.endpointId).toBeNull()
    expect(detail.idText).toBe(`事件 ID #${test.id}`)
    expect(detail.pairing).toEqual({ state: 'none', text: '—' })
  })

  it('group event carries the group_key in the id line and pairs by group scope', () => {
    const gDown = event('group_down', new Date(2026, 6, 31, 10, 0, 0), { groupKey: 'openai' })
    const gRec = event('group_recovered', new Date(2026, 6, 31, 11, 0, 0), { groupKey: 'openai' })
    const events = [gRec, gDown]
    const detail = buildEventDetail(gDown, events, pairIncidentDurations(events))
    expect(detail.idText).toBe(`事件 ID #${gDown.id} · 厂商组 openai`)
    expect(detail.endpointId).toBeNull() // group events deep-link nowhere
    expect(detail.pairing.state).toBe('paired')
  })

  it('endpoint id passes through verbatim so a deleted endpoint stays linkable by raw id', () => {
    const down = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 42 })
    const detail = buildEventDetail(down, [down], pairIncidentDurations([down]))
    expect(detail.endpointId).toBe(42)
    expect(detail.idText).toBe(`事件 ID #${down.id} · 端点 ID #42`)
  })

  it('timestamp is the full local reading with date and seconds (formatTime)', () => {
    const ev = event('down', new Date(2026, 6, 31, 10, 1, 2), { endpointId: 1 })
    const detail = buildEventDetail(ev, [ev], pairIncidentDurations([ev]))
    expect(detail.timestamp).toBe(formatTime(ev.created_at))
    expect(detail.timestamp).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/)
  })

  it('message passes through untruncated', () => {
    const longMessage = 'x'.repeat(500)
    const ev = { ...event('batch', new Date(2026, 6, 31, 10, 0, 0)), message: longMessage }
    const detail = buildEventDetail(ev, [ev], pairIncidentDurations([ev]))
    expect(detail.message).toBe(longMessage)
  })

  it('delivery detail keeps the row vocabulary and appends the raw sent_ok flag', () => {
    const ok = event('down', new Date(2026, 6, 31, 10, 0, 0), { endpointId: 1 })
    expect(buildEventDetail(ok, [ok], new Map()).sentText).toBe('成功 · sent_ok=true')
    const failed = { ...event('down', new Date(2026, 6, 31, 10, 1, 0), { endpointId: 1 }), sent_ok: false }
    expect(buildEventDetail(failed, [failed], new Map()).sentText).toBe('失败 · sent_ok=false')
    const skipped = { ...event('score_drop_skipped', new Date(2026, 6, 31, 10, 2, 0)), sent_ok: false }
    expect(buildEventDetail(skipped, [skipped], new Map()).sentText).toBe('未发送 · sent_ok=false')
  })
})

describe('alertSentLabel (GH #144 — row cell and detail share one source)', () => {
  it('maps sent state to the carried-over vocabulary', () => {
    const at = new Date(2026, 6, 31, 10, 0, 0)
    expect(alertSentLabel(event('down', at, { endpointId: 1 }))).toBe('成功')
    expect(alertSentLabel({ ...event('down', at, { endpointId: 1 }), sent_ok: false })).toBe('失败')
    expect(alertSentLabel(event('score_drop_skipped', at))).toBe('未发送')
  })
})

describe('alertsFilterToQuery / parseAlertsFilterQuery (GH #143)', () => {
  it('serializes only non-default params (clean URL = default view)', () => {
    expect(alertsFilterToQuery(ALERTS_FILTER_DEFAULT)).toEqual({})
    expect(alertsFilterToQuery({ model: null, kind: null, range: '7d' })).toEqual({})
    expect(alertsFilterToQuery({ model: 'gpt-5', kind: 'down', range: '30d' })).toEqual({
      model: 'gpt-5',
      kind: 'down',
      range: '30d',
    })
    expect(alertsFilterToQuery({ model: null, kind: 'recovered', range: '7d' })).toEqual({
      kind: 'recovered',
    })
    expect(alertsFilterToQuery({ model: '', kind: null, range: 'today' })).toEqual({
      range: 'today',
    })
  })

  it('roundtrips every representable state', () => {
    const states = [
      ALERTS_FILTER_DEFAULT,
      { model: 'claude-sonnet-4', kind: null, range: 'today' as const },
      { model: null, kind: 'group_down' as const, range: '24h' as const },
      { model: 'qwen-max', kind: 'score_drop' as const, range: '30d' as const },
    ]
    for (const s of states) {
      expect(parseAlertsFilterQuery(alertsFilterToQuery(s))).toEqual(s)
    }
  })

  it('parses all eleven kinds session-blind (a login-only kind in an anonymous deep link is the codec layer\'s business)', () => {
    expect(parseAlertsFilterQuery({ kind: 'test' }).kind).toBe('test')
    expect(parseAlertsFilterQuery({ kind: 'quiet_summary' }).kind).toBe('quiet_summary')
    expect(parseAlertsFilterQuery({ kind: 'retired' }).kind).toBe('retired')
  })

  it('falls back to defaults on bad values', () => {
    expect(parseAlertsFilterQuery({ range: '1y' }).range).toBe('7d')
    expect(parseAlertsFilterQuery({ range: '7D' }).range).toBe('7d')
    expect(parseAlertsFilterQuery({ range: '' }).range).toBe('7d')
    expect(parseAlertsFilterQuery({ kind: 'exploded' }).kind).toBeNull()
    expect(parseAlertsFilterQuery({ model: '' }).model).toBeNull()
    expect(parseAlertsFilterQuery({})).toEqual(ALERTS_FILTER_DEFAULT)
  })

  it('takes the first value of repeated (array) params — firstQueryValue precedent', () => {
    expect(parseAlertsFilterQuery({ range: ['30d', '7d'] }).range).toBe('30d')
    expect(parseAlertsFilterQuery({ range: ['bogus'] }).range).toBe('7d')
    expect(parseAlertsFilterQuery({ kind: ['down', 'recovered'] }).kind).toBe('down')
    expect(parseAlertsFilterQuery({ model: ['a', 'b'] }).model).toBe('a')
  })

  it('ignores non-string values defensively', () => {
    expect(parseAlertsFilterQuery({ range: 30, kind: null, model: undefined })).toEqual(
      ALERTS_FILTER_DEFAULT,
    )
  })
})
