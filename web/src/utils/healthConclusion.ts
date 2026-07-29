// Shared health-conclusion logic and status vocabulary. Extracted from
// HealthBanner so the banner (global scope, never filtered) and the
// StatusCard (filtered scope, ticket 56) speak with one voice — same words,
// same thresholds, same conclusion sentences (ui-guidelines §3/§7).
// Pure functions only: callers decide which entries enter the math
// (banner passes all enabled endpoints, the card its filtered snapshot).
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'
import { sortEntriesBySeverity } from '@/utils/severitySort'

// Canonical endpoint status words; the only source for these labels.
export const STATUS_LABELS: Record<EndpointStatus, string> = {
  healthy: '正常',
  degraded: '降级',
  down: '宕机',
  failing: '告警',
}

export type HealthTone = 'healthy' | 'degraded' | 'abnormal'

export type HealthCounts = Record<EndpointStatus, number>

export function emptyHealthCounts(): HealthCounts {
  return { healthy: 0, degraded: 0, down: 0, failing: 0 }
}

export function countByStatus(entries: OverviewEntry[]): HealthCounts {
  const counts = emptyHealthCounts()
  for (const entry of entries) counts[entry.status] += 1
  return counts
}

export function toneOf(counts: HealthCounts): HealthTone {
  if (counts.down + counts.failing > 0) return 'abnormal'
  if (counts.degraded > 0) return 'degraded'
  return 'healthy'
}

// One-sentence conclusion shared by banner and card. `empty` renders the
// neutral wording so zero data never reads as a misleading "全部正常".
export function conclusionText(tone: HealthTone, counts: HealthCounts, empty: boolean): string {
  if (empty) return '暂无数据'
  if (tone === 'abnormal') return `${counts.down + counts.failing} 个端点异常`
  if (tone === 'degraded') return `${counts.degraded} 个端点降级`
  return '全部正常'
}

// Cap of abnormal-endpoint chips on the HealthBanner (GH #53); the rest
// collapse into a neutral "+N" overflow chip.
export const MAX_ABNORMAL_CHIPS = 5

// Identity of one abnormal endpoint chip: no copy literals — the status word
// comes from STATUS_LABELS at render time.
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
// (GH #53). Abnormal = failing + down + degraded, ranked most-severe-first
// through the board's single severity ordering (sortEntriesBySeverity, GH #52)
// — never a second rank table. Callers pass the already enabled-filtered
// entries; never mutates the input.
export function abnormalChips(entries: OverviewEntry[]): AbnormalChipsResult {
  const abnormal = sortEntriesBySeverity(entries.filter(e => e.status !== 'healthy'))
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
