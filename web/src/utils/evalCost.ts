// Pure logic for the GH #42 cost metrics: the batch wall-clock derivation
// and the batch summary line shared by the progress-grid card top and the
// report page's cost detail table title. Formatting goes through
// utils/format (ui-guidelines §7 centralization); cost numbers are neutral
// metrics and never pick up band colors (ui-guidelines §5 成本指标条).
import type { CampaignCost } from '@/api/types'
import { formatDuration, formatTokens } from '@/utils/format'

// Batch wall-clock in milliseconds: finished_at - started_at on a settled
// batch, now - started_at while in flight. Null when the batch never
// recorded a start (the summary renders a dash).
export function wallClockMs(startedAt: string | null, finishedAt: string | null, nowMs: number): number | null {
  if (!startedAt) return null
  const start = new Date(startedAt).getTime()
  if (Number.isNaN(start)) return null
  const end = finishedAt ? new Date(finishedAt).getTime() : nowMs
  if (Number.isNaN(end)) return null
  return Math.max(0, end - start)
}

// The batch cost summary line (main ruling 2026-07-29: judging time and
// wall-clock side by side): "判分耗时 X · 批次用时 Y · Token T(输入 I / 输出
// O)". The token total splits into input/output per the registration.
export function batchCostSummary(
  cost: CampaignCost,
  startedAt: string | null,
  finishedAt: string | null,
  nowMs: number,
): string {
  const judging = formatDuration(cost.latency_ms)
  const wall = formatDuration(wallClockMs(startedAt, finishedAt, nowMs))
  const total = cost.input_tokens + cost.output_tokens
  return `判分耗时 ${judging} · 批次用时 ${wall} · Token ${formatTokens(total)}(输入 ${formatTokens(cost.input_tokens)} / 输出 ${formatTokens(cost.output_tokens)})`
}
