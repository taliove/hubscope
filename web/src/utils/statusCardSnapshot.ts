// Snapshot handed to the StatusCard when the share dialog opens. Frozen at
// open time so polling refreshes cannot swap the data between preview and
// export — what the user sees is exactly what gets shared (ticket 56).
// `group` marks a per-group share (ticket 59): the card leads its scope
// chips with it so a group subset never reads as the global picture.
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'

// Grouping dimension of a per-group share (identical to the Dashboard
// grouping selector values).
export type GroupDimension = 'family' | 'capability' | 'protocol'

export interface StatusCardSnapshot {
  entries: OverviewEntry[] // scoped set, disabled endpoints included
  keyword: string
  protocol: Protocol | ''
  status: EndpointStatus | ''
  group: { dimension: GroupDimension; key: string } | null
  generatedAt: string // ISO timestamp of the open/generation moment
}
