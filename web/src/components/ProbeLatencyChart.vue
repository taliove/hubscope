<template>
  <!-- 24h per-probe latency detail curve (2026-07-30, dashboard surface brief
       「24h 延迟明细」区; ui-guidelines §5 EndpointQuickViewDialog 条;
       2026-07-30 视觉三修 — user verdict 「毛毛虫圆点太丑」): a bare chart (no
       card wrapper — the parent section owns layout, TrendChart precedent).
       Success probes connect as a bare 1.5px neutral line (no point symbols);
       measurement holes break the line (null insertion, connectNulls:false);
       failed probes render as full-height danger markArea windows (their
       latency is a time-to-failure and never enters the y range). The x axis
       is a TIME axis — probe intervals are not constant; a category axis
       would forge an even spacing. The block ALWAYS renders `height` tall
       (empty state included) so the dialog's async regions keep
       skeleton === terminal height (no align-center gravity jump). -->
  <div v-if="records.length === 0" class="chart-empty" :style="{ height }">
    24h 内无探测数据
  </div>
  <div v-else ref="chartEl" class="chart" :style="{ height }" />
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
// Modular echarts via the shared registered entry (utils/echarts.ts) — the
// MarkAreaComponent registration there exists for this component's failure
// windows.
import { echarts } from '@/utils/echarts'
import { useChartColors } from '@/utils/chartColors'
import { buildProbeLatencySeries } from '@/utils/probeLatencyChart'
import { CHART_ANIMATION_DURATION_MS, chartAnimationEnabled } from '@/utils/chartMotion'
import { formatClockTime, formatMs } from '@/utils/format'
import type { ProbeRecord } from '@/api/types'

const props = withDefaults(defineProps<{ records: ProbeRecord[]; height?: string }>(), {
  height: '180px',
})

const chartEl = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

// Theme-aware palette (light/dark mirrors, utils/chartColors.ts): re-render
// on theme flip like every chart. The failure band reuses the existing
// danger mirror value (no new color); its 0.12 opacity band reads on both
// themes because the light/dark danger values are the same.
const colors = useChartColors()

// The pure transform owns the semantics (chronological order, success-only
// line with gap breaks, merged failure windows, data-driven y range) — the
// component only renders.
const seriesData = computed(() => buildProbeLatencySeries(props.records))

function clockOf(msEpoch: number): string {
  return formatClockTime(new Date(msEpoch).toISOString())
}

function render() {
  if (!chart) return
  const c = colors.value
  const s = seriesData.value
  chart.setOption({
    grid: { left: 56, right: 16, top: 28, bottom: 24 },
    // Entry animation: one left-to-right draw (spec 0018, 800–1200ms band),
    // zeroed under reduced-motion via the JS-side gate (chartMotion).
    animation: chartAnimationEnabled(),
    animationDuration: CHART_ANIMATION_DURATION_MS,
    animationEasing: 'cubicOut',
    legend: {
      // 2026-07-30 视觉三修裁决登记: the legend lists only the latency line.
      // The failure band is a markArea (not a series) and cannot carry a
      // legend toggle — a non-functional legend entry would be fake UI, so
      // the old 失败 item is deleted; the band's meaning is carried by its
      // own hover tooltip (「HH:mm–HH:mm · 失败 N 次」).
      top: 0,
      right: 0,
      textStyle: { color: c.textSecondary },
      data: ['延迟'],
    },
    tooltip: {
      trigger: 'item',
      formatter: (p: { seriesName: string; data: [number, number | null] }) => {
        const time = clockOf(p.data[0])
        if (p.data[1] === null) return `${time} · 无探测数据`
        return `${time} · 延迟 ${formatMs(p.data[1])}`
      },
    },
    xAxis: {
      type: 'time',
      axisLabel: { color: c.textSecondary },
      axisLine: { lineStyle: { color: c.border } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: Math.ceil(s.yMaxMs),
      name: 'ms',
      // 少坐标少网格 (spec 0018 图表纪律): no axis line, no ticks, sparse
      // dashed horizontal guides only.
      splitNumber: 4,
      nameTextStyle: { color: c.textSecondary },
      axisLabel: { color: c.textSecondary },
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { lineStyle: { color: c.border, type: 'dashed' } },
    },
    series: [
      {
        // Neutral stroke: the curve carries NO status semantics (sparkline
        // discipline) — status lives on the card edge and the StatusBadge.
        // Bare line, no per-point symbols (2026-07-30 视觉三修 ①).
        name: '延迟',
        type: 'line',
        data: s.points.map(p => [p.time, p.latencyMs]),
        showSymbol: false,
        connectNulls: false,
        // Deliberately UNSMOOTHED (GH #114 deviation, recorded): the
        // TimeSeriesChart/TrendChart monotone smoothing is not applied here —
        // ① this chart's tooltip is item-triggered per probe, so interpolated
        //    mid-probe samples would hover as invented readings (fidelity
        //    discipline violation that the category charts avoid by snapping
        //    to real slots); ② per-probe spikes are incident evidence, the
        //    curve is evidence not decoration. Styling-only migration.
        smooth: false,
        lineStyle: { color: c.textSecondary, width: 1.5 },
        itemStyle: { color: c.textSecondary },
        // Area fill (2026-07-30 救援批第二轮, user screenshot verdict 「梳齿
        // 噪音」): on low-success endpoints the sparse per-probe points jitter
        // into dense near-vertical segments — a solid bg-hover fill makes the
        // comb read as one textured surface instead of scribble. Same
        // precedent as LatencySparkline's `fill: var(--hs-bg-hover)`
        // (ui-guidelines §5: 功能性面积强调,非装饰;填充随段断): SOLID color
        // from the chart mirror, NO gradient (BrandMark is the site's only
        // gradient), no opacity stacking; null gap breaks stop the fill
        // together with the line. Render-only — the series data carries no
        // injected fill points (covered by probeLatencyChart.test.ts).
        areaStyle: { color: c.bgHover },
        markArea: {
          // Failure windows (2026-07-30 视觉三修 ③, replaces the rug scatter):
          // full-height danger bands at 0.12 opacity, no border. silent:false
          // so the band carries its own tooltip; dataIndex maps back to
          // s.failureWindows by construction (same array order). The band
          // uses the RENDER bounds (bandStart/bandEnd — lone failures are
          // widened to a minimum visible width); the tooltip reads the TRUE
          // bounds (start/end/count stay faithful, utils/probeLatencyChart).
          silent: false,
          itemStyle: { color: c.danger, opacity: 0.12 },
          data: s.failureWindows.map(w => [{ xAxis: w.bandStart }, { xAxis: w.bandEnd }]),
          tooltip: {
            show: true,
            formatter: (p: { dataIndex: number }) => {
              const w = s.failureWindows[p.dataIndex]
              return `${clockOf(w.start)}–${clockOf(w.end)} · 失败 ${w.count} 次`
            },
          },
        },
      },
    ],
  })
}

function onResize() {
  chart?.resize()
}

onMounted(() => {
  if (chartEl.value) {
    chart = echarts.init(chartEl.value)
    render()
  }
  window.addEventListener('resize', onResize)
})

watch(seriesData, render)
watch(colors, render)

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.chart {
  width: 100%;
}
.chart-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
  border: 1px dashed var(--hs-border);
  border-radius: var(--hs-radius-sm);
}
</style>
