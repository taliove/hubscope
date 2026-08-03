import type { Campaign } from '@/api/types'

// Batch-selection helpers for /eval (issue #16). The sidebar batch progress
// entry (AppSidebar, moved from AppHeader in GH #112) deep-links to
// /eval?batch=<id> so the user lands on the running batch they were
// watching; a direct /eval visit keeps the established default of the
// newest done batch.

// Parse the ?batch= route query into a campaign id, rejecting anything
// that cannot name a batch (missing, non-numeric, fractional, non-positive).
export function parseBatchQuery(raw: string | (string | null)[] | null | undefined): number | null {
  const first = Array.isArray(raw) ? raw[0] : raw
  if (first === undefined || first === null || first === '') return null
  const n = Number(first)
  return Number.isInteger(n) && n > 0 ? n : null
}

// Pick the initially selected batch: the query-specified one when it
// exists in the list, otherwise the newest batch overall (2026-08-03 user
// ruling — a freshly triggered running batch leads, so the page follows
// the action; the list is newest-first), falling back to the newest done
// batch only when the newest entry somehow cannot be shown.
export function resolveInitialBatchId(
  campaigns: ReadonlyArray<Pick<Campaign, 'id' | 'status'>>,
  requestedId: number | null,
): number | null {
  if (requestedId !== null && campaigns.some((c) => c.id === requestedId)) return requestedId
  return (campaigns[0] ?? campaigns.find((c) => c.status === 'done'))?.id ?? null
}
