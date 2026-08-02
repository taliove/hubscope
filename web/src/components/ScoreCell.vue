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
       material (registered information gap, ui-guidelines §5). GH #94:
       show-name prop renders the dimension name above the score for the
       narrow-viewport card-style list (the唯一组件纪律 stays intact). -->
  <div
    ref="rootRef"
    class="score-cell"
    :class="{ 'show-name': showName, clickable: isClickable }"
    :title="tooltip"
    :role="isClickable ? 'button' : undefined"
    :tabindex="isClickable ? 0 : undefined"
    @click="onCellClick"
    @keydown.enter.stop="onActivate"
    @keydown.space.prevent.stop="onActivate"
  >
    <span v-if="showName" class="cell-name">{{ name }}</span>
    <!-- Live-mode unscored cell (GH #40, ui-guidelines §5 运行中半成品模式
         ④): the batch status word inline instead of a bare dash — plain text
         at xs, colored by the batch/run status mapping, never a dot (dots +
         words are the progress grid's shape; the scores view must not grow a
         second status lamp). The empty track stays below. -->
    <span v-if="isLiveUnscored" class="cell-value cell-live-status" :class="`live-${cell?.status}`">
      {{ liveStatusWord }}
    </span>
    <span v-else class="cell-value" :class="valueClass">
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
import { cellStatusText, liveCellTooltip, scoreBand, tooltipOf, watermarkOf } from '@/utils/scoreTier'

// Watermark threshold (finalized in ticket 79 and registered in
// ui-guidelines §5): the score label ('100.0' at md/600, tabular, ~38px)
// plus the '·10/10' suffix (~34px) plus slack. Page dimension columns are
// ~100-140px, so the watermark normally fits; the material's ~56-68px
// columns stay below it, so the watermark never renders there (the
// registered information gap).
const WATERMARK_MIN_PX = 80

// Props seam (design-review freeze, ticket 78): split fields rather than the
// whole ReportRow, so the ticket-79 static EvalCard can construct the same
// inputs without the row shape. `staticMode` is the exported-material mode
// (no tooltip; width measured once, never observed). A null score is a
// dash over an empty track in every mode; the unscored tooltip wording
// comes from the cell's own status. `live` (GH #40, unfinished-batch board
// only) swaps the null-score dash for the inline batch status word — the
// settled board and the static material never pass it. `showName` (GH #94,
// narrow-viewport card-style list) renders the dimension name above the score
// for the 2-column dimension grid; the matrix mode never passes it.
const props = withDefaults(
  defineProps<{
    name: string
    score: number | null
    cell: ReportCell | undefined
    staticMode?: boolean
    live?: boolean
    showName?: boolean
    // GH #156 block 4: a clickable cell emits `activate` (cell drill-down to
    // the per-case run detail) with the click stopped, so the leaderboard
    // row's own select (trend dialog) never fires from a cell click. Never
    // clickable in staticMode — the exported material has no interaction.
    clickable?: boolean
  }>(),
  { staticMode: false, live: false, showName: false, clickable: false },
)

const emit = defineEmits<{ (e: 'activate'): void }>()

const isClickable = computed(() => props.clickable && !props.staticMode)

function onActivate() {
  if (!isClickable.value) return
  emit('activate')
}

// Non-clickable cells let the click bubble to the row (its own drill-down);
// clickable cells own the click.
function onCellClick(event: MouseEvent) {
  if (!isClickable.value) return
  event.stopPropagation()
  onActivate()
}

const isLiveUnscored = computed(() => props.live && props.score === null && props.cell !== undefined)
const liveStatusWord = computed(() => cellStatusText(props.cell))

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
  return liveCellTooltip(props.name, props.cell)
})
</script>

<style scoped>
.score-cell {
  min-width: 0;
}
/* Clickable cell (GH #156 block 4): pointer + hover fill out of the existing
   surface tokens; focus ring mirrors the leaderboard row's treatment. */
.score-cell.clickable {
  cursor: pointer;
  border-radius: var(--hs-radius-sm);
}
.score-cell.clickable:hover {
  background: var(--hs-bg-hover);
}
.score-cell.clickable:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: -2px;
}
/* GH #94: show-name mode (narrow-viewport card-style list) stacks the
   dimension name above the score + bar. */
.score-cell.show-name {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
}
.cell-name {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  line-height: 1.2;
}
.cell-value {
  font-size: var(--hs-text-md);
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
/* GH #69 text/graphics split: the band-colored score number is text —
   deepened grade (the .cell-fill bar below keeps the base as a graphic). */
.cell-value.band-success {
  color: var(--hs-success-text);
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
/* Live-mode unscored status word (GH #40): xs plain text on the batch/run
   status color mapping (§3) — running brand, pending placeholder, failed
   danger; a done-but-unscored cell falls back to the neutral placeholder.
   Never a dot, never a flash. */
.cell-live-status {
  font-size: var(--hs-text-xs);
  font-weight: 400;
}
.cell-live-status.live-running {
  color: var(--hs-brand);
}
.cell-live-status.live-pending {
  color: var(--hs-text-placeholder);
}
.cell-live-status.live-failed {
  color: var(--hs-danger);
}
.cell-live-status.live-done {
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
  /* Neutral track (ticket 82, spec 0010): the band colors are the only loud
     element on the board; the track is a surface tint one step above the
     card (--hs-bg-hover), never a brand tint. */
  background: var(--hs-bg-hover);
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
