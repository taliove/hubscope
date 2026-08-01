// Model-status-list row derivations (GH #131, UI v2 optimization O4):
// vendor-chip initials, availability progress-bar width, per-row latency
// sparkline series and its semantic tone, and the vendor-filter option list
// — centralized as pure functions so the list component never carries math
// (role.ts / overviewMetrics.ts precedent).
import type { OverviewDot, OverviewEntry } from '@/api/types'
import { toDisplayStatus } from '@/utils/statusDisplay'

// --- Vendor chip initials ---------------------------------------------------

// Initials for the vendor icon chip (≤3 chars): a single Latin word yields
// its first TWO letters uppercased (anthropic → AN, google → GO); a short
// word (≤3 chars) yields the whole word (gpt → GPT); a multi-word name
// yields the first character of each of the first three words (Ali Cloud →
// AC); CJK names yield the name itself when ≤3 chars (阿里 → 阿里), else the
// first character. No vendor color board and no real logo assets — the chip
// stays a neutral soft block (registered deviation of GH #131).
export function familyInitials(family: string): string {
  const words = family
    .trim()
    .split(/[^a-zA-Z0-9一-鿿]+/)
    .filter(Boolean)
  if (words.length === 0) return '—'
  if (words.length > 1) {
    return words
      .slice(0, 3)
      .map(w => [...w][0])
      .join('')
      .toUpperCase()
  }
  const chars = [...words[0]]
  if (chars.length <= 3) return words[0].toUpperCase()
  return /^[a-zA-Z0-9]/.test(chars[0]) ? (chars[0] + chars[1]).toUpperCase() : chars[0]
}

// --- Availability progress bar ----------------------------------------------

// Bar fill width as a 0–100 percentage of the constant-scale track: the bar
// is the same 0–100 absolute scale as the score bars (W7 visual mirror
// spirit) — never normalized per row. Null (no probes in the window) yields
// 0: the empty gray track pairs with the「-」number and never reads as data.
export function availabilityBarWidth(rate: number | null): number {
  if (rate === null || rate === undefined) return 0
  return Math.min(100, Math.max(0, rate * 100))
}

// --- Row trend sparkline ----------------------------------------------------

// Per-row 24h latency series for the trend column: the entry's own hourly
// P50 buckets (successful probes only — failed-probe latency is
// time-to-failure and never feeds the curve, the LatencySparkline
// discipline). Null buckets break the line.
export function entryLatencySeries(dots: OverviewDot[]): (number | null)[] {
  return dots.map(d => d.p50_ms)
}

// Semantic tone of the row sparkline, by the entry's DISPLAY state
// (statusDisplay single mapping): stable → success, degraded → warning,
// incident (down + failing) → danger. Disabled endpoints render neutral —
// out of service by admin choice, the curve carries no health signal.
// The row subset of TrendSparkline's SparklineTone (GH #130): the brand
// lane belongs to the request widget, never to a health row.
export type SparklineTone = 'neutral' | 'success' | 'warning' | 'danger'

export function rowSparklineTone(entry: OverviewEntry): SparklineTone {
  if (!entry.enabled) return 'neutral'
  const display = toDisplayStatus(entry.status)
  if (display === 'stable') return 'success'
  if (display === 'degraded') return 'warning'
  if (display === 'incident') return 'danger'
  return 'neutral'
}

// --- Vendor filter options --------------------------------------------------

// Options of the vendor (供应商) filter: the distinct family values of the
// UNFILTERED entry set, lexicographic by code unit (board determinism —
// never localeCompare). Derived from the full set so an active filter never
// collapses its own option list.
export function familyOptions(entries: OverviewEntry[]): string[] {
  return [...new Set(entries.map(e => e.family))].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
}
