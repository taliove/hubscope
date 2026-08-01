// Unit tests for the recent-events card derivations (GH #132, UI v2 O5;
// GH #138 chip double-track).
// All timestamps are built through local-time Date constructors so the
// suites pass in any host timezone (alertTimeline.test.ts precedent).
import { describe, it, expect } from 'vitest'
import {
  RECENT_EVENTS_CARD_LIMIT,
  buildEndpointModelMap,
  eventChip,
  eventTitle,
  impactText,
  incidentDurationText,
  selectRecentEvents,
} from '@/utils/recentEvents'
import { pairIncidentDurations } from '@/utils/alertTimeline'
import type { AlertEvent, AlertKind } from '@/api/settings'
import type { OverviewEntry } from '@/api/types'

let nextId = 1
// Local-time constructor + toISOString keeps comparisons ms-exact while the
// stored string stays RFC3339 like the API payload.
function event(
  kind: AlertKind,
  at: Date,
  opts: { endpointId?: number | null; groupKey?: string | null; message?: string } = {},
): AlertEvent {
  return {
    id: nextId++,
    endpoint_id: opts.endpointId ?? null,
    kind,
    message: opts.message ?? `${kind} happened`,
    sent_ok: true,
    created_at: at.toISOString(),
    group_key: opts.groupKey ?? null,
  }
}

function overviewEntry(endpointId: number, modelId: string): OverviewEntry {
  return { endpoint_id: endpointId, model_id: modelId } as OverviewEntry
}

describe('eventTitle', () => {
  it('returns a single-line message unchanged', () => {
    expect(eventTitle('端点告警:1 个端点连续 3 次探测失败')).toBe('端点告警:1 个端点连续 3 次探测失败')
  })

  it('keeps only the first line of an aggregate message', () => {
    expect(eventTitle('首行标题\n· gpt-4o(openai):连续 3 次探测失败\n· claude(anthropic):同上')).toBe(
      '首行标题',
    )
  })

  it('trims surrounding whitespace', () => {
    expect(eventTitle('  带空白的首行  \n次行')).toBe('带空白的首行')
  })
})

describe('buildEndpointModelMap', () => {
  it('maps endpoint ids to model ids', () => {
    const map = buildEndpointModelMap([overviewEntry(1, 'gpt-4o'), overviewEntry(2, 'claude-4')])
    expect(map.get(1)).toBe('gpt-4o')
    expect(map.get(2)).toBe('claude-4')
  })
})

describe('impactText', () => {
  const map = buildEndpointModelMap([overviewEntry(7, 'gpt-4o')])

  it('names the resolved model for endpoint events', () => {
    const ev = event('down', new Date(2026, 6, 31, 14, 0), { endpointId: 7 })
    expect(impactText(ev, map)).toBe('影响 gpt-4o')
  })

  it('falls back to the raw id when the endpoint left the overview', () => {
    const ev = event('down', new Date(2026, 6, 31, 14, 0), { endpointId: 99 })
    expect(impactText(ev, map)).toBe('影响 #99')
  })

  it('names the vendor group for group events', () => {
    const ev = event('group_down', new Date(2026, 6, 31, 14, 0), { groupKey: 'openai' })
    expect(impactText(ev, map)).toBe('影响 openai')
  })

  it('shows the event-kind word for hub-less events', () => {
    const ev = event('test', new Date(2026, 6, 31, 14, 0))
    expect(impactText(ev, map)).toBe('测试')
  })
})

describe('eventChip (double-track: word = state or kind, tone = kind tag type)', () => {
  const t0 = new Date(2026, 6, 31, 14, 0, 0)
  const later = (minutes: number) => new Date(t0.getTime() + minutes * 60_000)

  it('reads 已恢复 on the danger tone for a paired down', () => {
    const down = event('down', t0, { endpointId: 7 })
    const recovered = event('recovered', later(12), { endpointId: 7 })
    const durations = pairIncidentDurations([recovered, down])
    expect(eventChip(down, durations)).toEqual({ text: '已恢复', tone: 'danger' })
  })

  it('reads 进行中 on the danger tone for an unpaired down', () => {
    const down = event('down', t0, { endpointId: 7 })
    const durations = pairIncidentDurations([down])
    expect(eventChip(down, durations)).toEqual({ text: '进行中', tone: 'danger' })
  })

  it('pairs group incidents by group key (danger tone)', () => {
    const down = event('group_down', t0, { groupKey: 'openai' })
    const recovered = event('group_recovered', later(65), { groupKey: 'openai' })
    const durations = pairIncidentDurations([down, recovered])
    expect(eventChip(down, durations)).toEqual({ text: '已恢复', tone: 'danger' })
  })

  it('reads the kind word on the success tone for the recovery event itself', () => {
    const down = event('down', t0, { endpointId: 7 })
    const recovered = event('recovered', later(12), { endpointId: 7 })
    const durations = pairIncidentDurations([down, recovered])
    expect(eventChip(recovered, durations)).toEqual({ text: '恢复', tone: 'success' })
  })

  it('reads the kind word for point-in-time kinds, tone per the kind mapping', () => {
    const at = t0
    const cases: Array<[AlertKind, string, string]> = [
      ['test', '测试', 'info'],
      ['batch', '聚合发送', 'info'],
      ['quiet_summary', '静默摘要', 'info'],
      ['score_drop', '分数大跌', 'warning'],
      ['score_drop_skipped', '对比跳过', 'warning'],
      ['retire_pending', '待退役', 'warning'],
      ['retired', '已退役', 'info'],
      ['group_recovered', '厂商组恢复', 'success'],
    ]
    for (const [kind, text, tone] of cases) {
      const ev = event(kind, at)
      expect(eventChip(ev, pairIncidentDurations([ev]))).toEqual({ text, tone })
    }
  })

  it('treats a scopeless down (null endpoint) as point-in-time: kind word, danger tone', () => {
    const down = event('down', t0, { endpointId: null })
    const durations = pairIncidentDurations([down])
    expect(eventChip(down, durations)).toEqual({ text: '故障', tone: 'danger' })
  })
})

describe('incident duration', () => {
  const t0 = new Date(2026, 6, 31, 14, 0, 0)
  const later = (minutes: number) => new Date(t0.getTime() + minutes * 60_000)

  it('renders the paired span for a paired down', () => {
    const down = event('down', t0, { endpointId: 7 })
    const recovered = event('recovered', later(12), { endpointId: 7 })
    const durations = pairIncidentDurations([recovered, down])
    expect(incidentDurationText(down, durations, later(20))).toBe('持续 12 分 0 秒')
  })

  it('measures an unpaired down against now', () => {
    const down = event('down', t0, { endpointId: 7 })
    const durations = pairIncidentDurations([down])
    expect(incidentDurationText(down, durations, later(45))).toBe('已持续 45 分 0 秒')
  })

  it('renders the paired group span', () => {
    const down = event('group_down', t0, { groupKey: 'openai' })
    const recovered = event('group_recovered', later(65), { groupKey: 'openai' })
    const durations = pairIncidentDurations([down, recovered])
    expect(incidentDurationText(down, durations, later(70))).toBe('持续 1 小时 5 分')
  })

  it('gives the recovery event itself no duration', () => {
    const down = event('down', t0, { endpointId: 7 })
    const recovered = event('recovered', later(12), { endpointId: 7 })
    const durations = pairIncidentDurations([down, recovered])
    expect(incidentDurationText(recovered, durations, later(20))).toBe('')
  })

  it('gives point-in-time kinds no duration', () => {
    const test = event('test', t0)
    const scoreDrop = event('score_drop', t0)
    const durations = pairIncidentDurations([test, scoreDrop])
    expect(incidentDurationText(test, durations, later(5))).toBe('')
  })

  it('treats a scopeless down (null endpoint) as a non-incident card', () => {
    const down = event('down', t0, { endpointId: null })
    const durations = pairIncidentDurations([down])
    expect(incidentDurationText(down, durations, later(5))).toBe('')
  })
})

describe('selectRecentEvents', () => {
  it('takes the newest four by default (API order is newest first)', () => {
    const at = new Date(2026, 6, 31, 15, 0, 0)
    const events = Array.from({ length: 6 }, (_, i) =>
      event('test', new Date(at.getTime() - i * 60_000)),
    )
    const picked = selectRecentEvents(events)
    expect(picked).toHaveLength(RECENT_EVENTS_CARD_LIMIT)
    expect(picked.map((e) => e.id)).toEqual(events.slice(0, 4).map((e) => e.id))
  })

  it('returns everything when fewer than the limit exist', () => {
    const events = [event('test', new Date(2026, 6, 31, 15, 0))]
    expect(selectRecentEvents(events)).toHaveLength(1)
  })
})
