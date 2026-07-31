<template>
  <el-card shadow="never" class="progress-card">
    <!-- Card top: the grid/scores view switch plus the batch-level summary
         (progress bar + done count) above the grid, same card (ui-guidelines
         §5 EvalProgressGrid registration). Read-only mode (shared report
         page, ticket 54) hides the switch: the grid is the only in-flight
         view over there. -->
    <div class="card-top">
      <el-radio-group
        v-if="!readonly"
        :model-value="view"
        size="small"
        @update:model-value="emit('update:view', $event as EvalBoardView)"
      >
        <el-radio-button value="grid">进度网格</el-radio-button>
        <el-radio-button value="scores">实时分数</el-radio-button>
      </el-radio-group>
      <span class="batch-note">
        批次{{ statusWord }}:已完成 {{ report.progress.done + report.progress.failed }}/{{ report.progress.total }} 个评估运行
      </span>
      <!-- Batch cost summary (GH #42, console-only): judging time and
           wall-clock side by side plus the token split; accumulates with
           polling while in flight, terminal once settled. Neutral secondary
           text, never band-colored (cost is not a quality metric). The
           shared read-only view never renders it. -->
      <span v-if="!readonly && costSummary" class="batch-note">{{ costSummary }}</span>
    </div>
    <el-progress
      :percentage="progressPercent"
      :status="report.progress.failed > 0 ? 'exception' : undefined"
      class="batch-progress"
    />

    <!-- Empty state: no model has recorded results yet (the first run is
         still on its first model). -->
    <el-empty
      v-if="report.rows.length === 0"
      description="暂无模型进度:评估运行尚未产生结果,请稍候"
    />

    <!-- Model x suite status matrix. First column pinned ~220px with
         truncation + title hover; suite columns share the remaining width
         equally; no horizontal scroll (ui-guidelines §4). Cells are pure
         display, never clickable. -->
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
        <span v-for="cell in row.cells" :key="cell.suite_key" class="grid-cell" :title="cellTitle(cell)">
          <span class="cell-status" :class="`cell-${cell.status}`">
            <span class="cell-dot" /><span v-if="!isNarrow" class="cell-word">{{ cellStatusWord(cell.status) }}</span>
          </span>
          <span v-if="showCoverage(cell)" class="cell-coverage">
            {{ cell.judged_cases }}/{{ cell.expected_cases }} 题
          </span>
        </span>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { CampaignReport, EvalBoardView, ReportCell, ReportCellStatus } from '@/api/types'
import { batchCostSummary } from '@/utils/evalCost'
import { cellCostText } from '@/utils/scoreTier'
import { useBreakpoint } from '@/composables/useBreakpoint'

// EvalProgressGrid is the single batch-progress matrix component
// (ui-guidelines §5): one row per model, one column per suite, four-state
// cells colored by the batch/run status color mapping (§3 — success green
// for done, danger red for failed, brand blue for running, placeholder grey
// for pending; no flashing, no warning yellow). It is the default view of an
// unfinished batch; the parent feeds the report and owns polling. Read-only
// mode (ticket 54, shared report page) hides the grid/scores view switch:
// the shared boundary publishes progress metadata only, so there is no live
// board to switch to. GH #94: narrow viewport (<= 767px) shrinks the model
// column to 96px and omits the cell status word (the 8px dot stays, and the
// tooltip carries the full info).
const props = withDefaults(
  defineProps<{
    report: CampaignReport
    view: EvalBoardView
    readonly?: boolean
  }>(),
  { readonly: false },
)

const emit = defineEmits<{
  (e: 'update:view', view: EvalBoardView): void
}>()

// GH #94: Responsive breakpoint for narrow-viewport adaptations.
const { isNarrow } = useBreakpoint()
const progressPercent = computed(() => {
  const p = props.report.progress
  if (!p || p.total === 0) return 0
  return Math.round(((p.done + p.failed) / p.total) * 100)
})

// Batch/campaign status vocabulary (ui-guidelines §7), never mixed with the
// endpoint status words.
const statusWord = computed(() => (props.report.status === 'pending' ? '等待中' : '运行中'))

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
// Y"); a waiting cell has no meaningful coverage yet. The shared read-only
// view never shows cost (the shared payload omits the fields anyway; the
// readonly gate is the explicit boundary).
function cellTitle(cell: ReportCell): string {
  const word = cellStatusWord(cell.status)
  if (!showCoverage(cell)) return word
  const base = `${word} · ${cell.judged_cases}/${cell.expected_cases} 题`
  const cost = props.readonly ? '' : cellCostText(cell)
  return cost === '' ? base : `${base} · ${cost}`
}

// Batch cost summary (GH #42): judging time and wall-clock side by side
// (main ruling 2026-07-29) plus the token split. Empty when the payload
// carries no cost (shared/public surfaces).
const costSummary = computed(() => {
  if (!props.report.cost) return ''
  return batchCostSummary(props.report.cost, props.report.started_at, props.report.finished_at, Date.now())
})
</script>

<style scoped>
/* Consumption-page density: 16px card padding via the variable
   (ui-guidelines §2). */
.progress-card {
  --el-card-padding: 16px;
  margin-bottom: 16px;
}
.card-top {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.batch-note {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.batch-progress {
  margin-bottom: 16px;
}
.grid {
  display: flex;
  flex-direction: column;
  gap: 4px;
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
/* GH #94: Narrow viewport shrinks the model column to 96px (the truncation +
   title hover already existed). */
@media (max-width: 767px) {
  .grid-model {
    width: 96px;
  }
}
.grid-cell {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: baseline;
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
.cell-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--hs-text-sm);
  white-space: nowrap;
}
.cell-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}
/* Four-state batch/run status color mapping (ui-guidelines §3). */
.cell-done {
  color: var(--hs-success);
}
.cell-failed {
  color: var(--hs-danger);
}
.cell-running {
  color: var(--hs-brand);
}
.cell-pending {
  color: var(--hs-text-placeholder);
}
.cell-coverage {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
</style>
