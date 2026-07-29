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
// mirrors of semantics.css). Charts re-render when the theme flips.
const colors = useChartColors()

function render() {
  if (!chart) return
  const c = colors.value
  chart.setOption({
    color: seriesPalette(c),
    grid: { left: 56, right: 16, top: 32, bottom: 24 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 0, textStyle: { color: c.textRegular } },
    xAxis: {
      type: 'category',
      data: props.categories,
      boundaryGap: false,
      axisLabel: { color: c.textSecondary },
      axisLine: { lineStyle: { color: c.border } },
    },
    yAxis: {
      type: 'value',
      name: props.yName ?? '',
      scale: true,
      nameTextStyle: { color: c.textSecondary },
      axisLabel: { color: c.textSecondary },
      splitLine: { lineStyle: { color: c.border } },
    },
    series: props.series.map(s => ({
      name: s.name,
      type: 'line',
      data: s.data,
      showSymbol: false,
      // Honesty discipline (GH #56): never interpolate across gaps — hours
      // without probes break the line instead of inventing data points, and
      // latency spikes stay visible (the curve is evidence, not decoration).
      smooth: false,
      connectNulls: false,
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
