<template>
  <!-- Status hero (GH #115, spec 0018 §6; GH #129 reference-design rebuild):
       the first visual anchor of the overview page — left column carries the
       「AI 服务健康指数」 label, the 72px figure, the soft conclusion chip,
       the day-over-day delta and the statistical-scope annotation; the right
       column carries the refresh meta (top-right) and the 24h availability
       trend chart. Anti-fake invariants: a null index renders the neutral
       暂无数据 (NEVER a fabricated 100%) and suppresses the conclusion chip;
       the delta line renders only when the backend provides it; the scope
       line always names the population. -->
  <section class="status-hero" :class="{ 'is-empty': health === null }">
    <template v-if="!skeleton">
      <div class="hero-left">
        <span class="hero-label">AI 服务健康指数</span>
        <div class="hero-main">
          <template v-if="health !== null">
            <span class="hero-digits">{{ digits }}</span>
            <span class="hero-unit">%</span>
          </template>
          <span v-else class="hero-empty">暂无数据</span>
        </div>
        <div class="hero-side">
          <!-- Soft conclusion chip (GH #129): soft functional base + *-text
               word; never rendered in the no-data state. -->
          <span v-if="health !== null" class="hero-conclusion" :class="`tone-${conclusionTone}`">
            {{ conclusion }}
          </span>
          <span v-if="deltaText" class="hero-delta" :class="`tone-${deltaTone}`">{{ deltaText }}</span>
          <span class="hero-scope">{{ scope }}</span>
        </div>
      </div>
      <div class="hero-right">
        <span v-if="updatedAt" class="hero-meta">{{ metaText }}</span>
        <HeroTrendChart :categories="trendCategories" :values="trendValues" />
      </div>
    </template>
    <template v-else>
      <!-- First-load skeleton, height-anchored to the loaded layout so the
           list below never jumps when data lands. -->
      <div class="hero-left">
        <span class="skel skel-label" />
        <div class="hero-main">
          <span class="skel skel-hero" />
        </div>
        <div class="hero-side">
          <span class="skel skel-chip" />
          <span class="skel skel-line skel-line-short" />
        </div>
      </div>
      <div class="hero-right">
        <span class="skel skel-meta" />
        <span class="skel skel-chart" />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import HeroTrendChart from '@/components/HeroTrendChart.vue'
import { useTweenedNumber } from '@/composables/useTweenedNumber'
import { formatClockTime, formatPercentDigits } from '@/utils/format'
import { healthDeltaText, healthDeltaTone } from '@/utils/overviewMetrics'

const props = withDefaults(
  defineProps<{
    health: number | null // 0~1, straight from the backend aggregate
    delta: number | null
    conclusion: string
    conclusionTone: 'success' | 'warning' | 'danger' | 'neutral'
    scope: string
    // 24h trend inputs (derived upstream by overviewMetrics.heroTrendSeries):
    // local "HH:00" labels + hourly availability on the 0–100 display scale.
    trendCategories?: string[]
    trendValues?: (number | null)[]
    // Refresh meta: the poll interval comes from the useOverview constant
    // (passed down — never re-declared here), the timestamp from the last
    // overview response.
    updatedAt?: string | null
    refreshIntervalMs?: number
    skeleton?: boolean
  }>(),
  // Optional-with-empty-defaults (GH #129 check CRITICAL-1): a consumer that
  // predates the trend wiring must stay green — no trend props renders the
  // chart's neutral placeholder, a null updatedAt hides the meta line.
  { skeleton: false, trendCategories: () => [], trendValues: () => [], updatedAt: null, refreshIntervalMs: 0 },
)

// The hero figure tweens between poll updates (spec 0018 §15: 500–800ms,
// core numbers only); reduced-motion settles instantly inside the engine.
const tweened = useTweenedNumber(toRef(props, 'health'))
const digits = computed(() => formatPercentDigits(tweened.value))

const deltaText = computed(() => healthDeltaText(props.delta))
const deltaTone = computed(() => healthDeltaTone(props.delta))

// 「自动刷新：10s · 最后更新 HH:mm:ss」 — the interval derives from the
// prop (useOverview's POLL_INTERVAL_MS), never a second literal.
const metaText = computed(
  () => `自动刷新：${Math.round(props.refreshIntervalMs / 1000)}s · 最后更新 ${formatClockTime(props.updatedAt)}`,
)
</script>

<style scoped>
.status-hero {
  display: flex;
  align-items: flex-start;
  gap: var(--hs-space-8);
  padding: var(--hs-space-6) 0 var(--hs-space-5);
  /* Height anchor (loaded and skeleton share it): both columns measure
     194px — right: meta 18 + gap 8 + chart 168; left: label 19.5 + gap 8 +
     digits 72 + gap 8 + side ≈ 86.5 (padding-top 4 + chip 27.5 + 8 +
     delta 21 + 8 + scope 18). Vertical padding 44px → 238px floor. */
  min-height: 238px;
}
.hero-left {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
  /* Fixed narrow stats column (reference design ≈1:3): 320px of the
     1200px content width (content-box, no global reset): 1200 − 320
     − 64 (space-8 gap) = 816px ≈ 1:2.55; the trend chart takes the rest. */
  flex: 0 0 320px;
  min-width: 0;
}
.hero-label {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.hero-main {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
  line-height: 1;
}
.hero-digits {
  font-size: var(--hs-text-hero);
  font-weight: 600;
  letter-spacing: -0.02em;
  color: var(--hs-text-primary);
  font-variant-numeric: tabular-nums;
}
.hero-unit {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  color: var(--hs-text-secondary);
}
/* No-data reads as neutral ink, never as a number (anti-fake: 无数据 ≠
   全部正常). */
.hero-empty {
  font-size: var(--hs-text-3xl);
  font-weight: 600;
  color: var(--hs-text-placeholder);
}
.hero-side {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--hs-space-2);
  padding-top: var(--hs-space-1);
}
/* Conclusion chip: pill shape (radius-full), soft functional base with the
   *-text grade word (graphic/text division); the chip is self-sizing so the
   soft base hugs the word. */
.hero-conclusion {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  padding: var(--hs-space-1) var(--hs-space-3);
  border-radius: var(--hs-radius-full);
}
.hero-conclusion.tone-success {
  background: var(--hs-success-soft);
}
.hero-conclusion.tone-warning {
  background: var(--hs-warning-soft);
}
.hero-conclusion.tone-danger {
  background: var(--hs-danger-soft);
}
.hero-delta {
  font-size: var(--hs-text-md);
  font-variant-numeric: tabular-nums;
}
.hero-scope {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
/* Conclusion and delta words consume the *-text grade (graphic/text
   division); neutral stays secondary. */
.tone-success {
  color: var(--hs-success-text);
}
.tone-warning {
  color: var(--hs-warning-text);
}
.tone-danger {
  color: var(--hs-danger-text);
}
.tone-neutral {
  color: var(--hs-text-secondary);
}
/* Right column: the trend chart flex-fills everything the fixed 320px
   stats column leaves (≈68% of the hero width at 1200px content); the
   refresh meta sits top-right above it. */
.hero-right {
  flex: 1 1 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.hero-meta {
  align-self: flex-end;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  font-variant-numeric: tabular-nums;
}
.skel {
  display: block;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
}
.skel-label {
  width: 140px;
  height: 14px;
}
.skel-hero {
  width: 220px;
  height: 72px;
}
.skel-chip {
  width: 120px;
  height: 24px;
  border-radius: var(--hs-radius-full);
}
.skel-line {
  width: 180px;
  height: 16px;
}
.skel-line-short {
  width: 120px;
  height: 12px;
}
.skel-meta {
  align-self: flex-end;
  width: 200px;
  height: 12px;
}
.skel-chart {
  width: 100%;
  height: 168px;
  border-radius: var(--hs-radius-lg);
}
</style>
