// Chart entry-animation gate (GH #114, spec 0018 动效体系): every chart
// draws in once, left to right, over 800–1200ms; reduced-motion zeroes the
// animation. Per the Gate-The-Phases discipline the JS-side timing is gated
// separately from the CSS `transition: none` blanket — the matchMedia check
// runs once (lazy, cached), not per render.

// 1000ms sits in the middle of the spec's 800–1200ms band.
export const CHART_ANIMATION_DURATION_MS = 1000

let cached: boolean | null = null

// True when the user prefers reduced motion (or the preference cannot be
// read — animation is the default only when the query is answerable).
export function prefersReducedMotion(): boolean {
  if (cached === null) {
    cached =
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
  }
  return cached
}

// ECharts `animation` flag for chart entry draws: off under reduced motion,
// on otherwise.
export function chartAnimationEnabled(): boolean {
  return !prefersReducedMotion()
}
