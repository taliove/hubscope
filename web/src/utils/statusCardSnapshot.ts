// Snapshot handed to the StatusCard when the share dialog opens. Frozen at
// open time so polling refreshes cannot swap the data between preview and
// export — what the user sees is exactly what gets shared (ticket 49).
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'

export interface StatusCardSnapshot {
  entries: OverviewEntry[] // filtered set, disabled endpoints included
  keyword: string
  protocol: Protocol | ''
  status: EndpointStatus | ''
  generatedAt: string // ISO timestamp of the open/generation moment
}
