<template>
  <div class="trend-chart">
    <div class="chart-title">{{ title }}</div>
    <div ref="chartEl" class="chart" :style="{ height }" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import * as echarts from 'echarts'

// Generic bare line chart for trend views (no card wrapper — the parent
// dialog/section owns layout). Unlike TimeSeriesChart it breaks the line at
// null points by default (an unjudged batch must not read as a real score)
// and supports vertical break markers (suite-version changes).
const props = withDefaults(
  defineProps<{
    title: string
    categories: string[]
    series: { name: string; data: (number | null)[] }[]
    yName?: string
    height?: string
    connectNulls?: boolean
    markLines?: { xAxis: string; label: string }[]
    // fixYRange pins the axis to [min, max]; leave unset for latency-like
    // data. Score and success-rate charts must pin 0-100: auto-scaling would
    // render a 99.5 -> 99.0 wobble as a cliff, which reads as vendor decay.
    fixYRange?: { min: number; max: number }
  }>(),
  { yName: '', height: '220px', connectNulls: false, markLines: () => [], fixYRange: undefined }
)

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
  placeholder: '#9CA3AF', // --hs-text-placeholder
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
      name: props.yName,
      scale: props.fixYRange === undefined,
      min: props.fixYRange?.min,
      max: props.fixYRange?.max,
      nameTextStyle: { color: CHART_COLORS.textSecondary },
      axisLabel: { color: CHART_COLORS.textSecondary },
      splitLine: { lineStyle: { color: CHART_COLORS.border } },
    },
    series: props.series.map((s, i) => ({
      name: s.name,
      type: 'line',
      data: s.data,
      showSymbol: true,
      smooth: false,
      connectNulls: props.connectNulls,
      // Break markers (e.g. "v2 起题目变更") attach to the first series; they
      // are placeholder-gray dashed lines per ui-guidelines §3 — never a
      // red/green arrow, since scores across a break are not comparable.
      markLine:
        i === 0 && props.markLines.length > 0
          ? {
              symbol: 'none',
              lineStyle: { color: CHART_COLORS.placeholder, type: 'dashed' },
              label: {
                formatter: (p: { name: string }) => p.name,
                color: CHART_COLORS.textSecondary,
                fontSize: 12,
              },
              data: props.markLines.map(m => ({ xAxis: m.xAxis, name: m.label })),
            }
          : undefined,
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

// Re-render whenever the parent swaps in new data.
watch(() => [props.categories, props.series, props.markLines], render, { deep: true })

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.chart-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 8px;
}
.chart {
  width: 100%;
}
</style>
