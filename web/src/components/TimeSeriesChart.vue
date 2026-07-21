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

function render() {
  if (!chart) return
  chart.setOption({
    grid: { left: 56, right: 16, top: 32, bottom: 24 },
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 0 },
    xAxis: { type: 'category', data: props.categories, boundaryGap: false },
    yAxis: { type: 'value', name: props.yName ?? '', scale: true },
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
.chart-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}
.chart {
  width: 100%;
  height: 240px;
}
</style>
