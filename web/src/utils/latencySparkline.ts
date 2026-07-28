// Geometry for the EndpointCard latency sparkline (design review final form).
// Pure functions only — LatencySparkline.vue stays presentational.
//
// The x axis MUST align with the 24h dots strip above it: both are a 24-slot
// flex row with a 2px gap, so a bucket's center is derived from the same
// slot arithmetic the dots CSS layout produces. SPARKLINE_GAP_PX is the single
// shared constant — keep it in sync with the `.dots-strip` / `.hover-overlay`
// `gap: 2px` in EndpointCard.vue and LatencySparkline.vue (comments cross-
// reference this constant).

export const SPARKLINE_BUCKETS = 24
export const SPARKLINE_GAP_PX = 2 // see module header — the dots CSS gap twin

// LATENCY_DEGRADE_FACTOR mirrors the status machine's latency degradation
// threshold (internal/status/status.go:54, latencyDegradeFactor = 2.0): an
// endpoint degrades when its 24h P95 exceeds 2x the 7-day P50 baseline, so
// the sparkline's dashed threshold line sits at 2x baseline — never 1x.
export const LATENCY_DEGRADE_FACTOR = 2

// MIN_Y_RANGE_MS is the floor of the y scale (design ruling): the range is
// data-driven (peak x 1.25) but never narrower than 1s, otherwise sub-second
// jitter on a healthy endpoint would stretch into a fake "shape" that reads
// as instability. The curve's semantics come from the threshold line, not
// from how full the strip looks.
export const MIN_Y_RANGE_MS = 1000

export interface SparklinePoint {
  x: number
  y: number
}

export interface SparklineSegment {
  points: SparklinePoint[]
  // A lone non-null bucket (both neighbors null, or a window edge) has no
  // line to draw; the component renders it as a small dot instead.
  isolated: boolean
  // Closed SVG path filling the area under the polyline down to the strip
  // bottom; null for isolated single-point segments (dot only, no fill).
  areaPath: string | null
}

// Width of one slot given the measured pixel width of the strip.
export function bucketSlotWidth(stripWidth: number): number {
  return (stripWidth - (SPARKLINE_BUCKETS - 1) * SPARKLINE_GAP_PX) / SPARKLINE_BUCKETS
}

// Center x of bucket i — the same position the dots strip's flex layout puts
// the i-th slot's midpoint at.
export function bucketCenterX(index: number, stripWidth: number): number {
  const slot = bucketSlotWidth(stripWidth)
  return index * (slot + SPARKLINE_GAP_PX) + slot / 2
}

// The latency value of the dashed threshold line: 2x the baseline, the same
// factor the status machine degrades on (see LATENCY_DEGRADE_FACTOR).
export function latencyThresholdMs(baselineMs: number): number {
  return baselineMs * LATENCY_DEGRADE_FACTOR
}

// Upper bound of the y scale (design ruling): data-driven — peak bucket
// value with 25% headroom, floored at MIN_Y_RANGE_MS so sub-second jitter
// never inflates into a fake shape. The 2x baseline threshold does NOT
// participate in the range; it appears on demand (see thresholdVisible).
export function sparklineYMax(values: (number | null)[]): number {
  let peak = 0
  for (const v of values) {
    if (v !== null && v > peak) peak = v
  }
  return Math.max(peak * 1.25, MIN_Y_RANGE_MS)
}

// The threshold line is rendered exactly when its 2x value fits the range
// (equivalently the peak reaches 0.8x the threshold). When it does not fit
// there is zero residual indication — the tooltip's baseline value carries
// the information instead. No baseline, no line; no hysteresis.
export function thresholdVisible(baselineMs: number | null, yMax: number): boolean {
  return baselineMs !== null && latencyThresholdMs(baselineMs) <= yMax
}

// Map a latency onto the strip height (0 at the bottom). yMax <= 0 only
// happens when there is nothing to draw; the height fallback is inert.
export function latencyY(value: number, height: number, yMax: number): number {
  if (yMax <= 0) return height
  return height - (value / yMax) * height
}

// Y coordinate of the dashed threshold line (2x baseline).
export function thresholdY(baselineMs: number, height: number, yMax: number): number {
  return latencyY(latencyThresholdMs(baselineMs), height, yMax)
}

// Closed SVG path filling the area under a polyline down to the strip
// bottom: along the line, drop at the last point, run the bottom edge back
// to the first point, close. Requires at least two points.
export function buildLatencyAreaPath(points: SparklinePoint[], height: number): string {
  const first = points[0]
  const last = points[points.length - 1]
  const line = points.map(p => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' L ')
  return `M ${line} L ${last.x.toFixed(2)},${height} L ${first.x.toFixed(2)},${height} Z`
}

// Split the 24 bucket values into polyline segments, breaking the line (and
// the area fill with it) at null buckets. Segments of a single point are
// flagged isolated so the component renders a dot instead of an invisible
// one-point line, and carry no area path.
export function buildLatencySegments(
  values: (number | null)[],
  stripWidth: number,
  height: number,
  yMax: number,
): SparklineSegment[] {
  const segments: SparklineSegment[] = []
  let current: SparklinePoint[] = []
  const flush = () => {
    if (current.length === 0) return
    const isolated = current.length === 1
    segments.push({
      points: current,
      isolated,
      areaPath: isolated ? null : buildLatencyAreaPath(current, height),
    })
    current = []
  }
  values.forEach((value, i) => {
    if (value === null) {
      flush()
      return
    }
    current.push({ x: bucketCenterX(i, stripWidth), y: latencyY(value, height, yMax) })
  })
  flush()
  return segments
}
