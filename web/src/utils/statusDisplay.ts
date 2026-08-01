// Display-layer status mapping (spec 0018 「显示层状态映射」, GH #113; word
// list switched to the reference-design vocabulary 2026-08-01, GH #128): the
// SINGLE point where the backend's four-state domain model collapses into
// the three display states of UI v2 — healthy → 稳定运行 (success), degraded
// → 降级 (warning, degrade-cause sub-label preserved), down → 异常 (danger),
// failing → 异常 (danger; failing survives only in the domain model, the
// Lark pipeline and the alert-history event vocabulary, never on a status
// surface).
//
// Centralization discipline (role.ts / degradeCauses.ts precedent): every
// status word and color slot comes from this module — components must never
// write the status words as literals. The alert-event vocabulary (nine
// kinds, utils/alertKind.ts) is a separate event-category domain and is NOT
// routed through this mapping.
//
// Color slots vs tokens: this module returns abstract slots
// (success/warning/danger). Components map a slot to tokens by channel —
// text consumes the *-text grade (--hs-success-text etc., the graphic/text
// division), dots and lamps consume the body grade (--hs-success etc.).
import type { DegradeCause, EndpointStatus } from '@/api/types'
import { degradeCauseSuffix } from '@/utils/degradeCauses'

// The three display states (Chinese-primary vocabulary; Stable / Degraded /
// Incident survive only as the English-name archive of spec 0018 §11).
export type DisplayStatus = 'stable' | 'degraded' | 'incident'

// Abstract color slot of a display state; token resolution happens in the
// consuming component's CSS (text grade vs body grade per channel).
export type DisplayTone = 'success' | 'warning' | 'danger'

export interface StatusDisplay {
  status: DisplayStatus
  label: string // 稳定运行 / 降级 / 异常
  tone: DisplayTone
  // Degrade-cause sub-label suffix ('· 可用性' etc.); non-empty ONLY for the
  // degraded display state with causes passed in — defense in depth so a
  // caller passing causes for another state renders nothing.
  causeSuffix: string
}

const DISPLAY_INFO: Record<DisplayStatus, { label: string; tone: DisplayTone }> = {
  stable: { label: '稳定运行', tone: 'success' },
  degraded: { label: '降级', tone: 'warning' },
  incident: { label: '异常', tone: 'danger' },
}

// Domain → display: the four-state status machine (W5, untouched) collapses
// here; failing loses its separate display identity (spec 0018 decision 3).
const DOMAIN_TO_DISPLAY: Record<EndpointStatus, DisplayStatus> = {
  healthy: 'stable',
  degraded: 'degraded',
  down: 'incident',
  failing: 'incident',
}

// Defensive fallback for runtime payloads outside the domain union (the API
// is untyped at the wire): never render a bare technical string, and pick
// the middle slot — warning never reads as "healthy" (false comfort) nor as
// "incident" (false alarm), matching the role.ts neutral-fallback spirit.
const UNKNOWN_DISPLAY: StatusDisplay = { status: 'degraded', label: '未知', tone: 'warning', causeSuffix: '' }

// Display-layer severity order, heavy → light (mirror of severitySort's
// SEVERITY_ORDER): consumed wherever display-status counts are listed —
// the Hero band counts row and the group-header count chips.
export const DISPLAY_SEVERITY_ORDER: DisplayStatus[] = ['incident', 'degraded', 'stable']

// statusDisplay is the mapping entry point. It accepts a domain status, an
// already-collapsed display status (aggregate scenes — merged count rows —
// render a badge for the display state itself), or a raw string (defensive).
// causes is the degrade-cause passthrough (spec 0013 sub-label), honored
// only when the resolved display state is 'degraded'.
export function statusDisplay(status: EndpointStatus | DisplayStatus | string, causes?: DegradeCause[]): StatusDisplay {
  const displayStatus = toDisplayStatus(status)
  if (displayStatus === null) return { ...UNKNOWN_DISPLAY }
  const info = DISPLAY_INFO[displayStatus]
  const causeSuffix =
    displayStatus === 'degraded' && causes && causes.length > 0 ? degradeCauseSuffix(causes) : ''
  return { status: displayStatus, label: info.label, tone: info.tone, causeSuffix }
}

// toDisplayStatus collapses a domain status to its display state; returns
// null for anything outside both unions (defensive branch of statusDisplay).
export function toDisplayStatus(status: EndpointStatus | DisplayStatus | string): DisplayStatus | null {
  if (status in DOMAIN_TO_DISPLAY) return DOMAIN_TO_DISPLAY[status as EndpointStatus]
  if (status in DISPLAY_INFO) return status as DisplayStatus
  return null
}

// Convenience accessors for the common one-field cases.
export function statusLabel(status: EndpointStatus | DisplayStatus | string): string {
  return statusDisplay(status).label
}

export function statusTone(status: EndpointStatus | DisplayStatus | string): DisplayTone {
  return statusDisplay(status).tone
}

// displayStatusCounts merges domain-status counts into display-status counts
// (down + failing → incident). Consumed by the Hero band counts row and the
// group-header count chips so a mixed group never shows the 异常 word
// twice side by side. Unknown keys are ignored (defensive).
export function displayStatusCounts(
  counts: Partial<Record<EndpointStatus, number>>,
): Record<DisplayStatus, number> {
  return {
    stable: counts.healthy ?? 0,
    degraded: counts.degraded ?? 0,
    incident: (counts.down ?? 0) + (counts.failing ?? 0),
  }
}
