<template>
  <!-- EvalProgressGrid is the batch-progress matrix (2026-08-03 redesign,
       ui-guidelines §5): one row per model, one column per suite, each cell
       a single mini progress bar — color carries the four-state status
       (success = done full bar, danger = failed, brand = running filling
       with judged coverage, empty track = pending), width carries the
       judged/expected coverage. Status words and dots are retired; the
       tooltip (and aria-label) always carries the full caliber: status
       word + X/Y 题 + console cost. The batch-level header (view switch,
       progress bar, cost) lives in the parent's EvalBoardHeader. Readonly
       mode (shared report page) only gates the cost fragment out of the
       tooltip. GH #94: narrow viewport (<= 1023px) shrinks the model
       column to 96px and omits the inline count (the bar stays, the
       tooltip carries the numbers). -->
  <div class="progress-panel">
    <!-- Empty state: no model has recorded results yet (the first run is
         still on its first model). -->
    <el-empty
      v-if="report.rows.length === 0"
      description="暂无模型进度:评估运行尚未产生结果,请稍候"
    />

    <!-- Model x suite matrix. First column pinned ~220px with truncation +
         title hover; suite columns share the remaining width equally; no
         horizontal scroll (ui-guidelines §4). -->
    <div v-else class="grid">
      <div class="grid-row grid-head">
        <span class="grid-model">模型</span>
        <span
          v-for="s in report.suites"
          :key="s.key"
          class="grid-cell grid-head-cell"
          :title="s.name"
        >
          {{ s.name }}
        </span>
      </div>
      <div v-for="row in report.rows" :key="row.model_db_id" class="grid-row">
        <span class="grid-model" :title="row.model_id">{{ row.model_id }}</span>
        <span
          v-for="cell in row.cells"
          :key="cell.suite_key"
          class="grid-cell"
          :title="cellTitle(cell)"
          :aria-label="cellTitle(cell)"
        >
          <span class="cell-track">
            <span
              v-if="cell.status !== 'pending'"
              class="cell-fill"
              :class="`fill-${cell.status}`"
              :style="{ width: fillWidth(cell) }"
            />
          </span>
          <span v-if="showInlineCount(cell) && !isNarrow" class="cell-count">
            {{ cell.judged_cases }}/{{ cell.expected_cases }}
          </span>
        </span>
        <!-- Per-model ETA (2026-08-03): the model's remaining suite time at
             its own pace; console only (cost class data never crosses the
             share boundary). -->
        <span v-if="!readonly && rowEta(row)" class="row-eta" title="按当前速度的预估剩余时间">
          ≈{{ rowEta(row) }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { CampaignReport, ReportCell, ReportCellStatus, ReportRow } from '@/api/types'
import { cellCostText } from '@/utils/scoreTier'
import { avgUnitMs, cellRemainingMs, rowRemainingMs } from '@/utils/batchEta'
import { formatDuration } from '@/utils/format'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { computed } from 'vue'

// Props seam: the report only; view switching and batch meta moved to
// EvalBoardHeader. readonly (shared report page) gates cost out of the
// tooltip — the shared payload omits the fields anyway, the gate is the
// explicit boundary.
const props = withDefaults(
  defineProps<{
    report: CampaignReport
    readonly?: boolean
  }>(),
  { readonly: false },
)

// GH #94: responsive breakpoint for narrow-viewport adaptations.
const { isNarrow } = useBreakpoint()

// Cell fill width: coverage drives running/failed bars; a failed cell with
// zero judged cases keeps a 2px minimum visible segment so it never reads
// as a pending cell (the tooltip always reports the true numbers). A
// running cell with expected_cases 0 stays an empty track — null never
// impersonates progress.
function fillWidth(cell: ReportCell): string {
  if (cell.status === 'done') return '100%'
  if (cell.expected_cases <= 0) return cell.status === 'failed' ? '2px' : '0%'
  const ratio = (cell.judged_cases / cell.expected_cases) * 100
  if (cell.status === 'failed') return `max(2px, ${ratio}%)`
  return `${ratio}%`
}

// Inline judged/expected count: only cells with recorded progress (running
// or failed); done cells are full bars, pending cells empty tracks.
function showInlineCount(cell: ReportCell): boolean {
  return (cell.status === 'running' || cell.status === 'failed') && cell.expected_cases > 0
}

// Batch/run status vocabulary (ui-guidelines §7) — the words no longer
// render in the cells; the tooltip is their only standing carrier.
function cellStatusWord(status: ReportCellStatus): string {
  switch (status) {
    case 'done':
      return '已完成'
    case 'failed':
      return '失败'
    case 'running':
      return '运行中'
    default:
      return '等待中'
  }
}

// Coverage shows on cells with recorded progress; a waiting cell has no
// meaningful X/Y yet.
function showCoverage(cell: ReportCell): boolean {
  return cell.status !== 'pending' && cell.expected_cases > 0
}

// Cell tooltip: the status word plus the judged-case coverage ("运行中 ·
// 2/12 题") and, on the console, the GH #42 cost fragment ("· 耗时 X · Token
// Y") and the ETA fragment ("· 预估剩余 Z"); a waiting cell has no
// meaningful coverage yet. The shared read-only view never shows cost or
// ETA (the shared payload omits the latency fields anyway; the readonly
// gate is the explicit boundary).
function cellTitle(cell: ReportCell): string {
  const word = cellStatusWord(cell.status)
  if (!showCoverage(cell)) return word
  const base = `${word} · ${cell.judged_cases}/${cell.expected_cases} 题`
  if (props.readonly) return base
  const cost = cellCostText(cell)
  const eta = cellRemainingMs(cell, fallbackPace.value)
  const etaText = eta === null ? '' : ` · 预估剩余 ${formatDuration(eta)}`
  return (cost === '' ? base : `${base} · ${cost}`) + etaText
}

// Per-cell pace fallback (campaign-wide average unit latency), computed
// once per report refresh.
const fallbackPace = computed(() => avgUnitMs(props.report))

// Per-model ETA text (console only): the model's remaining suite time.
function rowEta(row: ReportRow): string {
  const remaining = rowRemainingMs(row, fallbackPace.value)
  return remaining === null ? '' : formatDuration(remaining)
}
</script>

<style scoped>
/* Light container (GH #120, v2 Apple syntax): white surface, 1px border,
   radius-lg, no shadow; the inner padding matches the Leaderboard's. */
.progress-panel {
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
  padding: var(--hs-space-5) var(--hs-space-6);
  margin-bottom: var(--hs-space-4);
}
@media (max-width: 1023px) {
  .progress-panel {
    padding: var(--hs-space-4);
  }
}
.grid {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.grid-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.grid-model {
  width: 220px;
  flex-shrink: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* GH #94: narrow viewport shrinks the model column to 96px (the truncation +
   title hover already existed). */
@media (max-width: 1023px) {
  .grid-model {
    width: 96px;
  }
}
.grid-cell {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  overflow: hidden;
}
.grid-head-cell {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}
/* Mini progress bar (2026-08-03 design review): 6px full-radius track on
   the neutral hover-ground (the ScoreCell track's token), fill color =
   four-state status on the §3.2 graphic bases. Width growth transitions at
   the feedback tier — one-directional fact growth, not flashing (the batch
   el-progress's 0.6s ease is the registered precedent; the global
   reduced-motion rule zeroes it). No stripes, no pulse, no warning yellow. */
.cell-track {
  flex: 1;
  min-width: 0;
  height: 6px;
  border-radius: var(--hs-radius-full);
  background: var(--hs-bg-hover);
  overflow: hidden;
}
.cell-fill {
  display: block;
  height: 100%;
  border-radius: var(--hs-radius-full);
  transition: width var(--hs-transition);
}
.fill-done {
  background: var(--hs-success);
}
.fill-failed {
  background: var(--hs-danger);
}
.fill-running {
  background: var(--hs-brand);
}
.cell-count {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
.row-eta {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  white-space: nowrap;
}
</style>
