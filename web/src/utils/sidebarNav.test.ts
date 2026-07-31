// Vitest coverage for the sidebar navigation model (GH #112, spec 0018
// testing decisions: login-state filtering is a listed pure-function seam).
import { describe, expect, it } from 'vitest'
import { SIDEBAR_NAV_ITEMS, isSidebarItemActive, visibleSidebarItems } from '@/utils/sidebarNav'
import type { AuthUser } from '@/api/auth'

const admin: AuthUser = { id: 1, username: 'devadmin', role: 'super_admin', hub_id: null, hub_name: null }

describe('visibleSidebarItems', () => {
  it('anonymous visitors only see the two public entries', () => {
    const items = visibleSidebarItems(null)
    expect(items.map((i) => i.key)).toEqual(['dashboard', 'benchmark'])
  })

  it('logged-in users see the full IA in declaration order', () => {
    const items = visibleSidebarItems(admin)
    expect(items.map((i) => i.key)).toEqual(['dashboard', 'benchmark', 'eval', 'models', 'alerts', 'settings'])
    expect(items).toBe(SIDEBAR_NAV_ITEMS)
  })
})

describe('isSidebarItemActive', () => {
  const byKey = (key: string) => SIDEBAR_NAV_ITEMS.find((i) => i.key === key)!

  it('dashboard claims / exactly and the endpoint deep links', () => {
    const item = byKey('dashboard')
    expect(isSidebarItemActive(item, '/')).toBe(true)
    expect(isSidebarItemActive(item, '/endpoints/12')).toBe(true)
    expect(isSidebarItemActive(item, '/benchmark')).toBe(false)
  })

  it('benchmark claims /benchmark and the legacy /board path', () => {
    const item = byKey('benchmark')
    expect(isSidebarItemActive(item, '/benchmark')).toBe(true)
    expect(isSidebarItemActive(item, '/board')).toBe(true)
    expect(isSidebarItemActive(item, '/eval')).toBe(false)
  })

  it('eval claims /eval and the campaign report deep link', () => {
    const item = byKey('eval')
    expect(isSidebarItemActive(item, '/eval')).toBe(true)
    expect(isSidebarItemActive(item, '/campaigns/12/report')).toBe(true)
    expect(isSidebarItemActive(item, '/report/some-token')).toBe(false)
  })

  it('models and alerts claim exactly their own subtree', () => {
    expect(isSidebarItemActive(byKey('models'), '/models')).toBe(true)
    expect(isSidebarItemActive(byKey('models'), '/models/add')).toBe(true)
    expect(isSidebarItemActive(byKey('models'), '/alerts')).toBe(false)
    expect(isSidebarItemActive(byKey('alerts'), '/alerts')).toBe(true)
    expect(isSidebarItemActive(byKey('alerts'), '/settings')).toBe(false)
  })

  it('settings claims /settings and the folded-in legacy /tasks route', () => {
    const item = byKey('settings')
    expect(isSidebarItemActive(item, '/settings')).toBe(true)
    expect(isSidebarItemActive(item, '/tasks')).toBe(true)
    expect(isSidebarItemActive(item, '/login')).toBe(false)
  })

  it('prefixes never over-match sibling paths', () => {
    // "/benchmarks" (note the trailing s) is not the benchmark section.
    expect(isSidebarItemActive(byKey('benchmark'), '/benchmarks')).toBe(false)
    expect(isSidebarItemActive(byKey('models'), '/modelsx')).toBe(false)
  })
})
