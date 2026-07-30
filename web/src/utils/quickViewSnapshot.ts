// Snapshot freeze for the endpoint quick-view dialog (2026-07-29, dashboard
// surface brief 速览弹窗快照冻结条): the dialog renders the entry frozen at
// open time — polling must never update it (same philosophy as the StatusCard
// / EvalShareDialog snapshots; live data lives behind 打开完整详情).
import type { OverviewEntry } from '@/api/types'

// Deep-copies the entry so later in-place mutations of the polled object
// (defense in depth — useOverview normally replaces objects outright) cannot
// leak into the open dialog. Nested arrays (dots, causes, score reasons) are
// the mutable surface; everything else is scalar.
export function freezeEntrySnapshot(entry: OverviewEntry): OverviewEntry {
  return {
    ...entry,
    degrade_causes: [...entry.degrade_causes],
    score_reasons: [...entry.score_reasons],
    dots_24h: entry.dots_24h.map(dot => ({ ...dot })),
  }
}
