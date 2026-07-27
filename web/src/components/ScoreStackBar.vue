<template>
  <!-- ScoreStackBar is the single shared stacked-bar component of the eval
       board (ui-guidelines §5, ticket 75): one compact bar replacing the old
       two-layer "total bar + per-suite strip" row. Segments follow
       report.suites order and stack from the left; their widths are
       weight-normalized against the scored suites so the bar length equals
       the total score by construction. Consumed by the Leaderboard row and
       (ticket 76) the static EvalCard — staticMode drops hover tooltips for
       exported material. -->
  <div class="stack-wrap">
    <div ref="trackRef" class="stack-track">
      <div
        v-for="seg in segments"
        :key="seg.key"
        class="stack-seg"
        :class="[`band-${seg.band}`, { dimmed: isDimmed(seg.key) }]"
        :style="{ width: seg.widthPct + '%' }"
        :title="staticMode ? undefined : seg.tooltip"
      >
        <span v-if="seg.showLabel" class="seg-label">
          {{ seg.label }}<span v-if="seg.showWatermark" class="seg-watermark">{{ seg.watermark }}</span>
        </span>
      </div>
    </div>
    <span v-if="live && (counts.inFlight > 0 || counts.failed > 0)" class="live-note">
      <template v-if="counts.inFlight > 0">{{ counts.inFlight }} 个维度进行中</template>
      <span v-if="counts.failed > 0" class="live-failed"
        >{{ counts.inFlight > 0 ? '· ' : '' }}{{ counts.failed }} 个失败</span
      >
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { ReportCell, ReportSuite } from '@/api/types'
import { buildStackSegments, liveCounts } from '@/utils/stackSegments'

// Props seam (design-review freeze, ui-guidelines §5): split fields rather
// than the whole ReportRow, so the ticket-76 EvalCard snapshot can construct
// the same inputs without being coupled to the row shape. `highlight` is the
// suite key selected in the dimension view (that segment stays opaque, the
// rest dim to ~40%); `live` gates the right-of-bar in-flight/failed note;
// `staticMode` is the exported-material mode (no hover tooltip).
const props = withDefaults(
  defineProps<{
    suites: ReportSuite[]
    weights: Record<string, number>
    suiteScores: Record<string, number | null>
    cells: ReportCell[]
    highlight?: string | null
    live?: boolean
    staticMode?: boolean
  }>(),
  { highlight: null, live: false, staticMode: false },
)

// The label/watermark width thresholds are pixel-based, so the component
// measures its own track; the pure segment model consumes the measured
// width and stays render-agnostic. The observer is paired with cleanup.
const trackRef = ref<HTMLElement | null>(null)
const trackWidth = ref(0)
let observer: ResizeObserver | null = null

onMounted(() => {
  if (!trackRef.value) return
  observer = new ResizeObserver((entries) => {
    trackWidth.value = entries[0]?.contentRect.width ?? 0
  })
  observer.observe(trackRef.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

const segments = computed(() =>
  buildStackSegments(props.suites, props.weights, props.suiteScores, props.cells, trackWidth.value),
)

const counts = computed(() => liveCounts(props.cells))

// Dimension switch: the highlighted segment stays fully opaque, the rest
// dim to ~40% (ui-guidelines §5; dark-mode legibility of the dimmed state is
// a registered risk under observation).
function isDimmed(key: string): boolean {
  return props.highlight !== null && props.highlight !== key
}
</script>

<style scoped>
.stack-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}
.stack-track {
  display: flex;
  flex: 1;
  min-width: 0;
  height: 20px;
  background: var(--hs-brand-soft);
  border-radius: var(--hs-radius-sm);
  overflow: hidden;
}
.stack-seg {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  box-sizing: border-box;
  overflow: hidden;
  transition:
    width 0.3s ease,
    opacity var(--hs-transition);
}
/* Inter-segment separator reveals the card surface (ui-guidelines §5:
   1-2px, --hs-bg-card token, never a literal). */
.stack-seg:not(:last-child) {
  border-right: 2px solid var(--hs-bg-card);
}
.stack-seg.dimmed {
  opacity: 0.4;
}
.band-success {
  background: var(--hs-success);
}
.band-warning {
  background: var(--hs-warning);
}
.band-danger {
  background: var(--hs-danger);
}
.seg-label {
  font-size: var(--hs-text-xs);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-on-color);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
/* The compressed coverage watermark rides the in-segment label; on the
   colored segment it keeps the same ink at reduced emphasis (the
   grey-secondary strip style would be illegible on a color fill). */
.seg-watermark {
  font-weight: 400;
  opacity: 0.85;
}
.live-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  white-space: nowrap;
  flex-shrink: 0;
}
.live-failed {
  color: var(--hs-danger);
}
</style>
