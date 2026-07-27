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
           active filter/view shows up as a chip (none omitted), all neutral
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

      <!-- Leaderboard rows: the shared ScoreStackBar in static mode (no
           tooltips; in-segment scores/watermarks follow the same width
           rules as the page), no family tag, no click/hover. The delta
           column renders only in the total view with a comparable baseline. -->
      <div v-else class="rows">
        <div v-for="row in snapshot.rows" :key="row.modelId" class="row">
          <span class="rank">{{ row.rank }}</span>
          <span class="model" :title="row.modelId">{{ row.modelId }}</span>
          <ScoreStackBar
            :suites="snapshot.suites"
            :weights="snapshot.weights"
            :suite-scores="row.suiteScores"
            :cells="row.cells"
            :highlight="snapshot.highlight"
            static-mode
          />
          <span class="score">{{ formatScore(row.score) }}</span>
          <span v-if="snapshot.showDeltaColumn" class="delta" :class="deltaTone(row)">
            <template v-if="hasArrow(row)">{{ arrowOf(row) }} {{ formatScoreDelta(row.delta) }}</template>
            <template v-else>–</template>
          </span>
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
// (ticket 76, spec 0007 — ui-guidelines §5). A designed brand artifact, not
// a page screenshot: 720px logical width, 2x export, always light theme.
// Purely presentational — it renders the frozen snapshot it is given and
// never fetches; chips and every number come from the same report response
// (buildEvalCardSnapshot), never from other page aggregates. The frame
// (brand/scope/footer) is isomorphic to the StatusCard by design convention;
// the two cards deliberately do not share a subcomponent (§5 外框约定).
// Static medium rules: no hover reliance, no animations, rows not clickable.
import { computed } from 'vue'
import type { EvalCardRow, EvalCardSnapshot } from '@/utils/evalCardSnapshot'
import { formatScore, formatScoreDelta, formatTime } from '@/utils/format'
import ScoreStackBar from '@/components/ScoreStackBar.vue'
import BrandMark from '@/components/BrandMark.vue'
import Wordmark from '@/components/Wordmark.vue'

const props = defineProps<{
  snapshot: EvalCardSnapshot
  origin: string
}>()

// "YYYY-MM-DD HH:mm" out of the shared formatTime helper.
const timeText = computed(() => {
  const full = formatTime(props.snapshot.generatedAt)
  return full.length >= 16 ? full.slice(0, 16) : full
})

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
  gap: 10px;
  margin-bottom: 24px;
}
.row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.rank {
  width: 24px;
  text-align: right;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  flex-shrink: 0;
}
.model {
  width: 200px;
  flex-shrink: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.score {
  width: 56px;
  text-align: right;
  font-size: var(--hs-text-md);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
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
