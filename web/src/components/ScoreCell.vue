<template>
  <!-- ScoreCell is the single dimension cell of the matrix Leaderboard
       (ticket 78, spec 0009): the band-colored score over a thin 4px band bar
       on a FIXED 0-100 track — 87.5 always renders shorter than 100, the
       visual mirror of the W7 absolute-score system (no per-row/column
       normalization). Null (unscored) renders a placeholder dash over an
       empty track. The compressed coverage watermark rides the score when
       the cell is wide enough; the hover tooltip always carries the full
       ticket-51 confidence caliber. staticMode (exported material, ticket
       79) drops the tooltip and measures the width once without observing
       (the material never resizes) — the watermark follows the same width
       rule as the page, and the tooltip's confidence info stays out of the
       material (registered information gap, ui-guidelines §5). -->
  <div ref="rootRef" class="score-cell" :title="tooltip">
    <span class="cell-value" :class="valueClass">
      {{ label }}<span v-if="showWatermark" class="cell-watermark">{{ watermark }}</span>
    </span>
    <div class="cell-track">
      <div v-if="score !== null" class="cell-fill" :class="`band-${band}`" :style="{ width: fillWidth }" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { ReportCell } from '@/api/types'
import { formatScore } from '@/utils/format'
import { cellStatusText, scoreBand, tooltipOf, watermarkOf } from '@/utils/scoreTier'

// Tentative watermark threshold: the score label ('100.0' at md/600, tabular)
// plus the '·10/10' suffix needs roughly 80px; the matrix dimension columns
// are ~100-140px, so the watermark normally fits. Calibrated against the
// production board before the guideline entry lands (ticket 79), same
// discipline as the stack bar's LABEL_MIN_PX.
const WATERMARK_MIN_PX = 80

// Props seam (design-review freeze, ticket 78): split fields rather than the
// whole ReportRow, so the ticket-79 static EvalCard can construct the same
// inputs without the row shape. `staticMode` is the exported-material mode
// (no tooltip; width measured once, never observed). A null score is a
// dash over an empty track in every mode; the unscored tooltip wording
// comes from the cell's own status.
const props = withDefaults(
  defineProps<{
    name: string
    score: number | null
    cell: ReportCell | undefined
    staticMode?: boolean
  }>(),
  { staticMode: false },
)

const label = computed(() => (props.score === null ? '–' : formatScore(props.score)))
const band = computed(() => (props.score === null ? null : scoreBand(props.score)))
const valueClass = computed(() => (band.value ? `band-${band.value}` : 'is-null'))
const fillWidth = computed(() => {
  if (props.score === null) return '0%'
  return `${Math.min(100, Math.max(0, props.score))}%`
})

const watermark = computed(() => watermarkOf(props.cell))

// The watermark is width-adaptive: rendered when the measured cell is wide
// enough, omitted otherwise — the tooltip below always carries the full
// confidence info (anti-fake: reachable, never silently dropped). Static
// material measures once (it has layout in the off-screen capture container
// but never resizes), so the page rule "rendered when wide enough" holds
// identically on both ends.
const rootRef = ref<HTMLElement | null>(null)
const cellWidth = ref(0)
let observer: ResizeObserver | null = null

onMounted(() => {
  if (!rootRef.value) return
  cellWidth.value = rootRef.value.clientWidth
  if (props.staticMode) return
  observer = new ResizeObserver((entries) => {
    cellWidth.value = entries[0]?.contentRect.width ?? 0
  })
  observer.observe(rootRef.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

const showWatermark = computed(() => watermark.value !== '' && cellWidth.value >= WATERMARK_MIN_PX)

const tooltip = computed(() => {
  if (props.staticMode) return undefined
  if (props.score !== null) return tooltipOf(props.name, props.score, props.cell)
  return `${props.name} · ${cellStatusText(props.cell)}`
})
</script>

<style scoped>
.score-cell {
  min-width: 0;
}
.cell-value {
  font-size: var(--hs-text-md);
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.cell-value.band-success {
  color: var(--hs-success);
}
.cell-value.band-warning {
  color: var(--hs-warning);
}
.cell-value.band-danger {
  color: var(--hs-danger);
}
.cell-value.is-null {
  color: var(--hs-text-placeholder);
}
/* The compressed coverage watermark rides the score in the same band ink at
   reduced emphasis (same treatment as the stack-bar watermark). */
.cell-watermark {
  font-weight: 400;
  opacity: 0.85;
}
.cell-track {
  height: 4px;
  margin-top: 2px;
  background: var(--hs-brand-soft);
  border-radius: var(--hs-radius-xs);
  overflow: hidden;
}
.cell-fill {
  height: 100%;
}
.cell-fill.band-success {
  background: var(--hs-success);
}
.cell-fill.band-warning {
  background: var(--hs-warning);
}
.cell-fill.band-danger {
  background: var(--hs-danger);
}
</style>
