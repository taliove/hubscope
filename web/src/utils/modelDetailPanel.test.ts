// Unit tests for the model detail panel derivations (GH #116, spec 0018
// §10). The load-bearing invariants: the error rate is the exact complement
// of availability (no second caliber), null never becomes an invented 0%,
// and the event list is newest-first, capped, and never mutates its input.
import { describe, it, expect } from 'vitest'
import type { EndpointStatus, OverviewEntry, ProbeRecord } from '@/api/types'
import {
  FAILURE_EVENT_LIMIT,
  panelMetrics,
  recentFailureEvents,
} from '@/utils/modelDetailPanel'

function makeEntry(overrides: Partial<OverviewEntry> = {}): OverviewEntry {
  return {
    endpoint_id: 1,
    model_id: 'm',
    protocol: 'openai',
    enabled: true,
    status: 'healthy' as EndpointStatus,
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

function makeProbe(id: number, ok: boolean, createdAt: string, errorSummary: string | null = null): ProbeRecord {
  return {
    id,
    endpoint_id: 1,
    streaming: id % 2 === 0,
    ok,
    http_status: ok ? 200 : 500,
    error_summary: errorSummary,
    latency_ms: 100,
    ttft_ms: null,
    input_tokens: null,
    output_tokens: null,
    created_at: createdAt,
  }
}

describe('panelMetrics', () => {
  it('derives the error rate as the exact complement of availability', () => {
    const m = panelMetrics(makeEntry({ success_rate_24h: 0.975, p50_ms: 320, p95_ms: 810 }))
    expect(m.availability).toBe(0.975)
    expect(m.errorRate).toBeCloseTo(0.025, 10)
    expect(m.latencyP50Ms).toBe(320)
    expect(m.latencyP95Ms).toBe(810)
  })

  it('keeps null null — a no-probe window never invents a 0% error rate', () => {
    const m = panelMetrics(makeEntry({ success_rate_24h: null }))
    expect(m.availability).toBeNull()
    expect(m.errorRate).toBeNull()
  })

  it('a fully failed window yields a 100% error rate', () => {
    const m = panelMetrics(makeEntry({ success_rate_24h: 0 }))
    expect(m.errorRate).toBe(1)
  })
})

describe('recentFailureEvents', () => {
  it('filters successes, sorts newest first, and caps at the limit', () => {
    const records = [
      makeProbe(1, false, '2026-07-31T08:00:00Z', 'timeout'),
      makeProbe(2, true, '2026-07-31T09:00:00Z'),
      makeProbe(3, false, '2026-07-31T10:00:00Z', 'http 502'),
    ]
    const events = recentFailureEvents(records)
    expect(events.map(e => e.id)).toEqual([3, 1])
    expect(events[0].reason).toBe('http 502')
    expect(events[0].streaming).toBe(false) // id 3 is odd → non-streaming fixture
  })

  it('breaks timestamp ties by id desc', () => {
    const records = [
      makeProbe(1, false, '2026-07-31T08:00:00Z', 'a'),
      makeProbe(2, false, '2026-07-31T08:00:00Z', 'b'),
    ]
    expect(recentFailureEvents(records).map(e => e.id)).toEqual([2, 1])
  })

  it('caps the list at FAILURE_EVENT_LIMIT by default', () => {
    const records = Array.from({ length: FAILURE_EVENT_LIMIT + 5 }, (_, i) =>
      makeProbe(i + 1, false, `2026-07-31T${String(i).padStart(2, '0')}:00:00Z`, 'x'),
    )
    const events = recentFailureEvents(records)
    expect(events).toHaveLength(FAILURE_EVENT_LIMIT)
    expect(events[0].id).toBe(FAILURE_EVENT_LIMIT + 5) // newest survives the cap
  })

  it('honors an explicit limit', () => {
    const records = [
      makeProbe(1, false, '2026-07-31T08:00:00Z', 'a'),
      makeProbe(2, false, '2026-07-31T09:00:00Z', 'b'),
    ]
    expect(recentFailureEvents(records, 1)).toHaveLength(1)
  })

  it('falls back to a neutral word for null or blank error summaries', () => {
    const events = recentFailureEvents([
      makeProbe(1, false, '2026-07-31T08:00:00Z', null),
      makeProbe(2, false, '2026-07-31T09:00:00Z', '   '),
    ])
    expect(events.map(e => e.reason)).toEqual(['未知错误', '未知错误'])
  })

  it('does not mutate the input array', () => {
    const records = [
      makeProbe(1, false, '2026-07-31T08:00:00Z', 'a'),
      makeProbe(2, false, '2026-07-31T09:00:00Z', 'b'),
    ]
    const before = records.map(r => r.id)
    recentFailureEvents(records)
    expect(records.map(r => r.id)).toEqual(before)
  })

  it('returns an empty list when nothing failed', () => {
    expect(recentFailureEvents([makeProbe(1, true, '2026-07-31T08:00:00Z')])).toEqual([])
  })
})
