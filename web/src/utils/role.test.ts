// Unit tests for the role label + tag type mapping (ui-guidelines §5,
// ticket 62; vitest coverage added GH #119). The mapping is the single
// source of truth for the role-tag consumers (AppSidebar account line,
// UserManager) — the load-bearing calibers pinned here: the four-word
// vocabulary never grows synonyms, management tiers claim the brand color,
// and unknown roles must never claim a management color.
import { describe, expect, it } from 'vitest'
import { roleLabel, roleTagType } from '@/utils/role'

describe('roleLabel', () => {
  it('maps the four roles to the fixed Chinese vocabulary', () => {
    expect(roleLabel('super_admin')).toBe('超级管理员')
    expect(roleLabel('admin')).toBe('管理员')
    expect(roleLabel('operator')).toBe('操作员')
    expect(roleLabel('viewer')).toBe('观察者')
  })

  it('falls back to the unknown-user placeholder for unknown roles', () => {
    expect(roleLabel('root')).toBe('未知用户')
    expect(roleLabel('')).toBe('未知用户')
  })

  it('falls back to the unknown-user placeholder for null/undefined', () => {
    expect(roleLabel(null)).toBe('未知用户')
    expect(roleLabel(undefined)).toBe('未知用户')
  })
})

describe('roleTagType', () => {
  it('maps the management tiers to primary (brand, has user management)', () => {
    expect(roleTagType('super_admin')).toBe('primary')
    expect(roleTagType('admin')).toBe('primary')
  })

  it('maps the non-management tiers to info (neutral)', () => {
    expect(roleTagType('operator')).toBe('info')
    expect(roleTagType('viewer')).toBe('info')
  })

  it('falls back to info for unknown/null roles (never claims management emphasis)', () => {
    expect(roleTagType('root')).toBe('info')
    expect(roleTagType(null)).toBe('info')
    expect(roleTagType(undefined)).toBe('info')
  })
})
