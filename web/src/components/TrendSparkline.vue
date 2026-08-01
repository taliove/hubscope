<template>
  <!-- Trend sparkline (GH #115, spec 0018 the chart discipline): a light auxiliary
       trend for the metric widgets — no axes, no grid. `variant` (GH #137):
       'line' (default) = monotone interpolation (never invents extrema) with
       null breaks; 'bars' = equal-width columns (request-volume lane), one
       bar per bucket, null buckets stay empty slots. `tone` (GH #130, user
       ruling 2026-08-01, superseding the T5 neutral-ink ruling): neutral
       keeps the original low-key ink; semantic tones tint line + area (bars
       variant: the bar fill) with the functional color so each widget reads
       its own semantic lane. The core number stays ink — the trendline must
       never compete with it. -->
  <svg
    class="trend-sparkline"
    :class="tone !== 'neutral' ? `tone-${tone}` : ''"
    :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    <template v-if="variant === 'bars'">
      <template v-if="bars">
        <path v-for="(b, i) in bars" :key="i" class="spark-bar" :d="b" />
      </template>
      <!-- Empty/all-zero series: same placeholder discipline as the line
           variant — the slot keeps its height and never reads as data. -->
      <line v-else class="spark-empty" :x1="0" :y1="HEIGHT / 2" :x2="WIDTH" :y2="HEIGHT / 2" />
    </template>
    <template v-else-if="paths.length > 0">
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
import { sparklineBarLayout, topRoundedBarPath } from '@/utils/sparklineBars'

// Semantic lane of the sparkline (GH #130): neutral (default, original ink)
// keeps every other consumer unchanged; the four widget lanes consume the
// functional-color base (graphic tier, 3:1 — never the *-text steps).
export type SparklineTone = 'neutral' | 'success' | 'brand' | 'warning' | 'danger'

// Rendering variant (GH #137): 'line' (default) keeps every existing
// consumer unchanged; 'bars' renders equal-width columns for the
// request-volume widget (a count series reads as a column chart, not a
// curve).
export type SparklineVariant = 'line' | 'bars'

const props = withDefaults(
  defineProps<{ values: (number | null)[]; tone?: SparklineTone; variant?: SparklineVariant }>(),
  { tone: 'neutral', variant: 'line' },
)

// Fixed internal coordinate system; the host stretches the SVG via CSS.
// Non-uniform scaling is acceptable here (no x-alignment invariant — the
// LatencySparkline ban on preserveAspectRatio="none" is about its dots-strip
// alignment, which this widget sparkline does not have).
const WIDTH = 120
const HEIGHT = 32
const PAD = 1 // stroke half-width headroom

// Bars-variant geometry (GH #137): 2px gap and 2px top-corner radius — the
// dots-strip family language (radius-xs). Bars are column paths with rounded
// top corners; the bottom edge stays square so columns sit flush on the
// track. The bar fill carries the lane color, so no PAD headroom is needed
// (unlike the stroked line, nothing paints outside the column box).
const BARS_GAP = 2
const BARS_RADIUS = 2
const bars = computed<string[] | null>(() => {
  const layout = sparklineBarLayout(props.values, WIDTH, HEIGHT, BARS_GAP)
  if (!layout) return null
  return layout.map(b => topRoundedBarPath(b.x, b.y, b.width, b.height, BARS_RADIUS))
})

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
/* Semantic tones (GH #130): the line takes the functional base color; the
   area is the same color at low opacity (0.15, inside the 0.12–0.2 band)
   via fill-opacity — no extra tint tokens needed. */
.tone-success .spark-line {
  stroke: var(--hs-success);
}
.tone-success .spark-area {
  fill: var(--hs-success);
  fill-opacity: 0.15;
}
.tone-brand .spark-line {
  stroke: var(--hs-brand);
}
.tone-brand .spark-area {
  fill: var(--hs-brand);
  fill-opacity: 0.15;
}
.tone-warning .spark-line {
  stroke: var(--hs-warning);
}
.tone-warning .spark-area {
  fill: var(--hs-warning);
  fill-opacity: 0.15;
}
.tone-danger .spark-line {
  stroke: var(--hs-danger);
}
.tone-danger .spark-area {
  fill: var(--hs-danger);
  fill-opacity: 0.15;
}
/* Bars variant (GH #137): solid columns in the lane color — the bar fill
   IS the trend shape, so it takes the same functional base the line stroke
   would (graphic tier); neutral keeps the original low-key ink. */
.spark-bar {
  fill: var(--hs-text-secondary);
}
.tone-success .spark-bar {
  fill: var(--hs-success);
}
.tone-brand .spark-bar {
  fill: var(--hs-brand);
}
.tone-warning .spark-bar {
  fill: var(--hs-warning);
}
.tone-danger .spark-bar {
  fill: var(--hs-danger);
}
.spark-empty {
  stroke: var(--hs-border);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
</style>
