// Severity-driven first-screen organization of the status board (GH #52):
// the board leads with the most severe content instead of the vendor
// alphabetical order — down/failing endpoints must never fall out of the
// first viewport. SEVERITY_RANK here is the SINGLE SOURCE of the severity
// rank; every consumer (Dashboard group sections, flat matrix, StatusCard
// abnormal detail) references it instead of re-declaring the ordering.
import type { EndpointStatus, OverviewEntry, OverviewGroup } from '@/api/types'

// Severity rank: lower = more severe = earlier on the board. GH #160 ruling
// ④: unverified sits between degraded and healthy — no evidence is not an
// alarm, but it is not "good" either.
export const SEVERITY_RANK: Record<EndpointStatus, number> = {
  failing: 0,
  down: 1,
  degraded: 2,
  unverified: 3,
  healthy: 4,
}

// The same severity caliber as an ordered LIST (GH #55): consumed wherever
// a status sequence is rendered — the Dashboard stats strip order and the
// group header status-count order. Both used to declare their own orderings
// (strip mild→severe, header its own priority list); they now share this
// single source so the board has one severity caliber, heavy→light. The
// test suite asserts this array stays consistent with SEVERITY_RANK.
export const SEVERITY_ORDER: EndpointStatus[] = ['failing', 'down', 'degraded', 'unverified', 'healthy']

// Rank of a disabled endpoint — and of a group with no enabled entries:
// sinks below every enabled rank. A disabled endpoint is out of service by
// admin choice; it must never compete for the first screen, and a disabled
// down endpoint must never lift its group's rank.
//
// (GH #136: the model list's availability ordering moved to
// utils/modelList.ts sortListEntries — multi-key, direction-aware. The
// retired GH #131「weakest-first」fixed ordering became the rate-asc branch
// there; this module keeps the severity rank table both consume.)
export const DISABLED_RANK = 5

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
