// Sidebar navigation model (GH #112, spec 0018 IA mapping): the v2 macOS
// Settings-style sidebar replaced the v1 AppHeader; GH #135 restores a slim top bar (AppTopbar) alongside it. Item definitions,
// login-state visibility filtering, and active-route matching live here as
// pure functions (role.ts / adminNav.ts centralization precedent) so the
// AppSidebar component stays declarative and the behavior is vitest-covered.
// Labels are the spec 0018 IA vocabulary — never component literals.
import type { AuthUser } from '@/api/auth'

export interface SidebarNavItem {
  // Stable key; AppSidebar maps it to the line-icon component.
  key: string
  label: string
  to: string
  // Public items render for anonymous visitors; the rest would only be
  // bounced to /login by the route guard, so they are hidden instead
  // (AppHeader precedent, spec 0018 user story 3).
  public?: boolean
  // Extra route prefixes (besides `to`) that count as "this section" for
  // active highlighting — deep links and legacy paths.
  matchPrefixes?: string[]
}

export const SIDEBAR_NAV_ITEMS: SidebarNavItem[] = [
  // The dashboard owns both '/' and the endpoint detail pages.
  { key: 'dashboard', label: '状态概览', to: '/', public: true },
  // The public settled-batch board (spec 0010); /board is the legacy path
  // kept alive by a router redirect (GH #112) and still highlights here.
  { key: 'benchmark', label: 'Benchmark', to: '/benchmark', public: true, matchPrefixes: ['/board'] },
  // Session-gated console board; the campaign report page is entered from it.
  { key: 'eval', label: '评估中心', to: '/eval', matchPrefixes: ['/campaigns'] },
  { key: 'models', label: '模型管理', to: '/models' },
  { key: 'alerts', label: '故障记录', to: '/alerts' },
  // The task center folds into the settings area (spec 0018 decision 63):
  // the legacy /tasks route stays alive and highlights this item.
  { key: 'settings', label: '系统设置', to: '/settings', matchPrefixes: ['/tasks'] },
]

// Anonymous visitors only get the public entries; logout flips user to null
// and the filter collapses the nav back immediately (AppHeader precedent).
export function visibleSidebarItems(user: AuthUser | null): SidebarNavItem[] {
  return user ? SIDEBAR_NAV_ITEMS : SIDEBAR_NAV_ITEMS.filter((item) => item.public)
}

// Active-route matching: the dashboard claims '/' exactly plus the endpoint
// deep links; every other item matches its own path and declared prefixes.
export function isSidebarItemActive(item: SidebarNavItem, path: string): boolean {
  if (item.to === '/') return path === '/' || path.startsWith('/endpoints')
  const prefixes = [item.to, ...(item.matchPrefixes ?? [])]
  return prefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}/`))
}
