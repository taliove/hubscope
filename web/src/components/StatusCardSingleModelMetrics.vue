<template>
  <div>
    <!-- Metrics panel: left side shows probe metrics (availability + latency),
         right side shows eval metrics (total score + capability tags) -->
    <div class="metrics-panel">
      <div class="metrics-left">
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
        <div class="metric-block">
          <span class="metric-label">
            平均延迟
            <span v-if="entry.p50_ms === null" class="metric-note">24h 内无探测数据</span>
          </span>
          <span class="metric-value" :class="{ 'av-none': entry.p50_ms === null }">
            {{ formatMs(entry.p50_ms) }}
          </span>
        </div>
      </div>

      <div class="metric-divider" />

      <div class="metrics-right">
        <template v-if="evalSummary">
          <div class="metric-block">
            <span class="metric-label">评估总分</span>
            <span class="metric-big">{{ formatScore(evalSummary.total_score) }}</span>
          </div>
          <div class="capability-tags">
            <el-tag v-for="suite in evalSummary.suite_scores" :key="suite.suite_id" size="small">
              {{ suite.suite_name }} {{ formatScore(suite.score) }}
            </el-tag>
          </div>
        </template>
        <template v-else>
          <div class="metric-block">
            <span class="metric-label empty">暂无评估数据</span>
          </div>
        </template>
      </div>
    </div>

    <!-- 24h segmented bar: per-hour availability visualization -->
    <div class="uptime-section">
      <div class="uptime-strip">
        <span v-for="(dot, i) in entry.dots_24h" :key="i" class="uptime-slot">
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
// Single-model metrics card for StatusCard (ticket 60.5): displays probe
// metrics (availability + latency) on the left, eval metrics (score + tags)
// on the right, followed by the 24h segmented bar.
import { computed } from 'vue'
import type { OverviewEntry, ModelEvalSummary } from '@/api/types'
import { formatPercentDigits, formatMs, formatScore } from '@/utils/format'
import { dotTier, availabilityTier, scopedAvailability } from '@/utils/statusCardSummary'

const props = defineProps<{
  entry: OverviewEntry
  evalSummary: ModelEvalSummary | null
}>()

// Compute availability from the entry's 24h dots
const availability = computed(() => scopedAvailability([props.entry]))
</script>

<style scoped>
.metrics-panel {
  display: flex;
  align-items: stretch;
  background: var(--hs-bg-page);
  border-radius: var(--hs-radius-lg);
  padding: 16px 20px;
  margin-bottom: 16px;
}
.metrics-left {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.metrics-right {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.metric-divider {
  width: 1px;
  background: var(--hs-border);
  margin: 0 20px;
}
.metric-block {
  display: flex;
  flex-direction: column;
}
.metric-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  margin-bottom: 4px;
}
.metric-label.empty {
  text-align: center;
  font-size: var(--hs-text-md);
  color: var(--hs-text-placeholder);
  margin-bottom: 0;
}
.metric-note {
  margin-left: 8px;
  color: var(--hs-text-placeholder);
}
.metric-big {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.metric-unit {
  font-size: var(--hs-text-md);
  font-weight: 400;
  color: var(--hs-text-secondary);
  margin-left: 2px;
}
.metric-value {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.capability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
/* Availability tier colors (ui-guidelines §3) */
.av-ok {
  color: var(--hs-success);
}
.av-partial {
  color: var(--hs-warning);
}
.av-fail {
  color: var(--hs-danger);
}
.av-none {
  color: var(--hs-text-placeholder);
}
/* 24h segmented bar */
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
  background: var(--hs-success);
}
.seg-partial {
  background: var(--hs-warning);
}
.seg-fail {
  background: var(--hs-danger);
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
