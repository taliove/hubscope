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
import type { CampaignReport, CampaignStatus, ReportCell, ReportSuite } from '@/api/types'
import { baselineChipText, campaignTriggerLabel, failedBatchWarning, incompleteWatermark } from '@/utils/evalWording'

// Cap on leaderboard rows rendered into the card; the rest collapse into a
// single overflow line (same shape as the StatusCard detail overflow).
export const EVAL_CARD_MAX_ROWS = 20

export interface EvalCardChip {
  label: string
  value: string
}

export interface EvalCardRow {
  // Positional rank; null for judged-incomplete rows (ticket 92, spec 0014
  // decision A) — they render the placeholder dash like the page and never
  // take the top-3 rail.
  rank: number | null
  modelId: string
  suiteScores: Record<string, number | null>
  cells: ReportCell[]
  // The matrix total column: always the (weighted) total score.
  score: number | null
  // Total delta vs the baseline; only rendered when showDeltaColumn holds.
  delta: number | null
  // The judged-incomplete watermark text ('' for complete rows): precomputed
  // here so the static card stays a dumb renderer (no tooltip fallback —
  // the watermark must be fully visible inside the material).
  incompleteNote: string
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

// Trigger wording comes from the shared campaignTriggerLabel (evalWording,
// GH #57) — the card chip and the board meta line must never drift.

function suiteName(suites: ReportSuite[], key: string): string {
  return suites.find((s) => s.key === key)?.name ?? key
}

// Scope chips in fixed order: 批次 → 厂商 → 排序 → 涨跌基准 (spec 0009; 厂商
// unifies the family wording, GH #122 ruling). The
// batch chip is always present and neutral (the failed emphasis is carried
// by the warning line, not a colored chip). The matrix has no dimension
// view, so the dimension chip and the sort/dimension dedup are gone; the
// baseline chip is always listed when a baseline exists.
function buildChips(report: CampaignReport, query: { family?: string; sort: string }): EvalCardChip[] {
  const chips: EvalCardChip[] = [
    {
      label: '批次',
      value: `#${report.id} · ${campaignTriggerLabel(report.trigger)} · ${STATUS_LABELS[report.status]}`,
    },
  ]
  if (query.family) chips.push({ label: '厂商', value: query.family })
  if (query.sort !== 'total') {
    chips.push({ label: '排序', value: suiteName(report.suites, query.sort) })
  }
  const baseline = baselineChipText(report.baseline)
  if (baseline !== null) chips.push({ label: '涨跌基准', value: baseline })
  return chips
}

// Build the frozen snapshot from the currently displayed report response and
// the toolbar query (family/sort). The rows arrive already filtered and
// ranked by the server, so ranks are positional — and the coverage gate
// (spec 0014) already sank judged-incomplete rows, so a positional rank is
// only ever assigned to a complete row; incomplete rows get rank null and
// their precomputed watermark.
export function buildEvalCardSnapshot(
  report: CampaignReport,
  query: { family?: string; sort: string },
): EvalCardSnapshot {
  const failedWarning = report.status === 'failed' ? failedBatchWarning(report.progress.failed) : null
  const rows: EvalCardRow[] = report.rows.slice(0, EVAL_CARD_MAX_ROWS).map((row, index) => ({
    rank: row.complete === false ? null : index + 1,
    modelId: row.model_id,
    suiteScores: row.suite_scores,
    cells: row.cells,
    score: row.total_score,
    delta: row.total_delta,
    incompleteNote: incompleteWatermark(row),
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
