<template>
  <div class="latency-row">
    <span class="latency-label">延迟</span>
    <span ref="stripRef" class="latency-strip">
      <span class="sparkline-wrap">
        <svg
          v-if="hasData && stripWidth > 0"
          class="sparkline"
          :width="stripWidth"
          :height="ROW_HEIGHT"
          aria-hidden="true"
        >
          <line
            v-if="showThreshold"
            class="baseline-line"
            x1="0"
            :x2="stripWidth"
            :y1="baselineLineY"
            :y2="baselineLineY"
          />
          <template v-for="(segment, index) in segments" :key="index">
            <path v-if="segment.areaPath !== null" class="spark-area" :d="segment.areaPath" />
            <circle
              v-if="segment.isolated"
              class="spark-point"
              :cx="segment.points[0].x"
              :cy="segment.points[0].y"
              r="1.5"
            />
            <polyline v-else class="spark-line" :points="polylinePoints(segment)" />
          </template>
        </svg>
        <span v-else class="spark-empty-track" />
        <span class="hover-overlay">
          <el-tooltip
            v-for="dot in dots"
            :key="dot.bucket_start"
            :content="bucketTooltip(dot)"
            placement="top"
            :show-after="100"
          >
            <span class="hover-slot" />
          </el-tooltip>
        </span>
      </span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { OverviewDot } from '@/api/types'
import { formatBucketTime, formatMs } from '@/utils/format'
import {
  buildLatencySegments,
  sparklineYMax,
  thresholdVisible,
  thresholdY,
  type SparklineSegment,
} from '@/utils/latencySparkline'

// Per-card 24h latency sparkline (design review final form): a neutral P50
// curve with a light area fill under the dots strip, hour-aligned with it by
// construction (same 24-slot flex + 2px gap geometry, see
// utils/latencySparkline.ts). The curve carries NO status semantics —
// coloring stays with the dots. The y range is data-driven (peak x 1.25,
// floored at 1s); the dashed line marks the latency degradation threshold
// (2x the status machine's 7-day P50 baseline) and appears only when it fits
// the range — otherwise the tooltip's baseline value carries the number.
const props = defineProps<{ dots: OverviewDot[]; baselineMs: number | null }>()

const ROW_HEIGHT = 28

// The SVG cannot stretch a vector curve, so the strip width is measured and
// the geometry recomputed on resize (ResizeObserver, ScoreCell precedent).
const stripRef = ref<HTMLElement | null>(null)
const stripWidth = ref(0)
let observer: ResizeObserver | null = null

onMounted(() => {
  if (!stripRef.value) return
  stripWidth.value = stripRef.value.clientWidth
  observer = new ResizeObserver(entries => {
    stripWidth.value = entries[0]?.contentRect.width ?? 0
  })
  observer.observe(stripRef.value)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

const hasData = computed(() => props.dots.some(d => d.p50_ms !== null))
const yMax = computed(() => sparklineYMax(props.dots.map(d => d.p50_ms)))
const segments = computed<SparklineSegment[]>(() => {
  if (!hasData.value || stripWidth.value <= 0) return []
  return buildLatencySegments(props.dots.map(d => d.p50_ms), stripWidth.value, ROW_HEIGHT, yMax.value)
})
const showThreshold = computed(() => thresholdVisible(props.baselineMs, yMax.value))
const baselineLineY = computed(() =>
  props.baselineMs !== null ? thresholdY(props.baselineMs, ROW_HEIGHT, yMax.value) : 0,
)

function polylinePoints(segment: SparklineSegment): string {
  return segment.points.map(p => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ')
}

// "HH:mm 时段 · P50 X · 基线 Y"; a bucket without successful probes has no
// latency data at all, and the baseline clause is omitted only when the
// status machine has no baseline. The tooltip ALWAYS carries the 1x
// baseline value — it is the fallback when the dashed threshold line does
// not fit the y range (thresholdVisible).
function bucketTooltip(dot: OverviewDot): string {
  const label = `${formatBucketTime(dot.bucket_start)} 时段`
  if (dot.p50_ms === null) return `${label} · 无数据`
  const baseline = props.baselineMs !== null ? ` · 基线 ${formatMs(props.baselineMs)}` : ''
  return `${label} · P50 ${formatMs(dot.p50_ms)}${baseline}`
}
</script>

<style scoped>
/* Row construction mirrors .card-dots in EndpointCard.vue: fixed-width
   label + flexible strip. The label width (26px) is shared with the dots
   label so both strips start at the same x. */
.latency-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
}
.latency-label {
  width: 26px;
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.latency-strip {
  flex: 1;
  min-width: 0;
}
.sparkline-wrap {
  position: relative;
  display: block;
  width: 100%;
  height: 28px;
}
.sparkline {
  position: absolute;
  inset: 0;
  display: block;
}
.spark-line {
  fill: none;
  stroke: var(--hs-text-secondary);
  stroke-width: 1.5;
  stroke-linejoin: round;
  stroke-linecap: round;
}
.spark-area {
  /* Light area fill under the curve: solid surface token, no opacity
     stacking; breaks at null buckets together with the polyline. */
  fill: var(--hs-bg-hover);
}
.spark-point {
  fill: var(--hs-text-secondary);
}
.baseline-line {
  stroke: var(--hs-warning);
  stroke-width: 1;
  stroke-dasharray: 4 3;
}
.spark-empty-track {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  height: 1px;
  background: var(--hs-border);
}
/* Transparent hover layer: same 24-slot flex + 2px gap construction as the
   dots strip (the gap literal is the twin of SPARKLINE_GAP_PX in
   utils/latencySparkline.ts — keep them in sync), so each tooltip zone
   covers exactly its bucket. */
.hover-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  gap: 2px;
}
.hover-overlay > * {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.hover-slot {
  width: 100%;
  height: 100%;
  display: inline-block;
}
</style>
