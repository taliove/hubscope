// Unified responsive breakpoint mechanism (GH #94): a single 1024px breakpoint
// consumed by the share-materials family (dialogs, Leaderboard,
// EvalProgressGrid, /report/:token page). The state is reactive and updates on
// viewport resize; every consumer mounting this composable shares the same
// MediaQueryList listener, so the overhead stays constant even with multiple
// consuming components on screen.封闭清单 = StatusShareDialog /
// EvalShareDialog / Leaderboard / EvalProgressGrid / CampaignReportView (shared
// mode) — 2026-08-01 外壳抽屉批扩入:外壳(App.vue 抽屉状态 / AppTopbar 汉堡 /
// AppSidebar overlay 抽屉)与状态概览页(DashboardView 族,ModelStatusList 卡片
// 形态)。修订登记:原「dashboard 不消费断点」的桌面优先声明在第八轮实机反馈后
// 修订(用户经 Wireguard 手机访问);管理台内容区(EP 表格)仍不消费,保持桌面
// 形态(ui-guidelines §5, DESIGN.md Layout)。
import { ref, onMounted, onUnmounted, readonly, type Ref } from 'vue'

// The single breakpoint value for the whole app: 1023px max-width means
// narrow viewports (<=1023px) get the responsive treatment, and desktop
// (>=1024px) is pixel-perfect unchanged. Moved up from 768px on 2026-08-01
// (round-9 device ruling): the 768–1023px band squeezed the 7-column model
// list into overlap/clip territory under the 220px sidebar — the move
// covers the whole band, and every consumer (share-materials family
// included) follows the single value.
const BREAKPOINT_QUERY = '(max-width: 1023px)'

let mediaQuery: MediaQueryList | null = null
let listenerCount = 0

// Reactive state shared across all consumers of this composable.
const isNarrow = ref(false)

function updateNarrowState() {
  if (mediaQuery) {
    isNarrow.value = mediaQuery.matches
  }
}

// Subscribes to the shared MediaQueryList: the first subscriber creates it and
// seeds the state, later subscribers share the same listener, and the last
// release tears it down. Exported as the testable seam (the project has no
// DOM test environment, so the composable's lifecycle glue stays thin and
// untested, mirroring the visibilityPoll factory precedent); components must
// consume useBreakpoint, not this.
export function subscribeBreakpoint(): { isNarrow: Readonly<Ref<boolean>>; release: () => void } {
  if (listenerCount === 0) {
    mediaQuery = window.matchMedia(BREAKPOINT_QUERY)
    isNarrow.value = mediaQuery.matches
    mediaQuery.addEventListener('change', updateNarrowState)
  }
  listenerCount += 1
  return {
    isNarrow: readonly(isNarrow),
    release: () => {
      listenerCount -= 1
      if (listenerCount === 0 && mediaQuery) {
        mediaQuery.removeEventListener('change', updateNarrowState)
        mediaQuery = null
      }
    },
  }
}

/**
 * Returns a reactive boolean indicating whether the viewport is narrow
 * (<= 1023px). The breakpoint is unified across the share-materials family,
 * the shell, and the dashboard — never consumed outside the 封闭清单
 * (ui-guidelines §5, DESIGN.md Layout).
 *
 * @returns Readonly ref: true if viewport width <= 1023px, false otherwise.
 */
export function useBreakpoint() {
  let release: (() => void) | null = null

  onMounted(() => {
    release = subscribeBreakpoint().release
  })

  onUnmounted(() => {
    release?.()
  })

  // Return readonly to prevent consumers from mutating the shared state.
  return { isNarrow: readonly(isNarrow) }
}
