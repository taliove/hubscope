// Monotone cubic smoothing for evidence charts (GH #114, spec 0018 「图表
// 纪律」: smooth = monotone interpolation — never invents extrema, never
// crosses data points). ECharts' built-in `smooth` is a cardinal spline that
// CAN overshoot around sharp peaks (a latency spike's neighbors pull the
// curve above/below the measured values), so the curve is computed here and
// ECharts only draws straight segments between dense samples.
//
// Method: Fritsch–Carlson monotone cubic Hermite interpolation over the real
// (non-null) points, sampled at a fixed rate per segment. The guarantees
// that make it safe for evidence charts:
//   - Interpolation: the curve passes through every real point EXACTLY —
//     output[i * k] === values[i] for every index i, so no data point is
//     ever crossed or moved.
//   - Boundedness: within one segment every sample stays between the two
//     endpoint values — no invented peaks, no overshoot; the global min/max
//     of the output never exceed the data's min/max.
//   - Monotonicity: on locally monotone data each segment is monotone
//     (Fritsch–Carlson 1980 limiting), so smoothing cannot forge a shape
//     the data does not have; a plateau stays flat instead of rippling.
//   - Null breaks: nulls are preserved as gaps — the segment spanning a
//     null emits null interior slots, matching connectNulls:false; no
//     bridge is invented across a measurement hole (GH #56 discipline).
//
// The x coordinate is the array index (category axes are uniform), so
// tangents are expressed in index units with segment width 1.

// Samples emitted per segment. 8 gives a visually continuous curve at
// typical chart widths while keeping the slot math cheap; the category
// axis, the series data and the tooltip snapping all share this constant.
export const SMOOTH_SAMPLES_PER_SEGMENT = 8

// Cubic Hermite basis at t ∈ [0, 1] for a segment of width 1 with endpoint
// values y0/y1 and tangents m0/m1 (dy per index unit).
function hermite(t: number, y0: number, y1: number, m0: number, m1: number): number {
  const t2 = t * t
  const t3 = t2 * t
  return (
    (2 * t3 - 3 * t2 + 1) * y0 +
    (t3 - 2 * t2 + t) * m0 +
    (-2 * t3 + 3 * t2) * y1 +
    (t3 - t2) * m1
  )
}

// Fritsch–Carlson tangents for one run of consecutive real points (uniform
// spacing, x = position inside the run). The limiting pass is what makes
// every segment provably monotone: after it, a segment's Hermite cubic
// cannot leave the corridor between its endpoint values.
function monotoneTangents(ys: number[]): number[] {
  const n = ys.length
  const d: number[] = []
  for (let i = 0; i < n - 1; i++) d.push(ys[i + 1] - ys[i])
  const m = new Array<number>(n).fill(0)
  m[0] = d[0]
  m[n - 1] = d[n - 2]
  for (let i = 1; i < n - 1; i++) {
    // A local extremum (secant sign change) gets a zero tangent — the curve
    // turns exactly AT the measured point instead of overshooting past it.
    m[i] = d[i - 1] * d[i] <= 0 ? 0 : (d[i - 1] + d[i]) / 2
  }
  for (let i = 0; i < n - 1; i++) {
    if (d[i] === 0) {
      m[i] = 0
      m[i + 1] = 0
      continue
    }
    const a = m[i] / d[i]
    const b = m[i + 1] / d[i]
    const s = a * a + b * b
    if (s > 9) {
      const tau = 3 / Math.sqrt(s)
      m[i] = tau * a * d[i]
      m[i + 1] = tau * b * d[i]
    }
  }
  return m
}

// Expands a real-point series into a densely sampled, monotonically smoothed
// series. The output has (n-1)*k + 1 slots and stays index-aligned with the
// input: slot i*k holds values[i] verbatim (including nulls). Runs shorter
// than two points have no segment to smooth and pass through as-is.
export function smoothSeries(
  values: (number | null)[],
  samplesPerSegment: number = SMOOTH_SAMPLES_PER_SEGMENT
): (number | null)[] {
  const n = values.length
  if (n === 0) return []
  const k = Math.max(1, Math.round(samplesPerSegment))
  if (n === 1 || k === 1) return [...values]

  const out: (number | null)[] = new Array((n - 1) * k + 1).fill(null)
  let i = 0
  while (i < n) {
    if (values[i] === null) {
      i++
      continue
    }
    // One maximal run of consecutive real points [i, j).
    let j = i + 1
    while (j < n && values[j] !== null) j++
    const run = values.slice(i, j) as number[]
    out[i * k] = run[0]
    if (run.length === 1) {
      i = j
      continue
    }
    const m = monotoneTangents(run)
    for (let seg = 0; seg < run.length - 1; seg++) {
      for (let s = 0; s < k; s++) {
        out[(i + seg) * k + s] = hermite(s / k, run[seg], run[seg + 1], m[seg], m[seg + 1])
      }
      out[(i + seg + 1) * k] = run[seg + 1]
    }
    i = j
  }
  return out
}

// Category-axis companion of smoothSeries: the axis gets one slot per
// sample, real labels at their original positions and empty strings for the
// interpolated slots (so tick labels only ever mark measured points).
export function expandCategories(
  categories: string[],
  samplesPerSegment: number = SMOOTH_SAMPLES_PER_SEGMENT
): string[] {
  const n = categories.length
  if (n === 0) return []
  const k = Math.max(1, Math.round(samplesPerSegment))
  const out: string[] = new Array((n - 1) * k + 1).fill('')
  for (let i = 0; i < n; i++) out[i * k] = categories[i]
  return out
}

// Tooltip companion: an axis hover can land on an interpolated slot; snap to
// the nearest REAL index so the tooltip always reports true measured values
// (fidelity discipline — an interpolated mid-segment value must never be
// presented as a measurement).
export function nearestRealIndex(
  slot: number,
  realCount: number,
  samplesPerSegment: number = SMOOTH_SAMPLES_PER_SEGMENT
): number {
  const k = Math.max(1, Math.round(samplesPerSegment))
  return Math.min(realCount - 1, Math.max(0, Math.round(slot / k)))
}
