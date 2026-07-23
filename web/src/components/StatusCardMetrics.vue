<template>
  <div>
    <!-- Metrics panel: the 3-second numbers. Availability digits get the
         biggest type on the card; latency stays one step smaller (primary
         vs secondary). Null renders a dash plus a no-data note. -->
    <div class="metrics-panel">
      <div class="metric-block">
        <span class="metric-label">
          24h 可用率
          <span v-if="availability === null" class="metric-note">24h 内无探测数据</span>
        </span>
        <span class="metric-big" :class="`av-${availabilityTier(availability)}`">
          <template v-if="availability !== null">
            {{ formatPercentDigits(availability) }}<span class="metric-unit">%</span>
          </template>
          <template v-else>-</template>
        </span>
      </div>
      <div class="metric-divider" />
      <div class="metric-block">
        <span class="metric-label">
          平均延迟
          <span v-if="avgLatency === null" class="metric-note">24h 内无探测数据</span>
        </span>
        <span class="metric-latency" :class="{ 'av-none': avgLatency === null }">{{ formatMs(avgLatency) }}</span>
      </div>
    </div>

    <!-- Aggregated 24h segmented bar: per-hour probe-weighted sums across
         the scoped entries (never a per-endpoint average). -->
    <div class="uptime-section">
      <div class="uptime-strip">
        <span v-for="(dot, i) in dots" :key="i" class="uptime-slot">
          <span class="uptime-seg" :class="`seg-${dotTier(dot.total, dot.failures)}`" />
        </span>
      </div>
      <div class="uptime-axis">
        <span>24 小时前</span>
        <span>现在</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// StatusCard metrics blocks (ticket 59): the big-number panel plus the
// aggregated 24h segmented bar. Purely presentational — all values are
// precomputed by the parent from the scoped snapshot.
import type { OverviewDot } from '@/api/types'
import { formatPercentDigits, formatMs } from '@/utils/format'
import { availabilityTier, dotTier } from '@/utils/statusCardSummary'

defineProps<{
  availability: number | null // 0~1 probe-weighted, null when no probes
  avgLatency: number | null // mean of per-endpoint p50, null when no data
  dots: OverviewDot[] // aggregated 24 buckets, oldest first
}>()
</script>

<style scoped>
/* Metrics panel: light-gray ground lifting the two key numbers off the
   white card body, with a hairline divider for the instrument feel. */
.metrics-panel {
  display: flex;
  align-items: stretch;
  background: var(--hs-bg-page);
  border-radius: var(--hs-radius);
  padding: 16px 20px;
  margin-bottom: 16px;
}
.metric-block {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.metric-divider {
  width: 1px;
  background: var(--hs-border);
  margin: 0 20px;
}
.metric-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.metric-note {
  margin-left: 8px;
  color: var(--hs-text-placeholder);
}
.metric-big {
  margin-top: 4px;
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  line-height: 1.2;
}
.metric-unit {
  font-size: var(--hs-text-md);
  font-weight: 400;
  color: var(--hs-text-secondary);
  margin-left: 2px;
}
.metric-latency {
  margin-top: 4px;
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.av-ok {
  color: var(--el-color-success);
}
.av-partial {
  color: var(--el-color-warning);
}
.av-fail {
  color: var(--el-color-danger);
}
.av-none {
  color: var(--hs-text-placeholder);
}
/* Aggregated 24h segmented bar: filled slots reading as one continuous
   timeline (§2 segmented-fill rule), one size up from the in-page 10px. */
.uptime-section {
  margin-bottom: 24px;
}
.uptime-strip {
  display: flex;
  gap: 2px;
}
.uptime-slot {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.uptime-seg {
  width: 100%;
  height: 16px;
  border-radius: var(--hs-radius-xs);
}
.seg-ok {
  background: var(--el-color-success);
}
.seg-partial {
  background: var(--el-color-warning);
}
.seg-fail {
  background: var(--el-color-danger);
}
.seg-none {
  background: var(--hs-border);
}
.uptime-axis {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
</style>
