// Shared health-conclusion logic and status vocabulary. Originally extracted
// from the retired HealthBanner; current consumers — the overview hero
// (DashboardView, GH #115) and the StatusCard share material — speak with
// one voice: same words, same thresholds, same conclusion sentences.
// Pure functions only: callers decide which entries enter the math
// (banner passes all enabled endpoints, the card its filtered snapshot).
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'
import { sortEntriesBySeverity } from '@/utils/severitySort'
import { statusLabel, toDisplayStatus } from '@/utils/statusDisplay'

// Status WORDS are owned by the display-layer mapping (utils/statusDisplay,
// GH #113) — the old four-state STATUS_LABELS table is retired with the
// three-state display language; this module composes its conclusions from
// statusLabel so the words still have a single source.

export type HealthTone = 'healthy' | 'degraded' | 'abnormal' | 'neutral'

export type HealthCounts = Record<EndpointStatus, number>

export function emptyHealthCounts(): HealthCounts {
  return { healthy: 0, degraded: 0, down: 0, failing: 0, unverified: 0 }
}

export function countByStatus(entries: OverviewEntry[]): HealthCounts {
  const counts = emptyHealthCounts()
  for (const entry of entries) {
    // Defensive (GH #160 L1): the wire is untyped — an out-of-domain runtime
    // status must not turn the table into NaN; the unknown key is ignored.
    if (entry.status in counts) counts[entry.status] += 1
  }
  return counts
}

export function toneOf(counts: HealthCounts): HealthTone {
  if (counts.down + counts.failing > 0) return 'abnormal'
  if (counts.degraded > 0) return 'degraded'
  return 'healthy'
}

// One-sentence conclusion shared by banner and card. `empty` renders the
// neutral wording so zero data never reads as a misleading "全部稳定".
// The status words compose from the display-layer mapping (three-state
// language, GH #113; reference-design vocabulary GH #128): abnormal covers
// down+failing — both display as 异常 — so the conclusion names the display
// state, not the domain ones.
export function conclusionText(tone: HealthTone, counts: HealthCounts, empty: boolean): string {
  if (empty) return '暂无数据'
  if (tone === 'abnormal') return `${counts.down + counts.failing} 个端点${statusLabel('incident')}`
  if (tone === 'degraded') return `${counts.degraded} 个端点${statusLabel('degraded')}`
  return `全部${statusLabel('stable')}`
}

// Neutral unverified sub-note of the hero conclusion row (GH #160 ruling
// ⑦): the conclusion WORD never changes for unverified (「全部稳定」 must
// not swallow the no-evidence dimension), so the hero appends 「N 个未验证」
// as a neutral note beside it. The word composes from the display-layer
// mapping like every status word. null when nothing is unverified.
export function unverifiedNote(counts: HealthCounts): string | null {
  if (counts.unverified <= 0) return null
  return `${counts.unverified} 个${statusLabel('unverified')}`
}

// Material-verdict variant of the note (GH #160, main ruling 2026-08-03 —
// same-source extension to the StatusCard face): the StatusCard verdict
// mirrors the hero, but the note rides the STABLE line only. An abnormal or
// degraded verdict already points at the worst problem, and the
// distribution string's fourth segment discloses the unverified count
// there — appending a second signal would dilute the alarm. Same wording
// source as the hero note (unverifiedNote); the summaryText one-liner
// appends the same note inline (its comment cross-references this one).
export function verdictUnverifiedNote(tone: HealthTone, counts: HealthCounts): string | null {
  return tone === 'healthy' ? unverifiedNote(counts) : null
}

// Cap of abnormal-endpoint chips carried by the old HealthBanner (GH #53,
// retired GH #115); the rest
// collapse into a neutral "+N" overflow chip.
export const MAX_ABNORMAL_CHIPS = 5

// Identity of one abnormal endpoint chip: no copy literals — the status word
// comes from the display-layer mapping (utils/statusDisplay) at render time.
export interface AbnormalChip {
  endpoint_id: number
  model_id: string
  protocol: Protocol
  status: EndpointStatus
}

export interface AbnormalChipsResult {
  chips: AbnormalChip[]
  overflow: number
}

// The banner must answer "WHICH endpoints are abnormal", not just "how many"
// (GH #53). Abnormal = the incident + degraded display states (failing, down,
// degraded), ranked most-severe-first through the board's single severity
// ordering (sortEntriesBySeverity, GH #52) — never a second rank table.
// unverified is NOT abnormal (GH #160: no evidence is not an alarm) and is
// excluded by the same display-state caliber. Callers pass the already
// enabled-filtered entries; never mutates the input.
export function abnormalChips(entries: OverviewEntry[]): AbnormalChipsResult {
  const abnormal = sortEntriesBySeverity(
    entries.filter(e => {
      const display = toDisplayStatus(e.status)
      return display === 'incident' || display === 'degraded'
    }),
  )
  return {
    chips: abnormal.slice(0, MAX_ABNORMAL_CHIPS).map(e => ({
      endpoint_id: e.endpoint_id,
      model_id: e.model_id,
      protocol: e.protocol,
      status: e.status,
    })),
    overflow: Math.max(0, abnormal.length - MAX_ABNORMAL_CHIPS),
  }
}
