<template>
  <!-- Metric widgets (GH #115, spec 0018 §7): four Apple-Widget-style
       cells — title, core number, sub-line, light trendline. The scalars
       are backend aggregates rendered verbatim (the dashboard never
       derives); the sparklines are display-only trends from
       utils/overviewMetrics. Core numbers tween between poll updates. -->
  <section class="metric-widgets">
    <template v-if="!skeleton">
      <div v-for="w in widgets" :key="w.key" class="widget">
        <span class="widget-title">{{ w.title }}</span>
        <span class="widget-value" :class="{ 'is-null': w.value === null }">{{ w.display }}</span>
        <span class="widget-sub" :class="w.subTone ? `tone-${w.subTone}` : ''">{{ w.sub }}</span>
        <TrendSparkline :values="w.series" />
      </div>
    </template>
    <template v-else>
      <div v-for="i in 4" :key="i" class="widget">
        <span class="skel skel-title" />
        <span class="skel skel-value" />
        <span class="skel skel-sub" />
        <span class="skel skel-spark" />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, toRef, type Ref } from 'vue'
import TrendSparkline from '@/components/TrendSparkline.vue'
import { useTweenedNumber } from '@/composables/useTweenedNumber'
import { formatCount, formatMs, formatPercent } from '@/utils/format'
import { healthDeltaText, healthDeltaTone } from '@/utils/overviewMetrics'
import type { AbnormalCounts } from '@/utils/overviewMetrics'
import { statusLabel } from '@/utils/statusDisplay'

const props = withDefaults(
  defineProps<{
    availability: number | null // 0~1, backend aggregate
    delta: number | null // day-over-day delta of the same caliber
    probes: number
    meanLatencyMs: number | null // enabled-entry P50 mean (registered caliber)
    abnormal: AbnormalCounts
    availabilitySeries: (number | null)[]
    probeSeries: number[]
    latencySeries: (number | null)[]
    failureSeries: number[]
    skeleton?: boolean
  }>(),
  { skeleton: false },
)

// Core numbers glide between poll updates (spec 0018 §15: the tween band
// covers the health index and widget core numbers only). Each widget keeps
// its own tweened ref; counts and latencies tween in their raw units.
const availabilityTween = useTweenedNumber(toRef(props, 'availability') as Ref<number | null>)
const probesRef = computed(() => (props.skeleton ? null : props.probes))
const probesTween = useTweenedNumber(probesRef as Ref<number | null>)
const latencyTween = useTweenedNumber(toRef(props, 'meanLatencyMs') as Ref<number | null>)
const abnormalRef = computed(() => (props.skeleton ? null : props.abnormal.total))
const abnormalTween = useTweenedNumber(abnormalRef as Ref<number | null>)

interface Widget {
  key: string
  title: string
  value: number | null
  display: string
  sub: string
  subTone: 'success' | 'danger' | 'neutral' | null
  series: (number | null)[]
}

const widgets = computed<Widget[]>(() => {
  const delta = healthDeltaText(props.delta)
  return [
    {
      key: 'availability',
      title: '24h 可用率',
      value: availabilityTween.value,
      display: formatPercent(availabilityTween.value),
      // The availability delta IS the health delta (same backend caliber);
      // null hides the comparison rather than inventing a flat trend.
      sub: delta || '较昨日暂无对比',
      subTone: delta ? healthDeltaTone(props.delta) : null,
      series: props.availabilitySeries,
    },
    {
      key: 'probes',
      title: '24h 请求量',
      value: probesTween.value,
      display: formatCount(probesTween.value),
      sub: '探测总次数',
      subTone: null,
      series: props.probeSeries,
    },
    {
      key: 'latency',
      title: '平均延迟',
      value: latencyTween.value,
      display: formatMs(latencyTween.value),
      // Caliber annotation (anti-fake): the only scope-consistent frontend
      // mean (batch-59 registration) — never a second derivation.
      sub: '启用端点 P50 均值',
      subTone: null,
      series: props.latencySeries,
    },
    {
      key: 'abnormal',
      title: '风险模型数',
      value: abnormalTween.value,
      display: abnormalTween.value === null ? '-' : String(Math.round(abnormalTween.value)),
      sub:
        props.abnormal.total === 0
          ? `全部${statusLabel('stable')}`
          : `${statusLabel('incident')} ${props.abnormal.incident} · ${statusLabel('degraded')} ${props.abnormal.degraded}`,
      subTone: props.abnormal.total === 0 ? 'neutral' : props.abnormal.incident > 0 ? 'danger' : 'neutral',
      series: props.failureSeries,
    },
  ]
})
</script>

<style scoped>
.metric-widgets {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--hs-space-4);
  margin-bottom: var(--hs-space-5);
}
/* Light containers (Apple syntax): white surface, 1px border, radius-lg;
   the hover lift (2–4px, spec 0018 §15) is the only shadow moment. */
.widget {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
  padding: var(--hs-space-4);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
  transition:
    transform var(--hs-transition),
    box-shadow var(--hs-transition);
}
.widget:hover {
  transform: translateY(-2px);
  box-shadow: var(--hs-shadow-md);
}
.widget-title {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.widget-value {
  font-size: var(--hs-text-3xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
  font-variant-numeric: tabular-nums;
}
.widget-value.is-null {
  color: var(--hs-text-placeholder);
}
.widget-sub {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  min-height: 18px; /* stable widget height with or without a delta line */
}
.tone-success {
  color: var(--hs-success-text);
}
.tone-danger {
  color: var(--hs-danger-text);
}
.skel {
  display: block;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
}
.skel-title {
  width: 72px;
  height: 13px;
}
.skel-value {
  width: 110px;
  height: 32px;
}
.skel-sub {
  width: 120px;
  height: 12px;
}
.skel-spark {
  width: 100%;
  height: 32px;
}
</style>
