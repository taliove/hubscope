// Data transform for the 24h per-probe latency detail curve in the endpoint
// quick-view dialog (2026-07-30, dashboard surface brief 「24h 延迟明细」区;
// semantics in ui-guidelines §5 EndpointQuickViewDialog 条; 2026-07-30 视觉三修
// — pure line / gap breaks / failure windows, user verdict 「毛毛虫圆点太丑」).
//
// Semantics that must hold here (the component only renders):
//   - Success probes connect as a bare line (latency_ms, no point symbols).
//     Failed probes NEVER enter the y range — a failed probe's latency is a
//     time-to-failure (same discipline as the sparkline's "failed probes
//     don't pollute the curve"); failures render as full-height danger
//     markArea windows instead of rug points.
//   - The x axis is a TIME axis — probe intervals are not constant (outages,
//     restarts leave holes); a category axis would present uneven spacing as
//     even, forging the evidence shape (anti-faking at the chart layer).
//   - Gap breaks: a straight line across a hole would forge continuity that
//     was never measured. Between two consecutive SUCCESSFUL probes whose
//     gap exceeds GAP_BREAK_FACTOR × the median interval, a null point is
//     inserted (ECharts connectNulls:false breaks the line).
//   - Failure windows: consecutive failures whose adjacent gap is within
//     WINDOW_MERGE_FACTOR × the median interval merge into one window
//     [{start, end, count}] — a merged band reads as one incident; a lone
//     failure is a start === end window whose RENDER bounds (bandStart/
//     bandEnd) widen to a minimum visible width without touching the true
//     bounds (2026-07-30 救援批第二轮).
//   - The median interval is computed over ALL record timestamps (success +
//     failure — the probe cadence is endpoint-level) and floored at
//     MIN_INTERVAL_MS so an abnormally dense burst cannot shrink the
//     reference and turn ordinary spacing into "gaps".
//   - y range is data-driven with P99 tail-trimming (2026-07-30 二次修订,
//     replacing peak ×1.25 — a 60s+ timeout spike pushed the peak range so
//     high the curve hugged the floor, unreadable): yMax = max(P99 ×1.25,
//     MIN_Y_RANGE_MS floor), successes only. Points beyond the range clip
//     naturally at the axis edge (ECharts clip) — NEVER clamp the data
//     itself (clamping reads a spike as "exactly at the ceiling", a shape
//     forgery); tooltips always carry the true value.
//   - Same principle, different parameters (ui-guidelines §5: 同原则不同参数
//     不算第二口径): utils/latencySparkline.ts uses peak ×1.25 because its
//     hourly P50 buckets already shave spikes; this per-point curve has no
//     such buffer, hence the P99 trim. The MIN_Y_RANGE_MS floor is shared
//     with the sparkline (same value, imported) — keep the two files'
//     range-discipline comments cross-referencing each other.
import type { ProbeRecord } from '@/api/types'
import { MIN_Y_RANGE_MS } from './latencySparkline'

export interface LatencyPoint {
  time: number // ms epoch, for the ECharts time axis
  // null = inserted gap break (the line stops here, connectNulls:false);
  // tooltip wording for null points lives in the component.
  latencyMs: number | null
}

export interface FailureWindow {
  // True incident bounds — the tooltip's 「HH:mm–HH:mm · 失败 N 次」 reads
  // these and they are NEVER widened (fidelity first).
  start: number // ms epoch, inclusive
  end: number // ms epoch; start === end for a lone failure
  count: number
  // markArea render bounds (2026-07-30 救援批第二轮): a lone failure is a
  // zero-width band — invisible at any zoom (user screenshot verdict: a
  // 26.9%-success endpoint was littered with failures yet showed no band).
  // Lone windows (start === end) widen to ±0.5 × the median interval purely
  // for presentation — the minimum visible width is a render-layer floor,
  // it does NOT rewrite the incident: count/start/end stay faithful.
  // Multi-failure windows already have width and are never widened
  // (bandStart === start, bandEnd === end).
  bandStart: number
  bandEnd: number
}

export interface ProbeLatencySeries {
  // Chronological order (oldest → newest); the API returns newest-first.
  points: LatencyPoint[]
  failureWindows: FailureWindow[]
  yMaxMs: number
}

// Headroom above the P99 reference latency — the sparkline's ×1.25 factor,
// restated here so the two curves share the range discipline by construction.
export const Y_HEADROOM = 1.25

// Below this many successful samples the P99 trim has no statistical basis —
// the range falls back to the raw peak (fidelity first on small samples).
export const P99_MIN_SAMPLES = 20

// Gap/window factors over the median interval (2026-07-30 视觉三修, single-
// sourced here): > 3× breaks the success line; ≤ 2× merges adjacent failures
// into one window.
export const GAP_BREAK_FACTOR = 3
export const WINDOW_MERGE_FACTOR = 2

// Median-interval floor: an abnormally dense burst must not shrink the
// cadence reference below one minute (probe cadence is minute-scale).
export const MIN_INTERVAL_MS = 60_000

// Lower median of the consecutive intervals between ALL record timestamps
// (chronological), floored at MIN_INTERVAL_MS. Returns MIN_INTERVAL_MS when
// fewer than two timestamps exist (no interval is measurable — and with at
// most one point there is nothing to break or merge).
export function medianIntervalMs(times: number[]): number {
  if (times.length < 2) return MIN_INTERVAL_MS
  const sorted = [...times].sort((a, b) => a - b)
  const gaps: number[] = []
  for (let i = 1; i < sorted.length; i++) gaps.push(sorted[i] - sorted[i - 1])
  gaps.sort((a, b) => a - b)
  return Math.max(gaps[Math.floor((gaps.length - 1) / 2)], MIN_INTERVAL_MS)
}

// Nearest-rank P99 over the successful latencies; falls back to the peak
// when the sample count is below P99_MIN_SAMPLES.
function rangeReferenceMs(latencies: number[]): number {
  if (latencies.length === 0) return 0
  const sorted = [...latencies].sort((a, b) => a - b)
  if (sorted.length < P99_MIN_SAMPLES) return sorted[sorted.length - 1]
  return sorted[Math.ceil(0.99 * sorted.length) - 1]
}

export function buildProbeLatencySeries(records: ProbeRecord[]): ProbeLatencySeries {
  // Interface is newest-first (api-contract); drawing is oldest-first.
  const chronological = [...records].reverse()
  const median = medianIntervalMs(chronological.map(r => new Date(r.created_at).getTime()))

  // Success line with gap breaks: the null sits at the CURRENT point's
  // timestamp, right before the real point — the segment ends at the
  // previous success and resumes at this one (no straight line across a
  // measurement hole).
  const points: LatencyPoint[] = []
  let prevSuccessTime: number | null = null
  for (const r of chronological) {
    if (!r.ok) continue
    const time = new Date(r.created_at).getTime()
    if (prevSuccessTime !== null && time - prevSuccessTime > GAP_BREAK_FACTOR * median) {
      points.push({ time, latencyMs: null })
    }
    points.push({ time, latencyMs: r.latency_ms })
    prevSuccessTime = time
  }

  // Failure windows: merge while the adjacent failure gap stays within
  // WINDOW_MERGE_FACTOR × median; a wider gap splits (two incidents, not
  // one). A lone failure keeps start === end (zero-width) — the render
  // bounds are widened below, the true bounds never are.
  const failureWindows: FailureWindow[] = []
  for (const r of chronological) {
    if (r.ok) continue
    const time = new Date(r.created_at).getTime()
    const last = failureWindows[failureWindows.length - 1]
    if (last && time - last.end <= WINDOW_MERGE_FACTOR * median) {
      last.end = time
      last.bandEnd = time
      last.count += 1
    } else {
      failureWindows.push({ start: time, end: time, count: 1, bandStart: time, bandEnd: time })
    }
  }
  // Minimum visible width (see FailureWindow): only lone zero-width windows
  // widen, by half a median interval on each side.
  for (const w of failureWindows) {
    if (w.start === w.end) {
      w.bandStart = w.start - median / 2
      w.bandEnd = w.end + median / 2
    }
  }

  const ref = rangeReferenceMs(points.flatMap(p => (p.latencyMs === null ? [] : [p.latencyMs])))
  return { points, failureWindows, yMaxMs: Math.max(ref * Y_HEADROOM, MIN_Y_RANGE_MS) }
}
