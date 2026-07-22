<template>
  <el-card shadow="never" class="leaderboard-card">
    <!-- Toolbar: suite view switch, family filter, ranking column. Suite
         switching also re-ranks by that suite so bar lengths stay monotonic
         top to bottom (the board is a ranking). In live mode (unfinished
         batch, ticket 52) the grid/scores view switch takes the lead slot
         and both ranking controls are disabled — the half-scored board
         never re-ranks; family filtering stays available. -->
    <div class="toolbar">
      <el-radio-group
        v-if="live"
        :model-value="view"
        size="small"
        @update:model-value="emit('update:view', $event as EvalBoardView)"
      >
        <el-radio-button value="grid">进度网格</el-radio-button>
        <el-radio-button value="scores">实时分数</el-radio-button>
      </el-radio-group>
      <el-radio-group v-model="viewSuite" size="small" :disabled="live" @change="onViewSuiteChange">
        <el-radio-button value="total">总分</el-radio-button>
        <el-radio-button v-for="s in report.suites" :key="s.key" :value="s.key">
          {{ s.name }}
        </el-radio-button>
      </el-radio-group>
      <el-select
        v-model="family"
        size="small"
        clearable
        placeholder="全部系列"
        class="family-select"
        @change="emitQuery"
      >
        <el-option v-for="f in familyOptions" :key="f" :label="f" :value="f" />
      </el-select>
      <el-select v-model="sortKey" size="small" class="sort-select" :disabled="live" @change="emitQuery">
        <el-option label="按总分排序" value="total" />
        <el-option v-for="s in report.suites" :key="s.key" :label="`按${s.name}排序`" :value="s.key" />
      </el-select>
      <span v-if="!live && viewSuite === 'total' && baselineNote" class="baseline-note">{{ baselineNote }}</span>
    </div>

    <!-- Empty state: no model ranked (nothing scored, or filtered out). -->
    <el-empty v-if="report.rows.length === 0" :description="emptyDescription" />

    <!-- DesignArena-style horizontal bar leaderboard (ui-guidelines §5).
         Rows are clickable: a click emits select for the trend drill-down
         dialog (ticket 32, no inline row expansion per §4). Live mode keeps
         the row order the backend sent (model-id lexicographic) and swaps
         the rank slot for a placeholder dash. -->
    <div v-else class="rows">
      <div
        v-for="(row, index) in report.rows"
        :key="row.model_db_id"
        class="row clickable"
        role="button"
        tabindex="0"
        @click="emit('select', row)"
        @keydown.enter="emit('select', row)"
        @keydown.space.prevent="emit('select', row)"
      >
        <div class="row-main">
          <span class="rank" :class="{ 'rank-live': live }">{{ live ? '–' : index + 1 }}</span>
          <span class="model" :title="row.model_id">{{ row.model_id }}</span>
          <el-tag size="small" effect="plain" class="family-tag">{{ row.family }}</el-tag>
          <div class="bar-track">
            <div class="bar-fill" :style="{ width: barWidth(scoreOf(row)), background: barColor(scoreOf(row)) }" />
          </div>
          <span class="score">{{ formatScore(scoreOf(row)) }}</span>
          <span v-if="!live && viewSuite === 'total'" class="delta" :class="deltaTone(row)" :title="deltaTitle(row)">
            <template v-if="hasArrow(row)">{{ arrowOf(row) }} {{ formatScoreDelta(row.total_delta) }}</template>
            <template v-else>–</template>
          </span>
        </div>
        <!-- Per-suite dimension strip (ticket 51, reusing the ticket-52
             strip shape): every capability score side by side with the
             total. In live mode an unrun suite keeps an empty bar and a
             "进行中" score placeholder (never counted into the total); a
             scored suite shows its bar plus the coverage watermark when not
             every case was judged. Hovering an item reveals the confidence
             detail (coverage + judged samples). -->
        <div class="suite-strip">
          <span v-for="s in report.suites" :key="s.key" class="suite-item" :title="confidenceOf(row, s.key)">
            <span class="suite-name" :title="s.name">{{ s.name }}</span>
            <span class="suite-bar-track">
              <span class="suite-bar-fill" :style="suiteBarStyle(row, s.key)" />
            </span>
            <span class="suite-score">
              <template v-if="suiteScoreOf(row, s.key) !== null">{{ formatScore(suiteScoreOf(row, s.key)) }}</template>
              <template v-else-if="isSuiteFailed(row, s.key)">
                <span class="suite-failed">失败</span>
              </template>
              <template v-else-if="isSuiteInFlight(row, s.key)">
                <span class="suite-inflight">进行中</span>
              </template>
              <template v-else>-</template>
            </span>
            <span v-if="coverageOf(row, s.key)" class="coverage">{{ coverageOf(row, s.key) }}</span>
          </span>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { formatScore, formatScoreDelta } from '@/utils/format'
import type { CampaignReport, EvalBoardView, ReportCell, ReportRow } from '@/api/types'

// Leaderboard is the single ranking display of the eval board (registered in
// ui-guidelines §5): one row per model — rank, truncated name, band-colored
// bar, 0-100 score, and the total-score delta arrow versus the previous
// done campaign. Suite switching, family filtering and ranking column live
// in the toolbar, never inside cells. The parent fetches data; this
// component only re-emits query changes.
//
// Live mode (ticket 52, half-scored board of an unfinished batch): the rank
// slot shows a placeholder dash (no rank badges pre-settle), rows keep the
// backend's model-id lexicographic order, the sort select and suite switch
// are disabled, the delta column hides entirely, and unrun suites show a
// "进行中" placeholder in the strip.
//
// Dimension strip (ticket 51): every row carries the per-suite strip in both
// settled and live mode — the total never stands alone. Each score's
// confidence marker is the coverage watermark ("X/Y 题", only when coverage
// is incomplete) plus the judged-sample count on the hover tooltip.
const props = withDefaults(
  defineProps<{
    report: CampaignReport
    // Options come from the unfiltered board, so filtering never collapses the
    // option list itself.
    familyOptions: string[]
    live?: boolean
    view?: EvalBoardView
  }>(),
  { live: false, view: 'grid' },
)

const emit = defineEmits<{
  (e: 'query', query: { family?: string; sort: string }): void
  (e: 'select', row: ReportRow): void
  (e: 'update:view', view: EvalBoardView): void
}>()

const viewSuite = ref('total')
const family = ref('')
const sortKey = ref('total')

function onViewSuiteChange(value: string | number | boolean) {
  sortKey.value = String(value)
  emitQuery()
}

function emitQuery() {
  emit('query', { family: family.value || undefined, sort: sortKey.value })
}

// Live mode pins the main bar to the total: the suite switch is disabled
// and per-suite detail lives in the strip below each row.
function scoreOf(row: ReportRow): number | null {
  if (props.live) return row.total_score
  if (viewSuite.value === 'total') return row.total_score
  return row.suite_scores[viewSuite.value] ?? null
}

function barWidth(score: number | null): string {
  if (score === null) return '0%'
  return `${Math.min(100, Math.max(0, score))}%`
}

// Bar fill follows the score-band color mapping (ui-guidelines §3): green at
// 80+, yellow at 50+, red below — the same bands as the score badge.
function barColor(score: number | null): string {
  if (score === null) return 'transparent'
  if (score >= 80) return 'var(--el-color-success)'
  if (score >= 50) return 'var(--el-color-warning)'
  return 'var(--el-color-danger)'
}

// Empty-state copy distinguishes "nothing scored" from "filtered out" so a
// fully-failed batch never reads as "deleted models don't rank". Live mode
// points back to the progress grid (ui-guidelines §5).
const emptyDescription = computed(() => {
  if (family.value) return `系列 ${family.value} 下暂无上榜模型`
  if (props.live) return '暂无已判分模型,可在进度网格查看运行状态'
  if (props.report.status === 'failed') return '暂无上榜模型:评估运行全部失败'
  return '暂无上榜模型:已删除模型不上榜'
})

// --- Live-mode per-suite strip helpers --------------------------------

function cellOf(row: ReportRow, suiteKey: string): ReportCell | undefined {
  return row.cells.find((c) => c.suite_key === suiteKey)
}

function suiteScoreOf(row: ReportRow, suiteKey: string): number | null {
  return row.suite_scores[suiteKey] ?? null
}

// A suite is "in flight" only while its cell is waiting or running; a failed
// cell reads as "失败" (batch/run status vocabulary, ui-guidelines §7) with
// an empty bar, and only a done run without any judged case renders the bare
// dash.
function isSuiteInFlight(row: ReportRow, suiteKey: string): boolean {
  const cell = cellOf(row, suiteKey)
  return cell !== undefined && (cell.status === 'pending' || cell.status === 'running')
}

function isSuiteFailed(row: ReportRow, suiteKey: string): boolean {
  return cellOf(row, suiteKey)?.status === 'failed'
}

function suiteBarStyle(row: ReportRow, suiteKey: string): { width: string; background: string } {
  const score = suiteScoreOf(row, suiteKey)
  return { width: barWidth(score), background: barColor(score) }
}

// Coverage watermark (ui-guidelines §5): "X/Y 题" next to a done suite's
// score when not every case was judged; full coverage shows nothing.
function coverageOf(row: ReportRow, suiteKey: string): string {
  const cell = cellOf(row, suiteKey)
  if (!cell || cell.status !== 'done') return ''
  if (cell.expected_cases <= 0 || cell.judged_cases >= cell.expected_cases) return ''
  return `${cell.judged_cases}/${cell.expected_cases} 题`
}

// Confidence detail (ticket 51) on the item's hover tooltip: judged-case
// coverage plus the number of judged answer attempts behind the score.
// Empty while the suite has judged nothing (nothing to be confident about).
function confidenceOf(row: ReportRow, suiteKey: string): string {
  const cell = cellOf(row, suiteKey)
  if (!cell || cell.judged_cases <= 0) return ''
  return `判分 ${cell.judged_cases}/${cell.expected_cases} 题 · 采样 ${cell.samples} 次`
}

// Baseline note next to the toolbar: names the comparison batch, or why the
// comparison is impossible (ADR 0007 question-bank break / ADR 0008
// scoring-caliber break).
const baselineNote = computed(() => {
  const baseline = props.report.baseline
  if (!baseline) return ''
  if (baseline.comparable) return `涨跌较批次 #${baseline.campaign_id}`
  if (baseline.reason === 'suite_changed') return `较批次 #${baseline.campaign_id}:题目已变更,分数不可比`
  if (baseline.reason === 'profile_changed') return `较批次 #${baseline.campaign_id}:判分口径已变更,分数不可比`
  return `较批次 #${baseline.campaign_id}:考核口径不同,分数不可比`
})

// Delta arrows (ui-guidelines §3): up is green, down is red, and ties,
// missing baselines and caliber breaks never show an arrow — a grey
// placeholder dash instead.
function hasArrow(row: ReportRow): boolean {
  return row.total_delta !== null && row.total_delta !== 0
}

function arrowOf(row: ReportRow): string {
  return (row.total_delta ?? 0) > 0 ? '▲' : '▼'
}

function deltaTone(row: ReportRow): string {
  if (!hasArrow(row)) return 'delta-flat'
  return (row.total_delta ?? 0) > 0 ? 'delta-up' : 'delta-down'
}

function deltaTitle(row: ReportRow): string {
  const baseline = props.report.baseline
  if (!baseline) return '首个已完成批次,无涨跌对比'
  if (!baseline.comparable) {
    if (baseline.reason === 'suite_changed') return `题目已变更,与批次 #${baseline.campaign_id} 分数不可比`
    if (baseline.reason === 'profile_changed') return `判分口径已变更,与批次 #${baseline.campaign_id} 分数不可比`
    return `与批次 #${baseline.campaign_id} 考核口径不同,分数不可比`
  }
  if (row.total_delta === null) return `批次 #${baseline.campaign_id} 无该模型分数`
  if (row.total_delta === 0) return `与批次 #${baseline.campaign_id} 持平`
  return `较批次 #${baseline.campaign_id} 总分变化`
}
</script>

<style scoped>
/* Consumption-page density: 16px card padding via the variable, never a
   :deep(.el-card__body) override (ui-guidelines §2). */
.leaderboard-card {
  --el-card-padding: 16px;
  margin-bottom: 16px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.family-select {
  width: 140px;
}
.sort-select {
  width: 160px;
}
.baseline-note {
  margin-left: auto;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.row-main {
  display: flex;
  align-items: center;
  gap: 12px;
}
.row.clickable {
  cursor: pointer;
  border-radius: var(--hs-radius);
  padding: 2px 4px;
  margin: -2px -4px;
}
.row.clickable:hover {
  background: var(--hs-brand-soft);
}
.row.clickable:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}
.rank {
  width: 24px;
  text-align: right;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  flex-shrink: 0;
}
.rank-live {
  color: var(--hs-text-placeholder);
}
.model {
  width: 220px;
  flex-shrink: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.family-tag {
  flex-shrink: 0;
}
.bar-track {
  flex: 1;
  height: 20px;
  background: var(--hs-brand-soft);
  border-radius: var(--hs-radius-sm);
  overflow: hidden;
}
.bar-fill {
  height: 100%;
  border-radius: var(--hs-radius-sm);
  transition: width 0.3s ease;
}
.score {
  width: 56px;
  text-align: right;
  font-size: var(--hs-text-md);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
  flex-shrink: 0;
}
.suite-strip {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding-left: 36px;
}
.suite-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.suite-name {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  max-width: 96px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.suite-bar-track {
  width: 64px;
  height: 8px;
  background: var(--hs-brand-soft);
  border-radius: var(--hs-radius-xs);
  overflow: hidden;
  flex-shrink: 0;
}
.suite-bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--hs-radius-xs);
}
.suite-score {
  font-size: var(--hs-text-xs);
  line-height: 1.2;
  color: var(--hs-text-primary);
  font-variant-numeric: tabular-nums;
}
.suite-inflight {
  color: var(--hs-text-placeholder);
}
/* Failed suite: the batch/run failure semantic color, same as the progress
   grid's failed cell (ui-guidelines §3). */
.suite-failed {
  color: var(--el-color-danger);
}
.coverage {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
.delta {
  width: 72px;
  text-align: right;
  font-size: var(--hs-text-sm);
  line-height: 1.2;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}
.delta-up {
  color: var(--el-color-success);
}
.delta-down {
  color: var(--el-color-danger);
}
.delta-flat {
  color: var(--hs-text-placeholder);
}
</style>
