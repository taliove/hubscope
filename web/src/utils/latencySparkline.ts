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

export interface SparklinePoint {
  x: number
  y: number
}

export interface SparklineSegment {
  points: SparklinePoint[]
  // A lone non-null bucket (both neighbors null, or a window edge) has no
  // line to draw; the component renders it as a small dot instead.
  isolated: boolean
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

// Upper bound of the y scale: max(2x baseline threshold, peak bucket value)
// with 10% headroom so the curve and the threshold line never touch the top
// edge.
export function sparklineYMax(values: (number | null)[], baselineMs: number | null): number {
  let max = baselineMs !== null ? latencyThresholdMs(baselineMs) : 0
  for (const v of values) {
    if (v !== null && v > max) max = v
  }
  return max * 1.1
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

// Split the 24 bucket values into polyline segments, breaking the line at
// null buckets. Segments of a single point are flagged isolated so the
// component can render a dot instead of an invisible one-point line.
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
    segments.push({ points: current, isolated: current.length === 1 })
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
