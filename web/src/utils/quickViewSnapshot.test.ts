// Unit tests for the quick-view snapshot freeze: the frozen entry must be a
// deep-enough copy that mutating the polled source object (or its nested
// arrays) afterwards never changes what the open dialog renders.
import { describe, it, expect } from 'vitest'
import { freezeEntrySnapshot } from '@/utils/quickViewSnapshot'
import type { OverviewEntry } from '@/api/types'

function makeEntry(): OverviewEntry {
  return {
    endpoint_id: 7,
    model_id: 'gpt-5.2',
    protocol: 'openai',
    enabled: true,
    status: 'degraded',
    status_reason: '24h 成功率低于 95%',
    degrade_causes: ['availability'],
    success_rate_24h: 0.92,
    p50_ms: 820,
    p95_ms: 2100,
    last_probe_at: '2026-07-29T08:00:00Z',
    family: 'gpt',
    capability: 'chat',
    score: 85,
    score_reasons: ['扣分项 A'],
    dots_24h: [
      { bucket_start: '2026-07-29T07:00:00Z', total: 10, failures: 1, p50_ms: 800 },
      { bucket_start: '2026-07-29T08:00:00Z', total: 12, failures: 2, p50_ms: 840 },
    ],
    eval_score: null,
    baseline_p50_ms: 700,
  }
}

describe('freezeEntrySnapshot', () => {
  it('copies scalar fields verbatim', () => {
    const entry = makeEntry()
    const frozen = freezeEntrySnapshot(entry)
    expect(frozen).toEqual(entry)
    expect(frozen).not.toBe(entry)
  })

  it('detaches the nested arrays so later source mutations stay invisible', () => {
    const entry = makeEntry()
    const frozen = freezeEntrySnapshot(entry)

    // Simulate a poll cycle mutating the source in place.
    entry.status = 'down'
    entry.dots_24h[0].failures = 10
    entry.dots_24h[0].p50_ms = null
    entry.dots_24h.push({ bucket_start: '2026-07-29T09:00:00Z', total: 5, failures: 5, p50_ms: null })
    entry.degrade_causes.push('latency')
    entry.score_reasons.push('扣分项 B')

    expect(frozen.status).toBe('degraded')
    expect(frozen.dots_24h).toHaveLength(2)
    expect(frozen.dots_24h[0]).toEqual({
      bucket_start: '2026-07-29T07:00:00Z',
      total: 10,
      failures: 1,
      p50_ms: 800,
    })
    expect(frozen.degrade_causes).toEqual(['availability'])
    expect(frozen.score_reasons).toEqual(['扣分项 A'])
  })
})
