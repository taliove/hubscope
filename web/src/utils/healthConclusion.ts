// Shared health-conclusion logic and status vocabulary. Extracted from
// HealthBanner so the banner (global scope, never filtered) and the
// StatusCard (filtered scope, ticket 49) speak with one voice — same words,
// same thresholds, same conclusion sentences (ui-guidelines §3/§7).
// Pure functions only: callers decide which entries enter the math
// (banner passes all enabled endpoints, the card its filtered snapshot).
import type { EndpointStatus, OverviewEntry } from '@/api/types'

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
