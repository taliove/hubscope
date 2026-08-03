// Bars variant geometry for TrendSparkline (GH #137): the request-volume
// widget reads as a column chart, not a curve. Pure geometry so the
// component only renders; normalization is unit-tested here.
//
// Discipline (mirrors the dots-strip family language):
//   - Equal-width bars with a fixed gap (2px, the family constant).
//   - Heights are bucket-normalized: the max bucket takes the full track
//     height — no invented baseline beyond the data.
//   - An empty or all-zero series reports `null` so the host renders the
//     1px placeholder track (same empty-state discipline as the line
//     variant) — a zero reading must never read as data.
//   - Null buckets keep their slot empty (no bar, no bridging).

export interface SparkBar {
  x: number
  y: number
  width: number
  height: number
}

// Lays out one bar per bucket inside a width × height track. Returns `null`
// when there is nothing to draw (empty input or every real value ≤ 0) so the
// caller falls back to the placeholder track.
export function sparklineBarLayout(
  values: (number | null)[],
  width: number,
  height: number,
  gap = 2,
): SparkBar[] | null {
  const n = values.length
  if (n === 0) return null
  const real = values.filter((v): v is number => v !== null)
  const max = real.length === 0 ? 0 : Math.max(...real)
  if (max <= 0) return null
  const slot = (width - (n - 1) * gap) / n
  if (slot <= 0) return null
  const bars: SparkBar[] = []
  values.forEach((v, i) => {
    if (v === null || v <= 0) return
    const h = (v / max) * height
    bars.push({ x: i * (slot + gap), y: height - h, width: slot, height: h })
  })
  return bars
}

// SVG path for a bar with rounded TOP corners only (radius-xs language):
// the bottom edge stays square so columns sit flush on the track. The
// radius clamps to half the bar width and the bar height so short bars and
// narrow slots never self-intersect; a degenerate clamp falls back to a
// plain rectangle.
export function topRoundedBarPath(
  x: number,
  y: number,
  width: number,
  height: number,
  radius: number,
): string {
  const r = Math.max(0, Math.min(radius, width / 2, height))
  if (r === 0) {
    return `M ${x} ${y} H ${x + width} V ${y + height} H ${x} Z`
  }
  return (
    `M ${x} ${y + height} ` +
    `V ${y + r} Q ${x} ${y} ${x + r} ${y} ` +
    `H ${x + width - r} Q ${x + width} ${y} ${x + width} ${y + r} ` +
    `V ${y + height} Z`
  )
}
