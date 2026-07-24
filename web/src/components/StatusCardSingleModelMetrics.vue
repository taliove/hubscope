<template>
  <div>
    <!-- Hero panel (single-model mode, design ruling): availability leads at
         display size; a single-status statement replaces the aggregate
         verdict + distribution (the scope is one endpoint — counts would be
         noise). Failing keeps its static double-encoding (orange dot + the
         count-less "含告警" chip). The right column carries eval data; the
         panel stays put (with a no-data line) when the model was never
         evaluated. The failing chip copy comes from the statement object
         (null unless failing), so no status wording lives in the template. -->
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
        <div class="statement">
          <span v-if="statement.failingChip" class="alert-dot" />
          <span class="statement-text" :class="`vc-${statement.tone}`">{{ statement.text }}</span>
          <span v-if="statement.failingChip" class="failing-chip">{{ statement.failingChip }}</span>
        </div>
        <div class="metric-block latency-block">
          <span class="metric-label">
            平均延迟
            <span v-if="entry.p50_ms === null" class="metric-note">24h 内无探测数据</span>
          </span>
          <span class="metric-latency" :class="{ 'av-none': entry.p50_ms === null }">
            {{ formatMs(entry.p50_ms) }}
          </span>
        </div>
      </div>

      <div class="metric-divider" />

      <div class="hero-right">
        <template v-if="evalSummary">
          <div class="metric-block">
            <span class="metric-label">评估总分</span>
            <span class="metric-score">{{ formatScore(evalSummary.total_score) }}</span>
          </div>
          <div v-if="suiteScores.length > 0" class="capability-tags">
            <el-tag v-for="suite in suiteScores" :key="suite.suite_id" size="small">
              {{ suite.suite_name }} {{ formatScore(suite.score) }}
            </el-tag>
            <el-tag v-if="overflowCount > 0" size="small" type="info">+{{ overflowCount }}</el-tag>
          </div>
        </template>
        <template v-else>
          <span class="eval-empty">暂无评估数据</span>
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
// Single-model hero panel for StatusCard (ticket 60.5, design-ruling rework):
// probe metrics (availability + statement + latency) on the left, eval
// metrics (total score + capability suite tags, capped at 6 with a +N
// counter) on the right, followed by the 24h segmented bar. Mounted by
// StatusCard only in single-model mode; the aggregate StatusCardMetrics is
// untouched.
import { computed } from 'vue'
import type { OverviewEntry, ModelEvalSummary } from '@/api/types'
import { formatPercentDigits, formatMs, formatScore } from '@/utils/format'
import { dotTier, availabilityTier, scopedAvailability, singleModelStatement } from '@/utils/statusCardSummary'

const props = defineProps<{
  entry: OverviewEntry
  evalSummary: ModelEvalSummary | null
}>()

// Compute availability from the entry's 24h dots
const availability = computed(() => scopedAvailability([props.entry]))
const statement = computed(() => singleModelStatement(props.entry, availability.value))

// Suite tags cap at 6; the overflow collapses into a +N counter chip so a
// wide capability set cannot blow up the export height.
const MAX_SUITE_TAGS = 6
const suiteScores = computed(() => props.evalSummary?.suite_scores.slice(0, MAX_SUITE_TAGS) ?? [])
const overflowCount = computed(() => (props.evalSummary?.suite_scores.length ?? 0) - suiteScores.value.length)
</script>

<style scoped>
/* Same neutral ground as the aggregate hero panel (ui-guidelines §5). */
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
/* Single-status statement: same visual weight as the aggregate verdict
   line, tone-colored so severity stays legible at a glance. */
.statement {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  font-size: var(--hs-text-sm);
}
.statement-text {
  font-weight: 600;
}
.vc-healthy {
  color: var(--hs-success);
}
.vc-degraded {
  color: var(--hs-warning);
}
.vc-abnormal {
  color: var(--hs-danger);
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
.latency-block {
  margin-top: 12px;
}
.latency-block .metric-label {
  margin-bottom: 4px;
}
.metric-latency {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.metric-score {
  margin-top: 4px;
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
.eval-empty {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
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
