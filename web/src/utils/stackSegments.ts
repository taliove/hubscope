// Pure functions behind the ScoreStackBar stacked bar (ticket 75, spec 0007).
// The segment model is render-agnostic plain data — the page bar and the
// ticket-76 static EvalCard both consume buildStackSegments output directly,
// so nothing render-time-only may hide in the component.
//
// Correctness invariant (anti-fake self-consistency): a segment's width is
// score_i x (weight_i / Σw_scored) on the total-score scale (full track =
// 100 points), where Σw_scored sums only the weights of SCORED suites — the
// same normalization as the backend totalScore
// (internal/server/report_scoring.go: unscored suites drop out of both
// numerator and denominator). Summing the segment widths therefore equals
// the total score under any coverage state; there is no second caliber.
import type { ReportCell, ReportSuite } from '@/api/types'
import { formatScore } from '@/utils/format'

// In-segment score label threshold (ui-guidelines §5 ScoreStackBar entry):
// tentative 44px, calibrated against the 15-model production board before
// being finalized in the guideline. The watermark needs its "·8/10" suffix
// on top of the score, so it asks for extra room.
export const LABEL_MIN_PX = 44
export const WATERMARK_EXTRA_PX = 30

export type ScoreBand = 'success' | 'warning' | 'danger'

export interface StackSegment {
  key: string
  name: string
  score: number // 0-100 scale, never null (null suites produce no segment)
  weight: number // effective weight: missing or <=0 falls back to 1 (backend parity)
  widthPct: number // percent of the track, on the total-score scale
  band: ScoreBand
  label: string // formatScore(score)
  showLabel: boolean // segment pixel width >= LABEL_MIN_PX
  watermark: string // '·8/10' for a done suite with incomplete coverage, else ''
  showWatermark: boolean // watermark fits next to the label
  tooltip: string // '{name} · {score} · 判分 X/Y 题 · 采样 N 次'
}

// Live-mode annotation counts (spec 0007): suites still waiting/running and
// failed runs, derived from the report cells. Rendered as grey text right of
// the bar; never occupies bar width, never feeds the total.
export interface LiveCounts {
  inFlight: number
  failed: number
}

// Weight fallback mirrors the backend: a missing key or a non-positive
// weight counts as 1 (internal/server/report_scoring.go totalScore).
export function effectiveWeight(weights: Record<string, number>, key: string): number {
  const w = weights[key]
  return w > 0 ? w : 1
}

// Score-band mapping (ui-guidelines §3): green at 80+, yellow at 50+, red
// below — the same bands as the score badge.
export function scoreBand(score: number): ScoreBand {
  if (score >= 80) return 'success'
  if (score >= 50) return 'warning'
  return 'danger'
}

// Coverage watermark: a done suite that judged fewer cases than expected
// carries the compressed '·8/10' form; full coverage (and non-done cells,
// whose status the live annotation carries) shows nothing.
function watermarkOf(cell: ReportCell | undefined): string {
  if (!cell || cell.status !== 'done') return ''
  if (cell.expected_cases <= 0 || cell.judged_cases >= cell.expected_cases) return ''
  return `·${cell.judged_cases}/${cell.expected_cases}`
}

// Confidence tooltip (ticket 51 caliber): suite name, score, judged-case
// coverage and the number of judged answer attempts. A scored suite always
// has judged cases, but the shape degrades gracefully if the cell is absent.
function tooltipOf(name: string, score: number, cell: ReportCell | undefined): string {
  const head = `${name} · ${formatScore(score)}`
  if (!cell || cell.judged_cases <= 0) return head
  return `${head} · 判分 ${cell.judged_cases}/${cell.expected_cases} 题 · 采样 ${cell.samples} 次`
}

// Build the stacked segments for one board row: one segment per SCORED
// suite, in report.suites order, stacked from the left; unscored suites
// (null) are zero-width and the gap stays on the right. trackWidthPx turns
// the width thresholds into show flags — the component measures its track,
// the static card passes its known fixed width.
export function buildStackSegments(
  suites: ReportSuite[],
  weights: Record<string, number>,
  suiteScores: Record<string, number | null>,
  cells: ReportCell[],
  trackWidthPx: number,
): StackSegment[] {
  const scored = suites.filter((s) => suiteScores[s.key] !== null && suiteScores[s.key] !== undefined)
  const wsum = scored.reduce((acc, s) => acc + effectiveWeight(weights, s.key), 0)
  if (wsum <= 0) return []
  return scored.map((s) => {
    const score = suiteScores[s.key] as number
    const weight = effectiveWeight(weights, s.key)
    const widthPct = (score * weight) / wsum
    const widthPx = (widthPct / 100) * trackWidthPx
    const cell = cells.find((c) => c.suite_key === s.key)
    const watermark = watermarkOf(cell)
    const showLabel = widthPx >= LABEL_MIN_PX
    return {
      key: s.key,
      name: s.name,
      score,
      weight,
      widthPct,
      band: scoreBand(score),
      label: formatScore(score),
      showLabel,
      watermark,
      showWatermark: showLabel && watermark !== '' && widthPx >= LABEL_MIN_PX + WATERMARK_EXTRA_PX,
      tooltip: tooltipOf(s.name, score, cell),
    }
  })
}

// Live counts for the right-of-bar annotation: pending/running cells read as
// "in flight", failed cells as failures (batch/run status vocabulary,
// ui-guidelines §7).
export function liveCounts(cells: ReportCell[]): LiveCounts {
  let inFlight = 0
  let failed = 0
  for (const c of cells) {
    if (c.status === 'pending' || c.status === 'running') inFlight += 1
    else if (c.status === 'failed') failed += 1
  }
  return { inFlight, failed }
}
