// Display-layer status mapping (spec 0018 「显示层状态映射」, GH #113; word
// list switched to the reference-design vocabulary 2026-08-01, GH #128;
// stable shortened 稳定运行 → 稳定 on 2026-08-02 by user ruling; unverified
// tier added 2026-08-02, GH #160): the SINGLE point where the backend's
// domain values collapse into the 3+1 display states of UI v2 — healthy →
// 稳定 (success), degraded → 降级 (warning, degrade-cause sub-label
// preserved), down → 异常 (danger), failing → 异常 (danger; failing
// survives only in the domain model, the Lark pipeline and the
// alert-history event vocabulary, never on a status surface), unverified →
// 未验证 (neutral — the Ping-monitoring no-evidence identity; the fourth
// PRESENTATION, not a fourth hue).
//
// 未验证 vs 未知 (GH #160): 未验证 is the formal identity of the in-domain
// unverified value; 未知 stays the defensive fallback for out-of-domain
// runtime values (a future domain value not yet synced to the frontend).
// The two never swap.
//
// Centralization discipline (role.ts / degradeCauses.ts precedent): every
// status word and color slot comes from this module — components must never
// write the status words as literals. The alert-event vocabulary (nine
// kinds, utils/alertKind.ts) is a separate event-category domain and is NOT
// routed through this mapping.
//
// Color slots vs tokens: this module returns abstract slots
// (success/warning/danger/neutral). Components map a slot to tokens by
// channel — text consumes the *-text grade (--hs-success-text etc., the
// graphic/text division; the neutral tier's word consumes
// --hs-text-placeholder), dots and lamps consume the body grade (the
// neutral tier's dot consumes --hs-info gray).
import type { DegradeCause, EndpointStatus } from '@/api/types'
import { degradeCauseSuffix } from '@/utils/degradeCauses'

// The 3+1 display states (Chinese-primary vocabulary; Stable / Degraded /
// Incident survive only as the English-name archive of spec 0018 §11).
export type DisplayStatus = 'stable' | 'degraded' | 'incident' | 'unverified'

// Abstract color slot of a display state; token resolution happens in the
// consuming component's CSS (text grade vs body grade per channel). The
// fourth slot 'neutral' serves unverified only — it introduces no new
// functional hue (word = placeholder grade, dot = info gray).
export type DisplayTone = 'success' | 'warning' | 'danger' | 'neutral'

export interface StatusDisplay {
  status: DisplayStatus
  label: string // 稳定 / 降级 / 异常 / 未验证
  tone: DisplayTone
  // Degrade-cause sub-label suffix ('· 可用性' etc.); non-empty ONLY for the
  // degraded display state with causes passed in — defense in depth so a
  // caller passing causes for another state renders nothing.
  causeSuffix: string
}

const DISPLAY_INFO: Record<DisplayStatus, { label: string; tone: DisplayTone }> = {
  stable: { label: '稳定', tone: 'success' },
  degraded: { label: '降级', tone: 'warning' },
  incident: { label: '异常', tone: 'danger' },
  // 未验证: neutral slot — never warning yellow (yellow = degraded only);
  // no evidence reads as neither good nor bad (GH #160 ruling ①).
  unverified: { label: '未验证', tone: 'neutral' },
}

// Domain → display: the four-state status machine (W5, untouched) collapses
// here; failing loses its separate display identity (spec 0018 decision 3).
// unverified is not a machine output — it is the Ping-endpoint presentation
// identity the backend overrides in (overview + detail APIs, GH #160) — and
// maps straight through.
const DOMAIN_TO_DISPLAY: Record<EndpointStatus, DisplayStatus> = {
  healthy: 'stable',
  degraded: 'degraded',
  down: 'incident',
  failing: 'incident',
  unverified: 'unverified',
}

// Defensive fallback for runtime payloads outside the domain union (the API
// is untyped at the wire): never render a bare technical string, and pick
// the middle slot — warning never reads as "healthy" (false comfort) nor as
// "incident" (false alarm), matching the role.ts neutral-fallback spirit.
// In-domain unverified never routes here (formal identity above).
const UNKNOWN_DISPLAY: StatusDisplay = { status: 'degraded', label: '未知', tone: 'warning', causeSuffix: '' }

// Display-layer severity order, heavy → light (mirror of severitySort's
// SEVERITY_ORDER): consumed wherever display-status counts are listed —
// the Hero band counts row and the group-header count chips. GH #160 ruling
// ④: unverified ranks heavier than stable but lighter than degraded — no
// evidence is not an alarm, but it is not "good" either.
export const DISPLAY_SEVERITY_ORDER: DisplayStatus[] = ['incident', 'degraded', 'unverified', 'stable']

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
// (down + failing → incident; unverified passes through unmerged, GH #160).
// Consumed by the Hero band counts row and the group-header count chips so a
// mixed group never shows the 异常 word twice side by side. Unknown keys are
// ignored (defensive).
export function displayStatusCounts(
  counts: Partial<Record<EndpointStatus, number>>,
): Record<DisplayStatus, number> {
  return {
    stable: counts.healthy ?? 0,
    degraded: counts.degraded ?? 0,
    unverified: counts.unverified ?? 0,
    incident: (counts.down ?? 0) + (counts.failing ?? 0),
  }
}
