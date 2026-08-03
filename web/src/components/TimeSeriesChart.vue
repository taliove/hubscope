<template>
  <el-card shadow="never" class="chart-card">
    <div class="chart-title">{{ title }}</div>
    <div ref="chartEl" class="chart" />
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
// Modular echarts via the shared registered entry (utils/echarts.ts) — see
// that file for the registration-to-setOption mapping.
import { echarts } from '@/utils/echarts'
import { useChartColors, seriesPalette } from '@/utils/chartColors'
import {
  SMOOTH_SAMPLES_PER_SEGMENT,
  expandCategories,
  nearestRealIndex,
  smoothSeries,
} from '@/utils/monotoneSmooth'
import { CHART_ANIMATION_DURATION_MS, chartAnimationEnabled } from '@/utils/chartMotion'

// Generic hourly-bucket line chart used by the endpoint detail page. The
// parent supplies category labels and one data array per line; the component
// owns the ECharts instance lifecycle (init, update, resize, dispose).
const props = defineProps<{
  title: string
  categories: string[]
  series: { name: string; data: (number | null)[] }[]
  yName?: string
}>()

const chartEl = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

// Theme-aware palette (single source: utils/chartColors.ts, light/dark
// mirrors of semantics.css). Charts re-render when the theme flips. (Dark is
// deferred in v2.0 — DARK mirrors LIGHT — but the watch stays so the future
// dark spec needs no wiring change here.)
const colors = useChartColors()

// Default-axis-tooltip marker replica for the custom formatter below.
function marker(color: string): string {
  return `<span style="display:inline-block;margin-right:4px;border-radius:50%;width:8px;height:8px;background-color:${color};"></span>`
}

function render() {
  if (!chart) return
  const c = colors.value
  const realCount = props.categories.length
  // Sparse tick labels (少坐标): at most ~6 real labels, only ever on
  // measured points (interpolated slots carry empty labels).
  const labelStride = Math.max(1, Math.ceil(realCount / 6))
  chart.setOption({
    color: seriesPalette(c),
    grid: { left: 56, right: 16, top: 32, bottom: 24 },
    // Entry animation: one left-to-right draw (spec 0018, 800–1200ms band),
    // zeroed under reduced-motion via the JS-side gate (chartMotion).
    animation: chartAnimationEnabled(),
    animationDuration: CHART_ANIMATION_DURATION_MS,
    animationEasing: 'cubicOut',
    tooltip: {
      trigger: 'axis',
      // Fidelity discipline: the smoothed curve adds interpolated slots, but
      // a hover always snaps to the nearest REAL bucket and reports the true
      // measured values — an interpolated value is never presented as data.
      formatter: (params: { dataIndex: number }[] | { dataIndex: number }) => {
        if (realCount === 0) return ''
        const first = Array.isArray(params) ? params[0] : params
        if (!first) return ''
        const idx = nearestRealIndex(first.dataIndex, realCount)
        const palette = seriesPalette(c)
        const lines = props.series.flatMap((s, si) => {
          const v = s.data[idx]
          if (v === null || v === undefined) return []
          return [`${marker(palette[si % palette.length])}${s.name}：${v}`]
        })
        if (lines.length === 0) return ''
        return `${props.categories[idx]}<br/>${lines.join('<br/>')}`
      },
    },
    legend: { top: 0, right: 0, textStyle: { color: c.textRegular } },
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
      name: props.yName ?? '',
      scale: true,
      // 少坐标少网格 (spec 0018 图表纪律): no axis line, no ticks, sparse
      // dashed horizontal guides only.
      splitNumber: 4,
      nameTextStyle: { color: c.textSecondary },
      axisLabel: { color: c.textSecondary },
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { lineStyle: { color: c.border, type: 'dashed' } },
    },
    series: props.series.map(s => ({
      name: s.name,
      type: 'line',
      // Monotone smoothing (GH #114): the curve is precomputed by
      // utils/monotoneSmooth (Fritsch–Carlson — passes through every real
      // point exactly, never invents extrema); ECharts' built-in smooth can
      // overshoot, so it stays OFF and only connects the dense samples.
      data: smoothSeries(s.data),
      showSymbol: false,
      smooth: false,
      // Honesty discipline (GH #56): never interpolate across gaps — hours
      // without probes break the line (smoothSeries preserves null slots)
      // instead of inventing data points.
      connectNulls: false,
      lineStyle: { width: 2 },
    })),
  })
}

function onResize() {
  chart?.resize()
}

onMounted(() => {
  chart = echarts.init(chartEl.value!)
  render()
  window.addEventListener('resize', onResize)
})

// Re-render whenever the parent swaps in new buckets or the theme flips.
watch(() => [props.categories, props.series], render, { deep: true })
watch(colors, render)

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.chart-card {
  margin-bottom: 16px;
}
/* Public status board density: 16px card padding (ui-guidelines §2). */
.chart-card {
  --el-card-padding: 16px;
}
.chart-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 8px;
}
.chart {
  width: 100%;
  height: 240px;
}
</style>
