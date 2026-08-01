<template>
  <div>
    <!-- Hero panel (single-model mode, design ruling): availability leads at
         display size; a single-status statement replaces the aggregate
         verdict + distribution (the scope is one endpoint — counts would be
         noise). The alert event keeps its static chip (event-worded
         "含告警", danger slot — failing has no fourth display color, GH #113).
         The right column carries eval data; the panel stays put (with a
         no-data line) when the model was never evaluated. The chip copy
         comes from the statement object (null unless failing), so no status
         wording lives in the template. -->
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
  compact?: boolean // GH #93: 480px compact variant
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
  background: var(--hs-bg-subtle);
  border-radius: var(--hs-radius-lg);
  /* Horizontal 20px is the material canvas constant (share-materials brief),
     not grid spacing; vertical values consume --hs-space-* (GH #95). */
  padding: var(--hs-space-4) 20px;
  margin-bottom: var(--hs-space-4);
}
/* Compact variant (GH #93): tighter padding. */
:deep(.compact) .hero-panel {
  padding: var(--hs-space-3) var(--hs-space-4);
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
  gap: var(--hs-space-2);
}
.metric-divider {
  width: 1px;
  /* Hairline tier (GH #121 line-lightening, same as GH #118). */
  background: var(--hs-border-light);
  margin: 0 20px;
}
/* Compact variant: narrower divider margin. */
:deep(.compact) .metric-divider {
  margin: 0 var(--hs-space-3);
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
  margin-left: var(--hs-space-2);
  color: var(--hs-text-placeholder);
}
.hero-big {
  margin-top: var(--hs-space-1);
  /* v2 hero tier (GH #121, spec 0018 §14): same typesetting as the overview
     StatusHero (72px, weight 600, tight tracking, tabular digits); the
     legacy display tier is retired. This panel only ever mounts at the full
     720 width (single-model + compact is the endpoint small card, a
     separate template in StatusCard.vue), so no compact tier is needed. */
  font-size: var(--hs-text-hero);
  font-weight: 600;
  line-height: 1.2;
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
}
.metric-unit {
  font-size: var(--hs-text-xl);
  font-weight: 400;
  color: var(--hs-text-secondary);
  margin-left: var(--hs-space-1);
}
/* Single-status statement: same visual weight as the aggregate verdict
   line, tone-colored so severity stays legible at a glance. */
.statement {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  margin-top: var(--hs-space-2);
  font-size: var(--hs-text-sm);
}
.statement-text {
  font-weight: 600;
}
/* Statement words: text channel → the *-text grade of each slot (GH #69
   text/graphics split; graphic/text division, GH #113). */
.vc-healthy {
  color: var(--hs-success-text);
}
.vc-degraded {
  color: var(--hs-warning-text);
}
.vc-abnormal {
  color: var(--hs-danger-text);
}
/* Alert chip + dot (event-worded "含告警"): failing has no separate display
   color in the three-state world — both take the danger slot. */
.alert-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-danger);
}
.failing-chip {
  font-size: var(--hs-text-xs);
  color: var(--hs-danger-text);
  border: 1px solid var(--hs-danger-text);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-card);
  padding: 0 var(--hs-space-2);
}
.latency-block {
  margin-top: var(--hs-space-3);
}
.latency-block .metric-label {
  margin-bottom: var(--hs-space-1);
}
.metric-latency {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.metric-score {
  margin-top: var(--hs-space-1);
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.capability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--hs-space-2);
}
.eval-empty {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
/* Availability tier colors: text channel → the *-text grade of each slot
 * (GH #69 text/graphics split; on the v2 palette the warning/danger bases
 * are graphic-tier and fail as text, GH #121). The segment strips keep the
 * bases as graphic fills. */
.av-ok {
  color: var(--hs-success-text);
}
.av-partial {
  color: var(--hs-warning-text);
}
.av-fail {
  color: var(--hs-danger-text);
}
.av-none {
  color: var(--hs-text-placeholder);
}
/* 24h segmented bar */
.uptime-section {
  margin-bottom: var(--hs-space-5);
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
  margin-top: var(--hs-space-1);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
</style>
