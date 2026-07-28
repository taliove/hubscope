// Pure presentation logic for the degraded-cause sub-label (spec 0011):
// the cause vocabulary and the suffix rendered after the status word in
// StatusBadge. Single source of truth — components must never write the
// cause wordings as literals.
//
// The suffix is plain secondary text, never a second status light: no dot,
// no icon, no background, no animation (ui-guidelines §5 StatusBadge).
import type { DegradeCause } from '@/api/types'

// Cause wordings (ui-guidelines §7 vocabulary): availability = the endpoint
// fails intermittently, latency = it answers but has become slow.
export const DEGRADE_CAUSE_LABELS: Record<DegradeCause, string> = {
  availability: '可用性',
  latency: '延迟',
}

// Sub-label suffix for a degraded badge: '· 可用性', '· 延迟', or
// '· 可用性 + 延迟' when both rules hit (availability first, matching the
// backend order). Empty input yields an empty string — nothing renders.
export function degradeCauseSuffix(causes: DegradeCause[]): string {
  if (causes.length === 0) return ''
  return `· ${causes.map((c) => DEGRADE_CAUSE_LABELS[c]).join(' + ')}`
}
