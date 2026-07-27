// Pure presentation logic for eval-board score cells (ticket 78, spec 0009):
// the score band, coverage watermark, confidence tooltip, live-cell status
// wording and live counts. Single source of truth for the matrix
// Leaderboard's ScoreCell on both ends (page and static EvalCard).
//
// The band thresholds (>=80 success / >=50 warning / below danger) are the
// business caliber shared with the backend score badge and never move
// (ui-guidelines §3); the absolute 0-100 scale is the visual mirror of the
// W7 absolute-score system.
import type { ReportCell } from '@/api/types'
import { formatScore } from '@/utils/format'

export type ScoreBand = 'success' | 'warning' | 'danger'

// Score-band mapping (ui-guidelines §3): green at 80+, yellow at 50+, red
// below — the same bands as the score badge.
export function scoreBand(score: number): ScoreBand {
  if (score >= 80) return 'success'
  if (score >= 50) return 'warning'
  return 'danger'
}

// Coverage watermark (anti-fake, ticket 51 caliber): a done suite that judged
// fewer cases than expected carries the compressed '·8/10' form; full
// coverage (and non-done cells, whose status the live annotation carries)
// shows nothing.
export function watermarkOf(cell: ReportCell | undefined): string {
  if (!cell || cell.status !== 'done') return ''
  if (cell.expected_cases <= 0 || cell.judged_cases >= cell.expected_cases) return ''
  return `·${cell.judged_cases}/${cell.expected_cases}`
}

// Confidence tooltip (ticket 51 caliber): suite name, score, judged-case
// coverage and the number of judged answer attempts. A scored suite always
// has judged cases, but the shape degrades gracefully if the cell is absent.
export function tooltipOf(name: string, score: number, cell: ReportCell | undefined): string {
  const head = `${name} · ${formatScore(score)}`
  if (!cell || cell.judged_cases <= 0) return head
  return `${head} · 判分 ${cell.judged_cases}/${cell.expected_cases} 题 · 采样 ${cell.samples} 次`
}

// Live-cell status wording for unscored cells (spec 0009 live section): the
// batch/run vocabulary of ui-guidelines §7, never the endpoint words. A done
// cell without a score should not happen; it falls back to a neutral "未判分".
export function cellStatusText(cell: ReportCell | undefined): string {
  if (!cell) return '未判分'
  if (cell.status === 'pending') return '等待中'
  if (cell.status === 'running') return '进行中'
  if (cell.status === 'failed') return '失败'
  return '未判分'
}

// Live-mode annotation counts (spec 0007/0009): suites still waiting/running
// and failed runs, derived from the report cells. Rendered as grey text at
// the row end; never feeds the total.
export interface LiveCounts {
  inFlight: number
  failed: number
}

export function liveCounts(cells: ReportCell[]): LiveCounts {
  let inFlight = 0
  let failed = 0
  for (const c of cells) {
    if (c.status === 'pending' || c.status === 'running') inFlight += 1
    else if (c.status === 'failed') failed += 1
  }
  return { inFlight, failed }
}
