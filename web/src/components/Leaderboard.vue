<template>
  <el-card shadow="never" class="leaderboard-card">
    <!-- Toolbar (spec 0009): family filter + baseline note + share entry.
         The dimension radio and sort select are gone — every dimension is
         on screen at once and the column headers are the ranking control.
         In live mode (unfinished batch, ticket 52) the grid/scores view
         switch takes the lead slot and the headers are not clickable — the
         half-scored board never re-ranks; family filtering stays
         available. -->
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
      <span v-if="!live" class="toolbar-end">
        <span v-if="baselineNote" class="baseline-note">{{ baselineNote }}</span>
        <!-- Share-image entry (ticket 76): settled batches only — the live
             toolbar never renders this button, so running/pending batches
             have no image-share entry on any of the three pages (spec 0004
             half-baked-score boundary). -->
        <el-button text size="small" @click="openShare">
          <el-icon><Share /></el-icon>
          {{ shared ? '保存图片' : '分享图片' }}
        </el-button>
      </span>
    </div>

    <!-- Empty state: no model ranked (nothing scored, or filtered out). -->
    <el-empty v-if="report.rows.length === 0" :description="emptyDescription" />

    <!-- Matrix board (ticket 78, spec 0009): one fixed column per dimension —
         rank | model(+family tag) | total | delta | suite 1..N. Header and
         rows share the same CSS grid template, so every column's x position
         is constant across the whole table regardless of family-tag width
         (the flex-row drift that made alignment structurally impossible).
         Dimension cells are the shared ScoreCell: band-colored score over a
         thin bar on a fixed 0-100 track. Rows are clickable: a click emits
         select for the trend drill-down dialog (ticket 32, no inline row
         expansion per §4). Live mode keeps the backend's model-id
         lexicographic order and swaps the rank slot for a placeholder dash. -->
    <div v-else>
      <div class="lb-grid lb-header" :style="gridStyle">
        <span class="h-rank">名次</span>
        <span class="h-model">模型</span>
        <!-- Sortable headers (spec 0009, descending-only ruling): click to
             rank by that column descending (↓ indicator on the active one);
             click the active column to fall back to the total. Ranking goes
             through the server-side query.sort unchanged. In live mode the
             headers are inert — no pointer, no indicator — because the
             half-scored board keeps the backend's lexicographic order. -->
        <span
          class="h-total h-sortable"
          :class="{ 'h-active': !live && sortKey === 'total', 'h-disabled': live }"
          :role="live ? undefined : 'button'"
          :tabindex="live ? undefined : 0"
          @click="onSort('total')"
          @keydown.enter="onSort('total')"
          @keydown.space.prevent="onSort('total')"
        >
          总分<span v-if="!live && sortKey === 'total'" class="sort-arrow">↓</span>
        </span>
        <span v-if="!live" class="h-delta">涨跌</span>
        <span
          v-for="s in report.suites"
          :key="s.key"
          class="h-suite h-sortable"
          :class="{ 'h-active': !live && sortKey === s.key, 'h-disabled': live }"
          :title="s.name"
          :role="live ? undefined : 'button'"
          :tabindex="live ? undefined : 0"
          @click="onSort(s.key)"
          @keydown.enter="onSort(s.key)"
          @keydown.space.prevent="onSort(s.key)"
        >
          {{ s.name }}<span v-if="!live && sortKey === s.key" class="sort-arrow">↓</span>
        </span>
        <span v-if="live" class="h-note" />
      </div>
      <div class="rows">
        <div
          v-for="(row, index) in report.rows"
          :key="row.model_db_id"
          class="lb-grid row"
          :class="{ clickable: selectable, 'rank-top-rail': isTopRank(row, index) }"
          :role="selectable ? 'button' : undefined"
          :tabindex="selectable ? 0 : undefined"
          :style="gridStyle"
          @click="selectable && emit('select', row)"
          @keydown.enter="selectable && emit('select', row)"
          @keydown.space.prevent="selectable && emit('select', row)"
        >
          <span class="rank" :class="{ 'rank-dash': live || row.complete === false, 'rank-top': isTopRank(row, index) }">{{
            live || row.complete === false ? '–' : index + 1
          }}</span>
          <span class="model">
            <span class="model-line">
              <span class="model-name" :title="row.model_id">{{ row.model_id }}</span>
              <el-tag size="small" effect="plain" class="family-tag">{{ row.family }}</el-tag>
            </span>
            <!-- Judged-incomplete watermark (ticket 92, spec 0014 decision A;
                 ui-guidelines §5 判分不完整模式): second line under the model
                 name — the model is the subject of the verdict, the three
                 dashes are its consequences. Never truncated (the static
                 material has no hover to fall back on). -->
            <span v-if="row.complete === false" class="model-watermark">{{ incompleteWatermark(row) }}</span>
          </span>
          <!-- Total column: xl ink number, NEVER band-colored — hierarchy
               comes from size and the thicker 6px bar, not color. In live
               mode the track stays empty and uncolored so a half-scored
               total can never read as a bad grade (spec 0004 mirror). -->
          <span class="total">
            <span class="total-value">{{ formatScore(row.total_score) }}</span>
            <span class="total-track">
              <span
                v-if="!live && row.total_score !== null"
                class="total-fill"
                :class="`band-${scoreBand(row.total_score)}`"
                :style="{ width: totalWidth(row.total_score) }"
              />
            </span>
          </span>
          <!-- Delta column is always visible on settled batches; row-level
               caliber unchanged (arrow on non-zero delta, dash otherwise). -->
          <span v-if="!live" class="delta" :class="deltaTone(row)" :title="deltaTitle(row)">
            <template v-if="hasArrow(row)">{{ arrowOf(row) }} {{ formatScoreDelta(row.total_delta) }}</template>
            <template v-else>–</template>
          </span>
          <ScoreCell
            v-for="s in report.suites"
            :key="s.key"
            :name="s.name"
            :score="row.suite_scores[s.key] ?? null"
            :cell="cellOf(row, s.key)"
          />
          <span v-if="live" class="live-note">
            <template v-if="countsOf(row).inFlight > 0">{{ countsOf(row).inFlight }} 个维度进行中</template>
            <span v-if="countsOf(row).failed > 0" class="live-failed"
              >{{ countsOf(row).inFlight > 0 ? '· ' : '' }}{{ countsOf(row).failed }} 个失败</span
            >
          </span>
        </div>
      </div>
    </div>

    <!-- Share-image dialog; the snapshot freezes at open time so a report
         refresh cannot swap the data between preview and export. -->
    <EvalShareDialog v-model:visible="shareVisible" :snapshot="shareSnapshot" :shared="shared" />
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Share } from '@element-plus/icons-vue'
import { formatScore, formatScoreDelta } from '@/utils/format'
import type { CampaignReport, EvalBoardView, ReportCell, ReportRow } from '@/api/types'
import ScoreCell from '@/components/ScoreCell.vue'
import EvalShareDialog from '@/components/EvalShareDialog.vue'
import { scoreBand, liveCounts } from '@/utils/scoreTier'
import { nextSortKey } from '@/utils/sortHeader'
import { buildEvalCardSnapshot, type EvalCardSnapshot } from '@/utils/evalCardSnapshot'
import { baselineNoteText, incompleteWatermark } from '@/utils/evalWording'

// Leaderboard is the single ranking display of the eval board (registered in
// ui-guidelines §5): one row per model — rank, truncated name, family tag,
// the 0-100 total with its 6px band bar, the total-score delta arrow versus
// the previous done campaign, and one ScoreCell per dimension. Family
// filtering lives in the toolbar, never inside cells. The parent fetches
// data; this component only re-emits query changes.
//
// Live mode (ticket 52, half-scored board of an unfinished batch): the rank
// slot shows a placeholder dash (no rank badges pre-settle), rows keep the
// backend's model-id lexicographic order, the ranking controls are disabled,
// and the delta column hides entirely. Scored suites render normally;
// unscored cells are a dash over an empty track and surface in the row-end
// "N 个维度进行中 / N 个失败" note.
//
// Judged-incomplete mode (ticket 92, spec 0014 decision A — settled batches
// only, mutually exclusive with live by construction): a row with
// complete === false forfeits rank/total/delta — dash in all three slots,
// the same empty uncolored total track — but keeps its real per-suite
// scores (the gate takes ranking eligibility, never the judged facts). The
// "判分不完整,缺 N/M 维度" watermark rides a second line under the model
// name; the server already sank these rows below every complete one (the
// /board client mirrors that caliber in utils/boardSort).
const props = withDefaults(
  defineProps<{
    report: CampaignReport
    // Options come from the unfiltered board, so filtering never collapses the
    // option list itself.
    familyOptions: string[]
    live?: boolean
    view?: EvalBoardView
    // Shared report page (/report/:token): the share-image entry copy reads
    // "保存图片" for the recipient reader (ticket 76, ui-guidelines §5).
    shared?: boolean
    // Public board (/board, ticket 81): rows are not clickable — the public
    // page has no trend drill-down. Default true; /eval behavior unchanged.
    selectable?: boolean
  }>(),
  { live: false, view: 'grid', shared: false, selectable: true },
)

const emit = defineEmits<{
  (e: 'query', query: { family?: string; sort: string }): void
  (e: 'select', row: ReportRow): void
  (e: 'update:view', view: EvalBoardView): void
}>()

const family = ref('')
const sortKey = ref('total')

// Share-image state (ticket 76): the snapshot freezes the currently
// displayed batch + filters at open time; a report prop refresh never
// rebuilds an open snapshot.
const shareVisible = ref(false)
const shareSnapshot = ref<EvalCardSnapshot | null>(null)

function openShare() {
  shareSnapshot.value = buildEvalCardSnapshot(props.report, {
    family: family.value || undefined,
    sort: sortKey.value,
  })
  shareVisible.value = true
}

// Column-header sorting (descending-only ruling): live mode never re-ranks —
// the rows keep the backend's lexicographic order.
function onSort(key: string) {
  if (props.live) return
  sortKey.value = nextSortKey(sortKey.value, key)
  emitQuery()
}

function emitQuery() {
  emit('query', { family: family.value || undefined, sort: sortKey.value })
}

// The grid template shared by the header and every row — the single source
// of column x positions. Fixed widths: rank 32 / model 260 / total 96 /
// delta 80; dimension columns split the rest equally (spec 0009: column
// width is independent of suite weight). Live mode drops the delta column
// and appends an auto-width column for the row-end note.
const gridStyle = computed(() => {
  const cols = ['32px', '260px', '96px']
  if (!props.live) cols.push('80px')
  for (let i = 0; i < props.report.suites.length; i += 1) cols.push('minmax(0, 1fr)')
  if (props.live) cols.push('auto')
  return { gridTemplateColumns: cols.join(' ') }
})

function cellOf(row: ReportRow, suiteKey: string): ReportCell | undefined {
  return row.cells.find((c) => c.suite_key === suiteKey)
}

// Top-3 ceremony (ticket 82) applies to the COMPLETE group's first three
// only: the coverage gate sinks incomplete rows, but nothing guarantees at
// least three complete rows, so the index alone would crown an incomplete
// row (ui-guidelines §5 判分不完整模式 — never judge by index alone).
function isTopRank(row: ReportRow, index: number): boolean {
  return !props.live && row.complete !== false && index < 3
}

function countsOf(row: ReportRow) {
  return liveCounts(row.cells)
}

function totalWidth(score: number): string {
  return `${Math.min(100, Math.max(0, score))}%`
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

// Baseline note next to the toolbar: names the comparison batch, or why the
// comparison is impossible (ADR 0007 question-bank break / ADR 0008
// scoring-caliber break).
const baselineNote = computed(() => baselineNoteText(props.report.baseline))

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
  // Judged-incomplete rows (ticket 92): they have suite scores, just no
  // total — the generic "no score in baseline" line would be inaccurate.
  if (row.complete === false) return '判分不完整,不参与排名与涨跌'
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
.toolbar-end {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 12px;
}
.baseline-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.lb-grid {
  display: grid;
  column-gap: 12px;
  align-items: center;
}
/* Table polish (ticket 82, spec 0010): a 1px hairline under the header and
   between rows replaces the bare gap; rows breathe at a fixed 46px (the
   fifteen-rows-per-viewport sweet spot). */
.lb-header {
  margin-bottom: 0;
  padding-bottom: var(--hs-space-2);
  border-bottom: 1px solid var(--hs-border);
  /* Matches the rows' transparent 3px rail so header columns share the
     rows' x positions exactly (matrix alignment invariant). */
  border-left: 3px solid transparent;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.h-rank {
  text-align: right;
}
.h-model,
.h-total,
.h-suite {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.h-delta {
  text-align: right;
}
/* Sortable headers: hover feedback on every ranking column, the active one
   takes primary ink plus the descending arrow (spec 0009). */
.h-sortable {
  cursor: pointer;
  user-select: none;
  border-radius: var(--hs-radius-sm);
}
.h-sortable:hover {
  color: var(--hs-text-primary);
}
.h-sortable:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}
.h-active {
  color: var(--hs-text-primary);
  font-weight: 600;
}
/* Live mode: headers are inert (no pointer, no hover feedback, no arrow). */
.h-disabled {
  cursor: default;
}
.h-disabled:hover {
  color: var(--hs-text-secondary);
}
.sort-arrow {
  margin-left: 2px;
}
.rows {
  display: flex;
  flex-direction: column;
}
.row {
  min-height: 46px;
  /* The 3px left border is the top-3 ceremony rail (transparent for every
     other rank so column x positions never shift); see .rank-top-rail. */
  border-left: 3px solid transparent;
}
.row + .row {
  border-top: 1px solid var(--hs-border-light);
}
.row.clickable {
  cursor: pointer;
}
/* Hover/focus fill the row's own box (table semantics): the hairlines and
   the 46px row height define the click area, no negative-margin bleed. */
.row.clickable:hover {
  background: var(--hs-brand-soft);
}
.row.clickable:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: -2px;
}
.rank {
  text-align: right;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
/* Top-3 ceremony (ticket 82, spec 0010): a 3px brand rail on the row's
   left edge — border language, never a background block — plus the rank
   number one size up at 600. The rail stays visible over the brand-soft
   hover tint in both themes. */
.rank-top-rail {
  border-left-color: var(--hs-brand);
}
.rank-top {
  color: var(--hs-brand);
  font-size: var(--hs-text-lg);
  font-weight: 600;
}
.rank-dash {
  color: var(--hs-text-placeholder);
}
.model {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--hs-space-1);
  min-width: 0;
}
.model-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
/* Judged-incomplete watermark (ticket 92): xs secondary at the ScoreCell
   watermark's reduced emphasis (weight 400 + opacity 0.85); wraps naturally
   on narrow columns, never truncated — the static material has no hover. */
.model-watermark {
  font-size: var(--hs-text-xs);
  font-weight: 400;
  line-height: 1.2;
  color: var(--hs-text-secondary);
  opacity: 0.85;
  white-space: normal;
}
.model-name {
  flex: 1;
  min-width: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.family-tag {
  flex-shrink: 0;
}
.total-value {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
  font-variant-numeric: tabular-nums;
}
.total-track {
  display: block;
  height: 6px;
  margin-top: 2px;
  /* Neutral track, same token and rationale as ScoreCell (ticket 82). */
  background: var(--hs-bg-hover);
  border-radius: var(--hs-radius-xs);
  overflow: hidden;
}
.total-fill {
  display: block;
  height: 100%;
}
.total-fill.band-success {
  background: var(--hs-success);
}
.total-fill.band-warning {
  background: var(--hs-warning);
}
.total-fill.band-danger {
  background: var(--hs-danger);
}
.delta {
  text-align: right;
  font-size: var(--hs-text-sm);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.delta-up {
  color: var(--hs-success);
}
.delta-down {
  color: var(--hs-danger);
}
.delta-flat {
  color: var(--hs-text-placeholder);
}
.live-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  white-space: nowrap;
}
.live-failed {
  color: var(--hs-danger);
}
</style>
