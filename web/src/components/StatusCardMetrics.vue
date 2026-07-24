<template>
  <div>
    <!-- Hero panel: the 24h availability leads (the objective 3-second
         number, tier-colored), with the verdict and full four-status
         distribution riding underneath as a secondary line. The top never
         foregrounds the abnormal endpoints — but the verdict and counts are
         still right there, so the anti-fake invariant (never paper over an
         abnormal state) holds. Failing keeps its static double-encoding
         (orange-red dot + chip). -->
    <div class="hero-panel">
      <div class="hero-left">
        <span class="metric-label">
          24h 可用率
          <span v-if="availability === null" class="metric-note">24h 内无探测数据</span>
        </span>
        <span class="hero-big" :class="`av-${availabilityTier(availability)}`">
          <template v-if="availability !== null">
            {{ formatPercentDigits(availability) }}<span class="metric-unit">%</span>
          </template>
          <template v-else>-</template>
        </span>
        <div v-if="verdict || hasFailing" class="hero-verdict">
          <span v-if="hasFailing" class="alert-dot" />
          <span v-if="verdict" class="verdict-text" :class="`vc-${tone}`">{{ verdict }}</span>
          <span v-if="hasFailing" class="failing-chip">含 {{ counts.failing }} 个告警</span>
        </div>
        <div v-if="!isEmpty" class="distribution">
          <span
            v-for="seg in distribution"
            :key="seg.status"
            class="dist-seg"
            :class="{ 'dist-zero': seg.count === 0 }"
          >
            <span class="dist-label" :class="seg.count > 0 ? `st-${seg.status}` : ''">{{ seg.label }}</span>
            <span class="dist-num">{{ seg.count }}</span>
          </span>
        </div>
      </div>
      <div class="metric-divider" />
      <div class="hero-right">
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
// StatusCard hero + metrics (ticket 59, iterated): the availability-led
// panel that replaced the old tone-tinted conclusion block, plus the
// aggregated 24h segmented bar. Purely presentational — all values are
// computed from the scoped enabled-entry set via the statusCardSummary
// pure functions.
import { computed } from 'vue'
import type { OverviewEntry } from '@/api/types'
import { formatPercentDigits, formatMs } from '@/utils/format'
import {
  countByStatus,
  toneOf,
  conclusionText,
  type HealthTone,
  type HealthCounts,
} from '@/utils/healthConclusion'
import {
  aggregateDots24h,
  availabilityTier,
  distributionSegments,
  dotTier,
  meanP50Ms,
  scopedAvailability,
} from '@/utils/statusCardSummary'

const props = defineProps<{
  entries: OverviewEntry[] // scoped ENABLED entries only
  isEmpty: boolean
}>()

const counts = computed<HealthCounts>(() => countByStatus(props.entries))
const tone = computed<HealthTone | 'empty'>(() => (props.isEmpty ? 'empty' : toneOf(counts.value)))
const availability = computed(() => scopedAvailability(props.entries))
const avgLatency = computed(() => meanP50Ms(props.entries))
const dots = computed(() => aggregateDots24h(props.entries))
// Verdict rides under the availability number; '' when empty so the panel
// stays neutral on the no-data state (never reads as "全部正常").
const verdict = computed(() => (props.isEmpty ? '' : conclusionText(toneOf(counts.value), counts.value, false)))
const hasFailing = computed(() => !props.isEmpty && counts.value.failing > 0)
const distribution = computed(() => distributionSegments(counts.value))
</script>

<style scoped>
/* Hero panel: neutral light-gray ground (no status tint — the availability
   number's tier color carries severity, so the top is not alarm-coded). */
.hero-panel {
  display: flex;
  align-items: stretch;
  background: var(--hs-bg-page);
  border-radius: var(--hs-radius-lg);
  padding: 16px 20px;
  margin-bottom: 16px;
}
.hero-left {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.hero-right {
  flex: 0 0 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  padding-bottom: 2px;
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
.hero-big {
  margin-top: 4px;
  font-size: var(--hs-text-display);
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
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
/* Verdict secondary line: smaller than the availability lead, but colored
   by tone so the severity is still legible at a glance. */
.hero-verdict {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  font-size: var(--hs-text-sm);
}
.verdict-text {
  font-weight: 600;
}
.vc-healthy {
  color: var(--el-color-success);
}
.vc-degraded {
  color: var(--el-color-warning);
}
.vc-abnormal {
  color: var(--el-color-danger);
}
.vc-empty {
  color: var(--hs-text-secondary);
}
.alert-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-status-failing);
}
.failing-chip {
  font-size: var(--hs-text-xs);
  color: var(--hs-status-failing);
  border: 1px solid var(--hs-status-failing);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-card);
  padding: 0 6px;
}
/* Distribution line: four segments always listed; a zero segment fades to
   placeholder so "no failing" is confirmed, not inferred. */
.distribution {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  margin-top: 4px;
  font-size: var(--hs-text-xs);
}
.dist-seg + .dist-seg::before {
  content: '·';
  margin: 0 6px;
  color: var(--hs-text-placeholder);
}
.dist-label {
  font-weight: 600;
  margin-right: 3px;
}
.dist-num {
  color: var(--hs-text-primary);
}
.dist-zero .dist-label,
.dist-zero .dist-num {
  color: var(--hs-text-placeholder);
  font-weight: 400;
}
.st-healthy {
  color: var(--el-color-success);
}
.st-degraded {
  color: var(--el-color-warning);
}
.st-down {
  color: var(--el-color-danger);
}
.st-failing {
  color: var(--hs-status-failing);
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
