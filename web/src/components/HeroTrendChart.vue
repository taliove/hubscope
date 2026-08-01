<template>
  <!-- Hero 24h availability trend (GH #129, reference-design hero): the
       right half of the status hero — a large area chart of the
       probe-weighted hourly availability on a fixed 0–100% axis, colored by
       the REGISTERED three-tier caliber (ui-guidelines §3: ≥95 green /
       below warning / exactly-0-with-probes red) through a piecewise
       visualMap. No new thresholds are invented here. Anti-fake
       invariants: no-probe hours are null and break the line (GH #56,
       connectNulls:false — never bridged); the monotone smoother passes
       through every measured point exactly and never invents extrema. -->
  <div v-if="hasData" ref="chartEl" class="hero-trend-chart" />
  <!-- All-null input (no probes in 24h) renders a neutral placeholder —
       an empty axis must never read as a healthy flat line. -->
  <div v-else class="hero-trend-empty">24h 内无探测数据</div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
// Modular echarts via the shared registered entry (utils/echarts.ts) — see
// that file for the registration-to-setOption mapping.
import { echarts } from '@/utils/echarts'
import { useChartColors } from '@/utils/chartColors'
import { availabilityTierPieces } from '@/utils/overviewMetrics'
import {
  SMOOTH_SAMPLES_PER_SEGMENT,
  expandCategories,
  nearestRealIndex,
  smoothSeries,
} from '@/utils/monotoneSmooth'
import { CHART_ANIMATION_DURATION_MS, chartAnimationEnabled } from '@/utils/chartMotion'

// The parent (StatusHero) supplies the derived series — categories are
// local "HH:00" labels, values hourly availability on the 0–100 display
// scale, null for no-probe hours.
const props = defineProps<{
  categories: string[]
  values: (number | null)[]
}>()

const chartEl = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

// At least one measured hour — otherwise the placeholder renders and no
// chart is created (an empty grid would fake a story).
const hasData = computed(() => props.values.some(v => v !== null))

// Theme-aware palette (single source: utils/chartColors.ts). Dark is
// deferred in v2.0 — the watch stays so the future dark spec needs no
// wiring change here.
const colors = useChartColors()

function render() {
  if (!chart) return
  const c = colors.value
  const realCount = props.categories.length
  // Sparse tick labels (少坐标): at most ~6 real labels, only ever on
  // measured points (interpolated slots carry empty labels).
  const labelStride = Math.max(1, Math.ceil(realCount / 6))
  chart.setOption({
    grid: { left: 8, right: 44, top: 12, bottom: 24 },
    // Entry animation: one left-to-right draw (spec 0018, 800–1200ms band),
    // zeroed under reduced-motion via the JS-side gate (chartMotion).
    animation: chartAnimationEnabled(),
    animationDuration: CHART_ANIMATION_DURATION_MS,
    animationEasing: 'cubicOut',
    tooltip: {
      trigger: 'axis',
      // Fidelity discipline: the smoothed curve adds interpolated slots, but
      // a hover always snaps to the nearest REAL bucket and reports the true
      // measured value — an interpolated value is never presented as data.
      formatter: (params: { dataIndex: number }[] | { dataIndex: number }) => {
        if (realCount === 0) return ''
        const first = Array.isArray(params) ? params[0] : params
        if (!first) return ''
        const idx = nearestRealIndex(first.dataIndex, realCount)
        const label = props.categories[idx]
        const v = props.values[idx]
        const valueText = v === null || v === undefined ? '无数据' : `可用率 ${v.toFixed(1)}%`
        return `${label} 时段 · ${valueText}`
      },
    },
    // Piecewise tier coloring: the pieces carry the registered three-tier
    // caliber (overviewMetrics.availabilityTierPieces); hidden — the
    // coloring is the message, no legend of its own.
    visualMap: {
      show: false,
      type: 'piecewise',
      dimension: 1,
      seriesIndex: 0,
      pieces: availabilityTierPieces(c),
    },
    xAxis: {
      type: 'category',
      data: expandCategories(props.categories),
      boundaryGap: false,
      axisLabel: {
        color: c.textSecondary,
        interval: (index: number) => index % (SMOOTH_SAMPLES_PER_SEGMENT * labelStride) === 0,
      },
      axisLine: { lineStyle: { color: c.border } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      // Fixed 0–100% evidence axis (W7-mirror discipline: a full-scale axis
      // can never inflate a dip into a dive); sparse right-side labels.
      min: 0,
      max: 100,
      position: 'right',
      splitNumber: 2,
      axisLabel: { color: c.textSecondary, formatter: '{value}%' },
      axisLine: { show: false },
      axisTick: { show: false },
      // 少网格 (spec 0018 图表纪律): no grid lines at all in the hero.
      splitLine: { show: false },
    },
    series: [
      {
        type: 'line',
        // Monotone smoothing (GH #114): precomputed by utils/monotoneSmooth
        // (Fritsch–Carlson — passes through every real point exactly, never
        // invents extrema); ECharts' built-in smooth can overshoot, so it
        // stays OFF and only connects the dense samples.
        data: smoothSeries(props.values),
        showSymbol: false,
        smooth: false,
        // Honesty discipline (GH #56): no-probe hours break the line instead
        // of inventing data points.
        connectNulls: false,
        lineStyle: { width: 2 },
        // Same-color low-alpha area fill under the line: the visualMap piece
        // color drives both line and fill; the constant opacity keeps the
        // fill subordinate to the line (functional emphasis, not decoration).
        areaStyle: { opacity: 0.16 },
      },
    ],
  })
}

function onResize() {
  chart?.resize()
}

// Lazy init: the chart div only exists once hasData flips true (the series
// can arrive after mount — first poll landing), so init waits for the div.
async function ensureChart() {
  if (chart || !hasData.value) return
  await nextTick()
  if (!chartEl.value) return
  chart = echarts.init(chartEl.value)
  render()
}

onMounted(() => {
  void ensureChart()
  window.addEventListener('resize', onResize)
})

// Re-render whenever the parent swaps in new buckets or the theme flips;
// a false→true hasData flip mounts the chart div first.
watch(hasData, () => void ensureChart())
watch(() => [props.categories, props.values], render, { deep: true })
watch(colors, render)

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.hero-trend-chart {
  width: 100%;
  height: 168px;
}
.hero-trend-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 168px;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
}
</style>
