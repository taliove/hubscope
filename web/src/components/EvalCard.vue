<template>
  <div class="eval-card">
    <div class="brand-bar" />
    <div class="brand-section">
      <BrandMark class="brand-mark" />
      <Wordmark class="brand-wordmark" />
      <span class="brand-title">评估榜单</span>
    </div>

    <div class="card-body">
      <!-- Scope: the anti-fake line. The batch chip always leads; every
           active filter/sort shows up as a chip (none omitted), all neutral
           — the failed emphasis is carried by the warning line below. -->
      <div class="scope-row">
        <span v-for="chip in snapshot.chips" :key="chip.label" class="scope-chip">
          <span class="chip-label">{{ chip.label }}</span>
          <span class="chip-value" :title="chip.value">{{ chip.value }}</span>
        </span>
      </div>

      <!-- Failed-batch warning: same wording as the page alert, colored text
           without a background (ui-guidelines §5 EvalCard entry). -->
      <p v-if="snapshot.failedWarning" class="failed-warning">{{ snapshot.failedWarning }}</p>

      <!-- Empty filtered result: chips stay, the row area states a neutral
           "no match" — never reads as "全部上榜" (anti-fake, §5). -->
      <p v-if="snapshot.rows.length === 0" class="empty-note">暂无匹配模型</p>

      <!-- Matrix board (ticket 79, spec 0009): the static isomorph of the
           page Leaderboard — one fixed column per dimension, header and rows
           sharing one grid template. The header carries the suite names
           (the only place dimension names appear, same convergence as the
           page); cells are the shared ScoreCell in static mode (no
           tooltips; the watermark follows the same width rule as the page).
           No family tag, no click/hover. The delta column renders only with
           a comparable baseline. -->
      <div v-else class="rows">
        <div class="ec-grid ec-header" :style="gridStyle">
          <span class="h-rank">名次</span>
          <span class="h-model">模型</span>
          <span class="h-total">总分</span>
          <span v-if="snapshot.showDeltaColumn" class="h-delta">涨跌</span>
          <span v-for="s in snapshot.suites" :key="s.key" class="h-suite">{{ s.name }}</span>
        </div>
        <div
          v-for="row in snapshot.rows"
          :key="row.modelId"
          class="ec-grid row"
          :class="{ 'rank-top-rail': row.rank !== null && row.rank <= 3 }"
          :style="gridStyle"
        >
          <span class="rank" :class="{ 'rank-top': row.rank !== null && row.rank <= 3, 'rank-dash': row.rank === null }">{{
            row.rank ?? '–'
          }}</span>
          <span class="model">
            <span class="model-name">{{ row.modelId }}</span>
            <!-- Judged-incomplete watermark (ticket 92): same wording and
                 position as the page, fully visible — the static material
                 has no tooltip fallback. -->
            <span v-if="row.incompleteNote" class="model-watermark">{{ row.incompleteNote }}</span>
          </span>
          <span class="total">
            <span class="total-value">{{ formatScore(row.score) }}</span>
            <span class="total-track">
              <span
                v-if="row.score !== null"
                class="total-fill"
                :class="`band-${scoreBand(row.score)}`"
                :style="{ width: totalWidth(row.score) }"
              />
            </span>
          </span>
          <span v-if="snapshot.showDeltaColumn" class="delta" :class="deltaTone(row)">
            <template v-if="hasArrow(row)">{{ arrowOf(row) }} {{ formatScoreDelta(row.delta) }}</template>
            <template v-else>–</template>
          </span>
          <ScoreCell
            v-for="s in snapshot.suites"
            :key="s.key"
            :name="s.name"
            :score="row.suiteScores[s.key] ?? null"
            :cell="cellOf(row, s.key)"
            static-mode
          />
        </div>
        <p v-if="snapshot.overflowCount > 0" class="overflow-note">
          另有 {{ snapshot.overflowCount }} 个模型未列出,详见评估榜单
        </p>
      </div>
    </div>

    <div class="card-footer">
      <span>生成于 {{ timeText }}</span>
      <span class="footer-origin">{{ origin }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
// EvalCard: the single render template of the leaderboard share image
// (ticket 76, matrix revision ticket 79 / spec 0009 — ui-guidelines §5). A
// designed brand artifact, not a page screenshot: 720px logical width, 2x
// export, always light theme. Purely presentational — it renders the frozen
// snapshot it is given and never fetches; chips and every number come from
// the same report response (buildEvalCardSnapshot), never from other page
// aggregates. The frame (brand/scope/footer) is isomorphic to the StatusCard
// by design convention; the two cards deliberately do not share a
// subcomponent (§5 外框约定). Static medium rules: no hover reliance, no
// animations, rows not clickable.
import { computed } from 'vue'
import type { EvalCardRow, EvalCardSnapshot } from '@/utils/evalCardSnapshot'
import type { ReportCell } from '@/api/types'
import { formatScore, formatScoreDelta, formatTime } from '@/utils/format'
import ScoreCell from '@/components/ScoreCell.vue'
import BrandMark from '@/components/BrandMark.vue'
import Wordmark from '@/components/Wordmark.vue'
import { scoreBand } from '@/utils/scoreTier'

const props = defineProps<{
  snapshot: EvalCardSnapshot
  origin: string
}>()

// "YYYY-MM-DD HH:mm" out of the shared formatTime helper.
const timeText = computed(() => {
  const full = formatTime(props.snapshot.generatedAt)
  return full.length >= 16 ? full.slice(0, 16) : full
})

// The grid template shared by the header and every row (same alignment
// discipline as the page matrix). Fixed widths: rank 24 / model 150 /
// total 64 / delta 60; dimension columns split the rest equally (~56-68px
// each with five suites, depending on the delta column — enough for the
// score digits, below the watermark threshold, so coverage detail stays
// page-side per the registered information gap).
const gridStyle = computed(() => {
  const cols = ['24px', '150px', '64px']
  if (props.snapshot.showDeltaColumn) cols.push('60px')
  for (let i = 0; i < props.snapshot.suites.length; i += 1) cols.push('minmax(0, 1fr)')
  return { gridTemplateColumns: cols.join(' ') }
})

function cellOf(row: EvalCardRow, suiteKey: string): ReportCell | undefined {
  return row.cells.find((c) => c.suite_key === suiteKey)
}

function totalWidth(score: number): string {
  return `${Math.min(100, Math.max(0, score))}%`
}

// Delta arrows (ui-guidelines §3): up green, down red; ties and missing
// deltas show a grey placeholder dash, never an arrow.
function hasArrow(row: EvalCardRow): boolean {
  return row.delta !== null && row.delta !== 0
}

function arrowOf(row: EvalCardRow): string {
  return (row.delta ?? 0) > 0 ? '▲' : '▼'
}

function deltaTone(row: EvalCardRow): string {
  if (!hasArrow(row)) return 'delta-flat'
  return (row.delta ?? 0) > 0 ? 'delta-up' : 'delta-down'
}
</script>

<style scoped>
.eval-card {
  width: 720px;
  border-radius: var(--hs-radius-lg);
  /* Static card: layering comes from the 1px border, not shadow
     (ui-guidelines §2 shadow semantics). */
  border: 1px solid var(--hs-border);
  background: var(--hs-bg-card);
  overflow: hidden;
  text-align: left;
}
.brand-bar {
  height: 4px;
  background: var(--hs-brand);
}
.brand-section {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 40px;
  background: var(--hs-brand-soft);
}
.brand-mark {
  font-size: 32px;
  flex-shrink: 0;
}
.brand-wordmark {
  font-size: var(--hs-text-xl);
  flex-shrink: 0;
}
.brand-title {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.card-body {
  padding: 24px 40px 0;
}
.scope-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.scope-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  padding: 2px 8px;
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-card);
  font-size: var(--hs-text-sm);
}
.chip-label {
  color: var(--hs-text-secondary);
}
.chip-value {
  color: var(--hs-text-primary);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.failed-warning {
  margin: -8px 0 16px;
  font-size: var(--hs-text-sm);
  color: var(--hs-warning);
}
.empty-note {
  margin: 0 0 24px;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
}
.rows {
  display: flex;
  flex-direction: column;
  margin-bottom: var(--hs-space-5);
}
/* Table polish (ticket 82, same rhythm as the page): header hairline,
   inter-row hairlines, 46px rows. */
.ec-header {
  padding-bottom: var(--hs-space-2);
  border-bottom: 1px solid var(--hs-border);
  /* Matches the rows' transparent 3px rail so header columns share the
     rows' x positions exactly (matrix alignment invariant). */
  border-left: 3px solid transparent;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.row {
  min-height: 46px;
  border-left: 3px solid transparent;
}
.row + .row {
  border-top: 1px solid var(--hs-border-light);
}
.ec-grid {
  display: grid;
  column-gap: 8px;
  align-items: center;
}
.h-rank {
  text-align: right;
}
.h-delta {
  text-align: right;
}
.h-model,
.h-total,
.h-suite {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rank {
  text-align: right;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
/* Top-3 ceremony (ticket 82, same as the page): 3px brand rail + the rank
   number one size up at 600. Judged-incomplete rows (ticket 92, rank null)
   render the placeholder dash and never take the rail. */
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
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
}
.model-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Same reduced-emphasis spec as the page watermark (weight 400 + opacity
   0.85); wraps naturally if the N/M ever outgrows the 150px column. */
.model-watermark {
  font-size: var(--hs-text-xs);
  font-weight: 400;
  line-height: 1.2;
  color: var(--hs-text-secondary);
  opacity: 0.85;
  white-space: normal;
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
.overflow-note {
  margin: 4px 0 0;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 16px;
  margin: 0 40px;
  padding: 16px 0 24px;
  border-top: 1px solid var(--hs-border);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.footer-origin {
  flex: none;
}
</style>
