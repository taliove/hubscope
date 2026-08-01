// Severity-driven first-screen organization of the status board (GH #52):
// the board leads with the most severe content instead of the vendor
// alphabetical order — down/failing endpoints must never fall out of the
// first viewport. SEVERITY_RANK here is the SINGLE SOURCE of the severity
// rank; every consumer (Dashboard group sections, flat matrix, StatusCard
// abnormal detail) references it instead of re-declaring the ordering.
import type { EndpointStatus, OverviewEntry, OverviewGroup } from '@/api/types'

// Severity rank: lower = more severe = earlier on the board.
export const SEVERITY_RANK: Record<EndpointStatus, number> = {
  failing: 0,
  down: 1,
  degraded: 2,
  healthy: 3,
}

// The same severity caliber as an ordered LIST (GH #55): consumed wherever
// a status sequence is rendered — the Dashboard stats strip order and the
// group header status-count order. Both used to declare their own orderings
// (strip mild→severe, header its own priority list); they now share this
// single source so the board has one severity caliber, heavy→light. The
// test suite asserts this array stays consistent with SEVERITY_RANK.
export const SEVERITY_ORDER: EndpointStatus[] = ['failing', 'down', 'degraded', 'healthy']

// Rank of a disabled endpoint — and of a group with no enabled entries:
// sinks below every enabled rank. A disabled endpoint is out of service by
// admin choice; it must never compete for the first screen, and a disabled
// down endpoint must never lift its group's rank.
export const DISABLED_RANK = 4

// entryRank is the per-endpoint rank: disabled always DISABLED_RANK
// whatever the status machine says.
export function entryRank(entry: OverviewEntry): number {
  return entry.enabled ? SEVERITY_RANK[entry.status] : DISABLED_RANK
}

// groupRank is the group's rank = the best (smallest) rank among its
// ENABLED entries (computed on the post-filter entry set). A group with no
// enabled entries ranks DISABLED_RANK and sinks to the bottom.
export function groupRank(entries: OverviewEntry[]): number {
  let rank = DISABLED_RANK
  for (const entry of entries) {
    if (entry.enabled) rank = Math.min(rank, SEVERITY_RANK[entry.status])
  }
  return rank
}

// lex compares two strings by code unit, never localeCompare — the board's
// tie-break must be deterministic across locales.
function lex(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

// sortEntriesBySeverity returns a FRESH array ranked by severity ascending;
// ties break by model_id → protocol → endpoint_id so the board is
// deterministic. Never mutates the input (boardSort precedent).
export function sortEntriesBySeverity(entries: OverviewEntry[]): OverviewEntry[] {
  return [...entries].sort((a, b) => {
    const ra = entryRank(a)
    const rb = entryRank(b)
    if (ra !== rb) return ra - rb
    return lex(a.model_id, b.model_id) || lex(a.protocol, b.protocol) || a.endpoint_id - b.endpoint_id
  })
}

// sortEntriesByAvailability ranks the list LOWEST 24h availability first
// (GH #131, reference-design list): the headline note 「(按可用率排序)」
// must be literally true — a label the data does not honor is an anti-fake
// violation. Buckets: rated enabled rows (ascending rate; ties fall back to
// the severity rank so a failing 0% still leads a down 0%) → enabled rows
// with a null rate (no probes in the window: no-data must never read as 0%
// false alarm nor 100% false comfort) → disabled rows last (DISABLED_RANK
// spirit). Final ties break by model_id → protocol → endpoint_id. Never
// mutates the input.
export function sortEntriesByAvailability(entries: OverviewEntry[]): OverviewEntry[] {
  const bucket = (e: OverviewEntry) => (!e.enabled ? 2 : e.success_rate_24h === null ? 1 : 0)
  return [...entries].sort((a, b) => {
    const ba = bucket(a)
    const bb = bucket(b)
    if (ba !== bb) return ba - bb
    if (ba === 0) {
      const ra = a.success_rate_24h as number
      const rb = b.success_rate_24h as number
      if (ra !== rb) return ra - rb
      const sa = entryRank(a)
      const sb = entryRank(b)
      if (sa !== sb) return sa - sb
    }
    return lex(a.model_id, b.model_id) || lex(a.protocol, b.protocol) || a.endpoint_id - b.endpoint_id
  })
}

// One group section of the status matrix: the aggregate plus its
// post-filter entries.
export interface GroupSection {
  group: OverviewGroup
  entries: OverviewEntry[]
}

// sortGroupSections returns FRESH sections ranked by groupRank ascending —
// the group's most severe ENABLED entry decides; ties (and the sink group:
// no enabled entries after filtering) break by the group key lexicographic.
// Entries inside each section are ranked by sortEntriesBySeverity, so a
// section arrives fully ordered. Never mutates the input array or its
// sections.
export function sortGroupSections(sections: GroupSection[]): GroupSection[] {
  return [...sections]
    .map((section) => ({ ...section, entries: sortEntriesBySeverity(section.entries) }))
    .sort((a, b) => {
      const ra = groupRank(a.entries)
      const rb = groupRank(b.entries)
      if (ra !== rb) return ra - rb
      return lex(a.group.key, b.group.key)
    })
}
