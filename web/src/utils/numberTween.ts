// Number tween engine (spec 0018 动效体系, GH #115): core numbers — the
// health-index hero figure and the metric-widget values — glide through a
// 500–800ms tween on poll updates instead of jumping. Per the
// Gate-The-Phases discipline the JS-side timing is gated separately from
// the CSS `transition: none` blanket: the reduced-motion check runs once
// per tween (chartMotion.ts precedent) and renders the terminal value
// immediately.
//
// The engine is driver-injectable (now / raf / cancelRaf) so tests run
// without a real frame clock; production callers use the defaults.
import { prefersReducedMotion } from '@/utils/chartMotion'

// 600ms sits in the middle of the spec's 500–800ms band.
export const TWEEN_DURATION_MS = 600

export interface TweenDrivers {
  now: () => number
  raf: (cb: (time: number) => void) => number
  cancelRaf: (id: number) => void
}

function defaultDrivers(): TweenDrivers {
  return {
    now: () => performance.now(),
    raf: cb => window.requestAnimationFrame(cb),
    cancelRaf: id => window.cancelAnimationFrame(id),
  }
}

// easeOutCubic: fast start, gentle settle — the number reads as "landing",
// not as linearly counting.
export function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3)
}

// tweenNumber animates onUpdate from `from` to `to` over durationMs and
// returns a cancel function. The terminal value is emitted EXACTLY (never
// an eased approximation), and a no-op tween (from === to) settles
// immediately. Under reduced motion the terminal value renders instantly
// and the returned cancel is a no-op.
export function tweenNumber(
  from: number,
  to: number,
  onUpdate: (value: number) => void,
  opts?: { durationMs?: number; reducedMotion?: boolean; drivers?: TweenDrivers },
): () => void {
  const duration = opts?.durationMs ?? TWEEN_DURATION_MS
  const reduced = opts?.reducedMotion ?? prefersReducedMotion()
  if (reduced || from === to || duration <= 0) {
    onUpdate(to)
    return () => {}
  }
  const drivers = opts?.drivers ?? defaultDrivers()
  const start = drivers.now()
  let frame: number | null = null
  let cancelled = false

  const step = () => {
    if (cancelled) return
    const t = Math.min(1, (drivers.now() - start) / duration)
    onUpdate(t >= 1 ? to : from + (to - from) * easeOutCubic(t))
    if (t < 1) frame = drivers.raf(step)
  }
  frame = drivers.raf(step)

  return () => {
    cancelled = true
    if (frame !== null) drivers.cancelRaf(frame)
  }
}
