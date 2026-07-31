<template>
  <!-- Trend sparkline (GH #115, spec 0018 图表纪律): a light auxiliary line
       for the metric widgets — monotone interpolation (never invents
       extrema), null breaks, no axes, no grid. Neutral ink by design: the
       trendline must never compete with the widget's core number. -->
  <svg
    class="trend-sparkline"
    :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    <template v-if="paths.length > 0">
      <path
        v-for="(p, i) in paths"
        :key="i"
        class="spark-area"
        :d="p.area"
      />
      <path
        v-for="(p, i) in paths"
        :key="`l-${i}`"
        class="spark-line"
        :d="p.line"
      />
    </template>
    <!-- All-null input renders the flat placeholder track (LatencySparkline
         precedent): the slot keeps its height and never reads as data. -->
    <line v-else class="spark-empty" :x1="0" :y1="HEIGHT / 2" :x2="WIDTH" :y2="HEIGHT / 2" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { smoothSeries } from '@/utils/monotoneSmooth'

const props = withDefaults(defineProps<{ values: (number | null)[] }>(), {})

// Fixed internal coordinate system; the host stretches the SVG via CSS.
// Non-uniform scaling is acceptable here (no x-alignment invariant — the
// LatencySparkline ban on preserveAspectRatio="none" is about its dots-strip
// alignment, which this widget sparkline does not have).
const WIDTH = 120
const HEIGHT = 32
const PAD = 1 // stroke half-width headroom

interface RunPath {
  line: string
  area: string
}

// Build one line+area path pair per maximal non-null run (nulls break the
// line — no bridging across measurement holes). Y range is data-driven with
// a zero floor for count-like series and a small headroom so the peak never
// touches the edge; a flat series renders at mid-height.
const paths = computed<RunPath[]>(() => {
  const smoothed = smoothSeries(props.values, 4)
  const real = smoothed.filter((v): v is number => v !== null)
  if (real.length === 0) return []
  const min = Math.min(0, ...real)
  const max = Math.max(...real)
  const span = max - min
  const yOf = (v: number) =>
    span === 0 ? HEIGHT / 2 : HEIGHT - PAD - ((v - min) / span) * (HEIGHT - PAD * 2)
  const n = smoothed.length
  const xOf = (i: number) => (n === 1 ? WIDTH / 2 : (i / (n - 1)) * WIDTH)

  const out: RunPath[] = []
  let run: { x: number; y: number }[] = []
  const flush = () => {
    if (run.length === 0) return
    const line =
      run.length === 1
        ? `M ${run[0].x - 1} ${run[0].y} L ${run[0].x + 1} ${run[0].y}` // lone point: a 2px tick, not an invisible dot
        : `M ${run.map(p => `${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' L ')}`
    const base = HEIGHT - PAD
    const area = `${line} L ${run[run.length - 1].x.toFixed(2)} ${base} L ${run[0].x.toFixed(2)} ${base} Z`
    out.push({ line, area })
    run = []
  }
  smoothed.forEach((v, i) => {
    if (v === null) {
      flush()
    } else {
      run.push({ x: xOf(i), y: yOf(v) })
    }
  })
  flush()
  return out
})
</script>

<style scoped>
.trend-sparkline {
  display: block;
  width: 100%;
  height: 32px;
}
.spark-line {
  fill: none;
  stroke: var(--hs-text-secondary);
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}
/* Functional area emphasis, not decoration (LatencySparkline precedent):
   the surface token keeps it category-correct. */
.spark-area {
  fill: var(--hs-bg-hover);
  stroke: none;
}
.spark-empty {
  stroke: var(--hs-border);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
</style>
