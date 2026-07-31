// Unit tests for the quick-view latency-detail data transform (2026-07-30
// 视觉三修): the line only ever carries successful probes with null gap
// breaks (holes never get a straight line across), failures merge into
// windows instead of rug points, the median cadence interval is floored at
// MIN_INTERVAL_MS, and the y range follows the P99 tail-trim discipline
// (fallback to raw peak below P99_MIN_SAMPLES — fidelity first on small
// samples).
import { describe, it, expect } from 'vitest'
import {
  buildProbeLatencySeries,
  medianIntervalMs,
  Y_HEADROOM,
  P99_MIN_SAMPLES,
  GAP_BREAK_FACTOR,
  WINDOW_MERGE_FACTOR,
  MIN_INTERVAL_MS,
} from '@/utils/probeLatencyChart'
import { MIN_Y_RANGE_MS } from '@/utils/probeLatencyChart'
import type { ProbeRecord } from '@/api/types'

function rec(id: number, ok: boolean, latencyMs: number, createdAt: string, errorSummary: string | null = null): ProbeRecord {
  return {
    id,
    endpoint_id: 1,
    streaming: false,
    ok,
    http_status: ok ? 200 : 503,
    error_summary: errorSummary,
    latency_ms: latencyMs,
    ttft_ms: null,
    input_tokens: null,
    output_tokens: null,
    created_at: createdAt,
  }
}

const T1 = '2026-07-30T10:00:00Z'
const T2 = '2026-07-30T10:05:00Z'
const T3 = '2026-07-30T10:10:00Z'

// n successful probes, one minute apart, oldest first (id 1..n) — returned
// newest-first to match the API contract.
function recs(n: number, latencyOf: (i: number) => number): ProbeRecord[] {
  const out: ProbeRecord[] = []
  for (let i = 1; i <= n; i++) {
    const t = new Date(new Date(T1).getTime() + (i - 1) * 60000).toISOString()
    out.push(rec(i, true, latencyOf(i), t))
  }
  return out.reverse()
}

describe('medianIntervalMs', () => {
  it('returns the floor for fewer than two timestamps', () => {
    expect(medianIntervalMs([])).toBe(MIN_INTERVAL_MS)
    expect(medianIntervalMs([1000])).toBe(MIN_INTERVAL_MS)
  })

  it('computes the median of consecutive intervals over all timestamps', () => {
    // Intervals: 60s, 120s, 60s → median 60s.
    const base = new Date(T1).getTime()
    expect(medianIntervalMs([base, base + 60000, base + 180000, base + 240000])).toBe(60000)
  })

  it('floors an abnormally dense cadence at MIN_INTERVAL_MS', () => {
    const base = new Date(T1).getTime()
    expect(medianIntervalMs([base, base + 5000, base + 10000])).toBe(MIN_INTERVAL_MS)
  })
})

describe('buildProbeLatencySeries', () => {
  it('empty input yields empty series with the floor range', () => {
    const s = buildProbeLatencySeries([])
    expect(s.points).toEqual([])
    expect(s.failureWindows).toEqual([])
    expect(s.yMaxMs).toBe(MIN_Y_RANGE_MS)
  })

  it('reverses the newest-first API order into oldest-first drawing order', () => {
    // Input is DESC by created_at (the API contract).
    const s = buildProbeLatencySeries([rec(3, true, 300, T3), rec(2, true, 200, T2), rec(1, true, 100, T1)])
    expect(s.points.map(p => p.time)).toEqual([
      new Date(T1).getTime(),
      new Date(T2).getTime(),
      new Date(T3).getTime(),
    ])
    expect(s.points.map(p => p.latencyMs)).toEqual([100, 200, 300])
  })

  it('a single successful probe still gets the floored range and no windows', () => {
    const s = buildProbeLatencySeries([rec(1, true, 120, T1)])
    expect(s.points).toHaveLength(1)
    expect(s.failureWindows).toEqual([])
    expect(s.yMaxMs).toBe(MIN_Y_RANGE_MS)
  })

  it('all-failed input yields windows only; failure latencies never enter the range', () => {
    const s = buildProbeLatencySeries([
      rec(2, false, 60000, T2, 'timeout'),
      rec(1, false, 59000, T1, 'HTTP 500'),
    ])
    expect(s.points).toEqual([])
    // Two failures 5min apart: median interval (single gap) = 300s, floored
    // to... 300s > 60s floor so it stands; 300s ≤ 2×300s → one merged window.
    // Multi-failure windows are never widened: band bounds === true bounds.
    expect(s.failureWindows).toEqual([
      {
        start: new Date(T1).getTime(),
        end: new Date(T2).getTime(),
        count: 2,
        bandStart: new Date(T1).getTime(),
        bandEnd: new Date(T2).getTime(),
      },
    ])
    // 60s timeouts would blow the range if they counted — the floor wins.
    expect(s.yMaxMs).toBe(MIN_Y_RANGE_MS)
  })

  it('inserts a null break between successes whose gap exceeds 3x the median interval', () => {
    // 1min cadence with one ~11-minute hole (between +1min and +12min):
    // gaps 60,660,60,60,60 → median 60s, 660s > 3×60s.
    const input = [0, 1, 12, 13, 14, 15]
      .map((m, i) => rec(i + 1, true, 1000, new Date(new Date(T1).getTime() + m * 60000).toISOString()))
      .reverse()
    const s = buildProbeLatencySeries(input)
    const nulls = s.points.filter(p => p.latencyMs === null)
    expect(nulls).toHaveLength(1)
    // The null sits at the first post-hole timestamp, right before the real
    // point — the segment ends at the last pre-hole success and resumes here.
    const idx = s.points.findIndex(p => p.latencyMs === null)
    expect(s.points[idx + 1].time).toBe(s.points[idx].time)
    expect(s.points[idx + 1].latencyMs).toBe(1000)
    expect(s.points).toHaveLength(7)
  })

  it('does not break the line at or below 3x the median interval', () => {
    // 1min cadence with one exactly-3× hole (180s between +1min and +4min):
    // gaps 60,180,60 → median 60s; 180 = 3×60 is NOT > 3×60.
    const input = [0, 1, 4, 5]
      .map((m, i) => rec(i + 1, true, 1000, new Date(new Date(T1).getTime() + m * 60000).toISOString()))
      .reverse()
    const s = buildProbeLatencySeries(input)
    expect(s.points.every(p => p.latencyMs !== null)).toBe(true)
    expect(GAP_BREAK_FACTOR).toBe(3)
  })

  it('merges failures whose adjacent gap is within 2x the median into one window', () => {
    // Successes at 1min cadence anchor the median at 60s; three failures
    // 2min apart (120s ≤ 2×60s) merge into a single window of count 3.
    const f = (id: number, offsetMin: number) =>
      rec(id, false, 30000, new Date(new Date(T1).getTime() + offsetMin * 60000).toISOString(), 'timeout')
    const input = [...recs(10, () => 1000), f(101, 2), f(102, 4), f(103, 6)]
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    const s = buildProbeLatencySeries(input)
    expect(WINDOW_MERGE_FACTOR).toBe(2)
    expect(s.failureWindows).toEqual([
      {
        start: new Date(T1).getTime() + 2 * 60000,
        end: new Date(T1).getTime() + 6 * 60000,
        count: 3,
        bandStart: new Date(T1).getTime() + 2 * 60000,
        bandEnd: new Date(T1).getTime() + 6 * 60000,
      },
    ])
  })

  it('splits failures whose adjacent gap exceeds 2x the median into separate windows', () => {
    // 1min cadence anchor; failures at +2min and +10min — the 8min gap
    // (480s > 2×60s) splits them.
    const f = (id: number, offsetMin: number) =>
      rec(id, false, 30000, new Date(new Date(T1).getTime() + offsetMin * 60000).toISOString(), 'timeout')
    const input = [...recs(10, () => 1000), f(101, 2), f(102, 10)]
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    const s = buildProbeLatencySeries(input)
    expect(s.failureWindows).toHaveLength(2)
    expect(s.failureWindows[0].start).toBe(new Date(T1).getTime() + 2 * 60000)
    expect(s.failureWindows[1].start).toBe(new Date(T1).getTime() + 10 * 60000)
  })

  it('a lone failure keeps true bounds start === end and widens only the render band (2026-07-30 救援批第二轮)', () => {
    // Timestamps T1(fail) + T2(success): single 300s gap → median 300s.
    // The zero-width window widens to ±0.5 × 300s = ±150s for visibility;
    // start/end/count stay faithful (tooltip reads the true bounds).
    const s = buildProbeLatencySeries([
      rec(2, true, 250, T2),
      rec(1, false, 30000, T1, 'timeout'),
    ])
    const t1 = new Date(T1).getTime()
    expect(s.failureWindows).toEqual([
      { start: t1, end: t1, count: 1, bandStart: t1 - 150000, bandEnd: t1 + 150000 },
    ])
  })

  it('widens a lone-failure band by ±0.5× the floored median when the cadence anchors at 1 minute', () => {
    // 1-min success cadence anchors the median at the 60s floor; a lone
    // failure at +20min renders as a 60s-wide band centered on the true time.
    const f = rec(101, false, 30000, new Date(new Date(T1).getTime() + 20 * 60000).toISOString(), 'timeout')
    const input = [...recs(10, () => 1000), f]
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    const s = buildProbeLatencySeries(input)
    const tf = new Date(f.created_at).getTime()
    expect(s.failureWindows).toHaveLength(1)
    const w = s.failureWindows[0]
    expect([w.start, w.end, w.count]).toEqual([tf, tf, 1])
    expect(w.bandStart).toBe(tf - MIN_INTERVAL_MS / 2)
    expect(w.bandEnd).toBe(tf + MIN_INTERVAL_MS / 2)
  })

  it('never widens multi-failure windows (render band === true bounds)', () => {
    // Two failures merging into one window: the merged band already has
    // width, so bandStart/bandEnd mirror start/end exactly.
    const f = (id: number, offsetMin: number) =>
      rec(id, false, 30000, new Date(new Date(T1).getTime() + offsetMin * 60000).toISOString(), 'timeout')
    const input = [...recs(10, () => 1000), f(101, 2), f(102, 4)]
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    const s = buildProbeLatencySeries(input)
    expect(s.failureWindows).toHaveLength(1)
    const w = s.failureWindows[0]
    expect(w.count).toBe(2)
    expect(w.bandStart).toBe(w.start)
    expect(w.bandEnd).toBe(w.end)
    expect(w.end).toBeGreaterThan(w.start)
  })

  it('the area fill is a pure render layer — series points carry no injected fill points', () => {
    // The component's areaStyle fills under the line in ECharts; the data
    // transform must stay untouched — every point sits at a REAL record
    // timestamp (gap-break nulls included), none is a fill helper.
    const input = [0, 1, 12, 13, 14]
      .map((m, i) => rec(i + 1, true, 1000, new Date(new Date(T1).getTime() + m * 60000).toISOString()))
      .reverse()
    const s = buildProbeLatencySeries(input)
    const inputTimes = new Set(input.map(r => new Date(r.created_at).getTime()))
    // 5 real points + 1 gap-break null (the 11-minute hole), nothing else.
    expect(s.points).toHaveLength(6)
    expect(s.points.every(p => inputTimes.has(p.time))).toBe(true)
    expect(s.points.filter(p => p.latencyMs !== null)).toHaveLength(5)
  })

  it('below P99_MIN_SAMPLES the range falls back to the raw peak (spike included)', () => {
    // 10 samples (< 20): a 60s spike must still drive the range — small
    // samples have no trimming basis, fidelity first.
    const s = buildProbeLatencySeries(recs(10, i => (i === 5 ? 60000 : 1000)))
    expect(s.points.filter(p => p.latencyMs !== null)).toHaveLength(10)
    expect(10).toBeLessThan(P99_MIN_SAMPLES)
    expect(s.yMaxMs).toBe(60000 * Y_HEADROOM)
  })

  it('at >= P99_MIN_SAMPLES a lone spike trims out of the range (natural clip, not clamp)', () => {
    // 100 samples: 99 x 1000ms + one 60s spike. Nearest-rank P99 = sorted[98]
    // = 1000, so the range ignores the spike; the point itself stays in the
    // data at full value (the chart clips it visually, tooltips stay true).
    const s = buildProbeLatencySeries(recs(100, i => (i === 50 ? 60000 : 1000)))
    expect(s.yMaxMs).toBe(1000 * Y_HEADROOM)
    expect(s.points.some(p => p.latencyMs === 60000)).toBe(true)
  })

  it('regular case: range is P99 x headroom once it clears the floor', () => {
    // 30 samples ramping 100..3000 by 100: sorted[ceil(0.99*30)-1] =
    // sorted[29] = 3000 (the max is the P99 rank at this size).
    const s = buildProbeLatencySeries(recs(30, i => i * 100))
    expect(s.yMaxMs).toBe(3000 * Y_HEADROOM)
  })

  it('mixed input splits successes to the line and failures to windows, both chronological', () => {
    const s = buildProbeLatencySeries([
      rec(3, false, 60000, T3, 'timeout'),
      rec(2, true, 250, T2),
      rec(1, true, 100, T1),
    ])
    expect(s.points.map(p => p.latencyMs)).toEqual([100, 250])
    expect(s.failureWindows.map(w => w.start)).toEqual([new Date(T3).getTime()])
    expect(s.yMaxMs).toBe(MIN_Y_RANGE_MS)
  })
})
