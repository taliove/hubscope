<template>
  <div>
    <!-- Hero panel: the 24h availability leads (the objective 3-second
         number, tier-colored), with the verdict and full three-state
         distribution riding underneath as a secondary line (down + failing
         merge into 异常, GH #113). The top never foregrounds the
         abnormal endpoints — but the verdict and counts are still right
         there, so the anti-fake invariant (never paper over an abnormal
         state) holds. The alert count keeps its static event-worded chip. -->
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
            <span class="dist-label" :class="seg.count > 0 ? `st-${seg.tone}` : ''">{{ seg.label }}</span>
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
  compact?: boolean // GH #93: 480px compact variant
}>()

const counts = computed<HealthCounts>(() => countByStatus(props.entries))
const tone = computed<HealthTone | 'empty'>(() => (props.isEmpty ? 'empty' : toneOf(counts.value)))
const availability = computed(() => scopedAvailability(props.entries))
const avgLatency = computed(() => meanP50Ms(props.entries))
const dots = computed(() => aggregateDots24h(props.entries))
// Verdict rides under the availability number; '' when empty so the panel
// stays neutral on the no-data state (never reads as "全部稳定运行").
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
  /* Horizontal 20px is the material canvas constant (share-materials brief),
     not grid spacing; vertical values consume --hs-space-* (GH #95). */
  padding: var(--hs-space-4) 20px;
  margin-bottom: var(--hs-space-4);
}
/* Compact-variant overrides for this panel live in StatusCard.vue as
   `.compact :deep(...)` — scoped ids flow parent-to-child only, so
   `:deep(.compact)` here was a structurally dead selector (GH #121 check
   HIGH-1; two GH #93-era rules had the same disease). */
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
  /* GH #95: the 2px baseline nudge retires to the nearest on-grid value. */
  padding-bottom: var(--hs-space-1);
}
.metric-divider {
  width: 1px;
  /* Hairline tier (GH #121 line-lightening, same as GH #118). */
  background: var(--hs-border-light);
  margin: 0 20px;
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
  /* v2 hero tier (GH #121, spec 0018 §14: core numbers 72+) — the same
     typesetting as the overview StatusHero (weight 600, tight tracking,
     tabular digits); the legacy display tier is retired. */
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
.metric-latency {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
/* Verdict secondary line: smaller than the availability lead, but colored
   by tone so the severity is still legible at a glance. Text channel → the
   *-text grade of each slot (graphic/text division, GH #113). */
.hero-verdict {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  margin-top: var(--hs-space-2);
  font-size: var(--hs-text-sm);
}
.verdict-text {
  font-weight: 600;
}
/* GH #69 text/graphics split: verdict/distribution/availability words are
   text — success as text consumes the deepened text grade. */
.vc-healthy {
  color: var(--hs-success-text);
}
.vc-degraded {
  color: var(--hs-warning-text);
}
.vc-abnormal {
  color: var(--hs-danger-text);
}
.vc-empty {
  color: var(--hs-text-secondary);
}
/* Alert chip + dot (event-worded "含 N 个告警"): failing has no separate
   display color in the three-state world — both take the danger slot. */
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
/* Distribution line: three display segments always listed; a zero segment
   fades to placeholder so a clean dimension is confirmed, not inferred. */
.distribution {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  margin-top: var(--hs-space-1);
  font-size: var(--hs-text-xs);
}
.dist-seg + .dist-seg::before {
  content: '·';
  margin: 0 var(--hs-space-1);
  color: var(--hs-text-placeholder);
}
.dist-label {
  font-weight: 600;
  margin-right: var(--hs-space-1);
}
.dist-num {
  color: var(--hs-text-primary);
}
.dist-zero .dist-label,
.dist-zero .dist-num {
  color: var(--hs-text-placeholder);
  font-weight: 400;
}
/* Distribution words: text channel → *-text grades (GH #69 text/graphics
   split; GH #113 tone slots success/warning/danger). */
.st-success {
  color: var(--hs-success-text);
}
.st-warning {
  color: var(--hs-warning-text);
}
.st-danger {
  color: var(--hs-danger-text);
}
/* Availability tier of the hero number and the rate figures: text channel →
 * the *-text grade of each slot (GH #69 text/graphics split; on the v2
 * palette the warning/danger bases are graphic-tier and fail as text,
 * GH #121). The segment strips below keep the bases as graphic fills. */
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
/* Aggregated 24h segmented bar: filled slots reading as one continuous
   timeline (§2 segmented-fill rule), one size up from the in-page 10px. */
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
