// Visibility-aware polling (spec 0015 decision 5, ui-guidelines §6): a
// setInterval wrapper that reacts to tab visibility. While hidden the poll
// either throttles to a slower cadence (hiddenIntervalMs > 0) or pauses
// entirely (hiddenIntervalMs omitted/0); returning to the foreground fires
// one immediate tick and then resumes the normal cadence. All page polling
// must go through this wrapper — no ad-hoc visibilitychange listeners.
//
// The handle is deliberately a plain timer (not a composable): consumers
// with re-arm semantics (batch polls arm only while a batch is unfinished)
// swap their setInterval/clearInterval pair for create/handle.clear() and
// keep their arming logic untouched. Unmount cleanup is unchanged: call
// handle.clear() where clearInterval used to run.
export interface VisibilityPollOptions {
  // Cadence while the tab is visible, in milliseconds.
  intervalMs: number
  // Cadence while hidden; omitted or 0 pauses polling entirely.
  hiddenIntervalMs?: number
  // Fire one immediate tick when the tab returns to the foreground
  // (default true).
  refreshOnVisible?: boolean
}

export interface VisibilityPollHandle {
  // Idempotent: stops the timer and removes the visibility listener.
  clear(): void
}

export function createVisibilityPoll(tick: () => void, options: VisibilityPollOptions): VisibilityPollHandle {
  const { intervalMs, hiddenIntervalMs = 0, refreshOnVisible = true } = options
  let timer: ReturnType<typeof setInterval> | undefined
  let cleared = false

  function disarm() {
    clearInterval(timer)
    timer = undefined
  }

  // Arm at the cadence matching current visibility; a hidden cadence of 0
  // means no timer at all (pause).
  function arm() {
    disarm()
    const ms = document.hidden ? hiddenIntervalMs : intervalMs
    if (ms > 0) {
      timer = setInterval(tick, ms)
    }
  }

  function onVisibilityChange() {
    if (cleared) return
    if (document.hidden) {
      // Switch to the hidden cadence (or pause).
      arm()
      return
    }
    disarm()
    if (refreshOnVisible) tick()
    // The tick may have synchronously retired this handle (a consumer whose
    // re-arm clears and recreates); never resurrect a cleared handle.
    if (!cleared) arm()
  }

  document.addEventListener('visibilitychange', onVisibilityChange)
  arm()

  return {
    clear() {
      if (cleared) return
      cleared = true
      disarm()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    },
  }
}
