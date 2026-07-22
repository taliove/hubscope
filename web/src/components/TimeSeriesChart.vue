<template>
  <el-card shadow="never" class="chart-card">
    <div class="chart-title">{{ title }}</div>
    <div ref="chartEl" class="chart" />
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import * as echarts from 'echarts'

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

// Mirror of styles/tokens.css — ECharts reads colors from JS, not CSS, so the
// palette is duplicated here. Keep these values in sync with tokens.css;
// every chart color must come from this map (ui-guidelines §3).
const CHART_COLORS = {
  brand: '#3B5BFD', // --hs-brand
  success: '#67C23A', // --el-color-success
  warning: '#E6A23C', // --el-color-warning
  danger: '#F56C6C', // --el-color-danger
  failing: '#FF4500', // --hs-status-failing
  textRegular: '#3E4450', // --hs-text-regular
  textSecondary: '#646A73', // --hs-text-secondary
  border: '#E5E6EB', // --hs-border
} as const

// Series palette: lines are assigned in order from the brand/status palette.
const SERIES_PALETTE = [
  CHART_COLORS.brand,
  CHART_COLORS.success,
  CHART_COLORS.warning,
  CHART_COLORS.danger,
  CHART_COLORS.failing,
]

function render() {
  if (!chart) return
  chart.setOption({
    color: SERIES_PALETTE,
    grid: { left: 56, right: 16, top: 32, bottom: 24 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 0, textStyle: { color: CHART_COLORS.textRegular } },
    xAxis: {
      type: 'category',
      data: props.categories,
      boundaryGap: false,
      axisLabel: { color: CHART_COLORS.textSecondary },
      axisLine: { lineStyle: { color: CHART_COLORS.border } },
    },
    yAxis: {
      type: 'value',
      name: props.yName ?? '',
      scale: true,
      nameTextStyle: { color: CHART_COLORS.textSecondary },
      axisLabel: { color: CHART_COLORS.textSecondary },
      splitLine: { lineStyle: { color: CHART_COLORS.border } },
    },
    series: props.series.map(s => ({
      name: s.name,
      type: 'line',
      data: s.data,
      showSymbol: false,
      smooth: true,
      connectNulls: true,
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

// Re-render whenever the parent swaps in new buckets.
watch(() => [props.categories, props.series], render, { deep: true })

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
