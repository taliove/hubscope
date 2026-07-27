// Snapshot handed to the EvalCard when the share dialog opens (ticket 76,
// revised to the matrix board by ticket 79 / spec 0009). Frozen at open
// time so a report refresh cannot swap the data between preview and export
// — what the user sees is exactly what gets shared (same freeze discipline
// as statusCardSnapshot, ticket 56).
//
// Anti-fake invariant (ui-guidelines §5「结论必须标注统计范围」ticket 76
// 补充): the scope chips describe exactly the batch + family filter + sort +
// baseline the card's numbers come from — every number is read from the
// same report response, never from any other page aggregate. An empty
// filtered result keeps all chips and shows a neutral "暂无匹配模型", never
// a "全部上榜"-style conclusion.
import type { CampaignReport, CampaignStatus, EvalTrigger, ReportCell, ReportSuite } from '@/api/types'
import { baselineChipText, failedBatchWarning } from '@/utils/evalWording'

// Cap on leaderboard rows rendered into the card; the rest collapse into a
// single overflow line (same shape as the StatusCard detail overflow).
export const EVAL_CARD_MAX_ROWS = 20

export interface EvalCardChip {
  label: string
  value: string
}

export interface EvalCardRow {
  rank: number
  modelId: string
  suiteScores: Record<string, number | null>
  cells: ReportCell[]
  // The matrix total column: always the (weighted) total score.
  score: number | null
  // Total delta vs the baseline; only rendered when showDeltaColumn holds.
  delta: number | null
}

export interface EvalCardSnapshot {
  campaignId: number
  suites: ReportSuite[]
  chips: EvalCardChip[]
  // Page-alert wording for a failed batch, null otherwise (EvalCard renders
  // the warning line only when present).
  failedWarning: string | null
  // The delta column renders only with a comparable baseline; otherwise the
  // whole column is omitted (no placeholder column).
  showDeltaColumn: boolean
  rows: EvalCardRow[]
  overflowCount: number
  generatedAt: string // ISO timestamp of the open/generation moment
}

// Batch/run status vocabulary (ui-guidelines §7), never the endpoint words.
const STATUS_LABELS: Record<CampaignStatus, string> = {
  done: '已完成',
  failed: '失败',
  running: '运行中',
  pending: '等待中',
}

const TRIGGER_LABELS: Record<EvalTrigger, string> = {
  scheduled: '定时',
  manual: '手动',
}

function suiteName(suites: ReportSuite[], key: string): string {
  return suites.find((s) => s.key === key)?.name ?? key
}

// Scope chips in fixed order: 批次 → 系列 → 排序 → 涨跌基准 (spec 0009). The
// batch chip is always present and neutral (the failed emphasis is carried
// by the warning line, not a colored chip). The matrix has no dimension
// view, so the dimension chip and the sort/dimension dedup are gone; the
// baseline chip is always listed when a baseline exists.
function buildChips(report: CampaignReport, query: { family?: string; sort: string }): EvalCardChip[] {
  const chips: EvalCardChip[] = [
    {
      label: '批次',
      value: `#${report.id} · ${TRIGGER_LABELS[report.trigger]} · ${STATUS_LABELS[report.status]}`,
    },
  ]
  if (query.family) chips.push({ label: '系列', value: query.family })
  if (query.sort !== 'total') {
    chips.push({ label: '排序', value: suiteName(report.suites, query.sort) })
  }
  const baseline = baselineChipText(report.baseline)
  if (baseline !== null) chips.push({ label: '涨跌基准', value: baseline })
  return chips
}

// Build the frozen snapshot from the currently displayed report response and
// the toolbar query (family/sort). The rows arrive already filtered and
// ranked by the server, so ranks are positional.
export function buildEvalCardSnapshot(
  report: CampaignReport,
  query: { family?: string; sort: string },
): EvalCardSnapshot {
  const failedWarning = report.status === 'failed' ? failedBatchWarning(report.progress.failed) : null
  const rows: EvalCardRow[] = report.rows.slice(0, EVAL_CARD_MAX_ROWS).map((row, index) => ({
    rank: index + 1,
    modelId: row.model_id,
    suiteScores: row.suite_scores,
    cells: row.cells,
    score: row.total_score,
    delta: row.total_delta,
  }))
  return {
    campaignId: report.id,
    suites: report.suites,
    chips: buildChips(report, query),
    failedWarning,
    showDeltaColumn: Boolean(report.baseline?.comparable),
    rows,
    overflowCount: Math.max(0, report.rows.length - EVAL_CARD_MAX_ROWS),
    generatedAt: new Date().toISOString(),
  }
}
