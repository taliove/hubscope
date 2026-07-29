// Pure logic for the console live feed (issue #17): cursor bookkeeping,
// incremental merge and the verdict-method vocabulary. Kept component-free
// so vitest covers the empty/append/dedupe/cap behaviors directly.
import type { LiveFeedEntry } from '@/api/types'

// Upper bound of accumulated feed entries: a long batch cannot grow the
// list without bound; once capped, the oldest events drop off the view
// (they remain fetchable via the API).
export const LIVE_FEED_CAP = 200

// Next since_id cursor: the largest id seen so far. Entries accumulate in
// ascending order (the API serves ascending pages), so the cursor is the
// last entry's id; 0 before the first pull.
export function liveFeedCursor(entries: LiveFeedEntry[]): number {
  return entries.length === 0 ? 0 : entries[entries.length - 1].id
}

// Append one incremental page to the accumulated feed. The API guarantees
// ascending ids strictly after the cursor, but the merge still dedupes by
// id so an overlapping refetch (same cursor computed twice in flight) can
// never double-render an event. The result is capped to the newest
// LIVE_FEED_CAP entries. Inputs stay untouched.
export function mergeLiveFeed(
  existing: LiveFeedEntry[],
  incoming: LiveFeedEntry[],
  cap: number = LIVE_FEED_CAP,
): LiveFeedEntry[] {
  if (incoming.length === 0) return existing
  const seen = new Set(existing.map((e) => e.id))
  const merged = [...existing]
  for (const entry of incoming) {
    if (!seen.has(entry.id)) {
      seen.add(entry.id)
      merged.push(entry)
    }
  }
  if (merged.length > cap) return merged.slice(merged.length - cap)
  return merged
}

// Display order: newest first — the feed reads as a stream of the latest
// events (the API serves oldest-first pages for the cursor's sake).
export function liveFeedDisplay(entries: LiveFeedEntry[]): LiveFeedEntry[] {
  return [...entries].reverse()
}

// Row-expansion toggle (GH #41): the expansion set is keyed by entry id, so
// polling prepends never collapse an open row. Returns a fresh set; the
// input stays untouched.
export function toggleExpansion(expanded: ReadonlySet<number>, id: number): Set<number> {
  const next = new Set(expanded)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}

// Verdict-method vocabulary (ui-guidelines §7: wording centralized in
// utils). An empty type (the case was purged after judging) renders a
// neutral dash, same as the unknown-word fallback discipline.
export function verdictTypeLabel(verdictType: string): string {
  if (verdictType === 'rule') return '规则'
  if (verdictType === 'judge') return '裁判'
  return '-'
}

// A judge failure: the case went to the judge model but came back unscored
// (W7: judge failure is null, never zero). These rows — and only these —
// carry the GH #29 deep link to the judge-model setting; a rule failure is
// a case-authoring matter, not a judge-setting one.
export function isJudgeFailure(entry: Pick<LiveFeedEntry, 'verdict_type' | 'score'>): boolean {
  return entry.verdict_type === 'judge' && entry.score === null
}
