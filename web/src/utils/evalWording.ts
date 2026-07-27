// Shared eval-board wording (ticket 76 check follow-up): the failed-batch
// warning and the baseline comparison note each reached the "second
// occurrence" threshold across consumers, so the verbatim strings live here
// to prevent drift. Consumers: EvalLeaderboardView / CampaignReportView
// (failed alert title) and evalCardSnapshot (card warning line) for the
// warning; Leaderboard (toolbar baseline note) and evalCardSnapshot (card
// baseline chip) for the baseline note. Wordings NOT shared on purpose:
// the settle-transition ElMessage ("批次 #N 已结束:…") and the delta-arrow
// hover titles use different sentence shapes and stay local.
import type { ReportBaseline } from '@/api/types'

// Failed-batch warning, identical to the page alert title (ui-guidelines §5
// EvalCard entry: the card line carries the same wording and caliber).
export function failedBatchWarning(failed: number): string {
  return `批次有 ${failed} 个评估运行失败,榜单仅统计已完成的评估集`
}

// Toolbar baseline note (Leaderboard): names the comparison batch, or why
// the comparison is impossible (ADR 0007 question-bank break / ADR 0008
// scoring-caliber break). '' when no earlier done batch exists.
export function baselineNoteText(baseline: ReportBaseline | null): string {
  if (!baseline) return ''
  if (baseline.comparable) return `涨跌较批次 #${baseline.campaign_id}`
  if (baseline.reason === 'suite_changed') return `较批次 #${baseline.campaign_id}:题目已变更,分数不可比`
  if (baseline.reason === 'profile_changed') return `较批次 #${baseline.campaign_id}:判分口径已变更,分数不可比`
  return `较批次 #${baseline.campaign_id}:考核口径不同,分数不可比`
}

// EvalCard baseline chip value: the incomparable branches are identical to
// the page note; the comparable value drops the 涨跌 prefix because the chip
// label already reads 涨跌基准 (ticket 76 design-review registration).
// null when there is no baseline — the chip is omitted then.
export function baselineChipText(baseline: ReportBaseline | null): string | null {
  if (!baseline) return null
  if (baseline.comparable) return `较批次 #${baseline.campaign_id}`
  return baselineNoteText(baseline)
}
