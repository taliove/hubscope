// Role label + tag type mapping (ui-guidelines §5, ticket 62).
// Role semantics is "permission tier", NOT health — status colors
// (success/warning/danger) are reserved for endpoint & batch health
// signals per §3, so role tags only use primary (brand) and info (neutral).

import type { TagProps } from 'element-plus'

export type Role = 'super_admin' | 'admin' | 'operator' | 'viewer'

const ROLE_LABELS: Record<Role, string> = {
  super_admin: '超级管理员',
  admin: '管理员',
  operator: '操作员',
  viewer: '观察者',
}

// el-tag `type` accepts 'primary' | 'success' | 'warning' | 'danger' | 'info'.
// Roles only use primary (super_admin/admin — have user management) and
// info (operator/viewer — no user management). Brand vs neutral distinguishes
// the tier without color-weighting; the word label tells them apart.
const ROLE_TAG_TYPES: Record<Role, TagProps['type']> = {
  super_admin: 'primary',
  admin: 'primary',
  operator: 'info',
  viewer: 'info',
}

// roleLabel returns the Chinese display word for a role. Unknown/null
// roles fall back to the "unknown" placeholder so the UI never renders
// a bare technical role string to end readers.
export function roleLabel(role: string | null | undefined): string {
  if (role && role in ROLE_LABELS) {
    return ROLE_LABELS[role as Role]
  }
  return '未知用户'
}

// roleTagType returns the el-tag `type` for a role. Unknown/null falls
// back to 'info' (neutral) per the three-state spirit — no color emphasis
// when identity is ambiguous.
export function roleTagType(role: string | null | undefined): TagProps['type'] {
  if (role && role in ROLE_TAG_TYPES) {
    return ROLE_TAG_TYPES[role as Role]
  }
  return 'info'
}
