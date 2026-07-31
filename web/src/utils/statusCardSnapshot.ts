// Snapshot handed to the StatusCard when the share dialog opens. Frozen at
// open time so polling refreshes cannot swap the data between preview and
// export — what the user sees is exactly what gets shared (ticket 56).
// `group` marks a per-group share (ticket 59): the card leads its scope
// chips with it so a group subset never reads as the global picture.
// Ticket 60.5: single-model snapshots include hubName and evalSummary.
import type { OverviewEntry, Protocol, ModelEvalSummary } from '@/api/types'
import type { DisplayStatus } from '@/utils/statusDisplay'

// Grouping dimension of a per-group share (identical to the Dashboard
// grouping selector values).
export type GroupDimension = 'family' | 'capability' | 'protocol'

export interface StatusCardSnapshot {
  entries: OverviewEntry[] // scoped set, disabled endpoints included
  keyword: string
  protocol: Protocol | ''
  // The Dashboard status filter speaks display states (GH #113): the scope
  // chip names 稳定/性能下降/服务异常 and the filter matches down+failing
  // together under incident.
  status: DisplayStatus | ''
  group: { dimension: GroupDimension; key: string } | null
  generatedAt: string // ISO timestamp of the open/generation moment
  // Single-model specific fields (only populated when entries.length === 1).
  // hubName doubles as the single-model marker: StatusCard renders the
  // single-model layout only when entries.length === 1 && hubName is set, so
  // a filter that narrows the board to one endpoint keeps the aggregate
  // layout (design ruling).
  hubName?: string
  evalSummary?: ModelEvalSummary | null
}

// Create a single-model snapshot (ticket 60.5): entries array contains exactly
// one item, all filter fields are empty (no scope concept for single model).
export function createSingleModelSnapshot(
  entry: OverviewEntry,
  hubName: string,
  evalSummary: ModelEvalSummary | null,
  generatedAt: string
): StatusCardSnapshot {
  return {
    entries: [entry],
    keyword: '',
    protocol: '',
    status: '',
    group: null,
    generatedAt,
    hubName,
    evalSummary,
  }
}
