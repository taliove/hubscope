// Unified responsive breakpoint mechanism (GH #94): a single 768px breakpoint
// consumed by the share-materials family (dialogs, Leaderboard,
// EvalProgressGrid, /report/:token page). The state is reactive and updates on
// viewport resize; every consumer mounting this composable shares the same
// MediaQueryList listener, so the overhead stays constant even with multiple
// consuming components on screen.封闭清单 = StatusShareDialog /
// EvalShareDialog / Leaderboard / EvalProgressGrid / CampaignReportView (shared
// mode); the dashboard and admin console never consume this breakpoint (the
// desktop-first declaration does not invalidate — ui-guidelines §5).
import { ref, onMounted, onUnmounted, readonly } from 'vue'

// The single breakpoint value for the whole batch: 767px max-width means
// narrow viewports (<=767px) get the responsive treatment, and desktop
// (>=768px) is pixel-perfect unchanged.
const BREAKPOINT_QUERY = '(max-width: 767px)'

let mediaQuery: MediaQueryList | null = null
let listenerCount = 0

// Reactive state shared across all consumers of this composable.
const isNarrow = ref(false)

function updateNarrowState() {
  if (mediaQuery) {
    isNarrow.value = mediaQuery.matches
  }
}

function addListener() {
  if (listenerCount === 0) {
    mediaQuery = window.matchMedia(BREAKPOINT_QUERY)
    updateNarrowState()
    mediaQuery.addEventListener('change', updateNarrowState)
  }
  listenerCount += 1
}

function removeListener() {
  listenerCount -= 1
  if (listenerCount === 0 && mediaQuery) {
    mediaQuery.removeEventListener('change', updateNarrowState)
    mediaQuery = null
  }
}

/**
 * Returns a reactive boolean indicating whether the viewport is narrow
 * (<= 767px). The breakpoint is unified for the share-materials family and
 * never consumed outside the 封闭清单 (ui-guidelines §5, DESIGN.md Layout).
 *
 * @returns Readonly ref: true if viewport width <= 767px, false otherwise.
 */
export function useBreakpoint() {
  onMounted(() => {
    addListener()
  })

  onUnmounted(() => {
    removeListener()
  })

  // Return readonly to prevent consumers from mutating the shared state.
  return { isNarrow: readonly(isNarrow) }
}
