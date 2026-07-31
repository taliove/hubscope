<template>
  <!-- Status hero (GH #115, spec 0018 §6): the first visual anchor of the
       overview page — the 72px health-index figure, its day-over-day delta,
       the conclusion word and the statistical-scope annotation.
       Anti-fake invariants: a null index renders the neutral 暂无数据
       (NEVER a fabricated 100%); the delta line renders only when the
       backend provides it; the scope line always names the population. -->
  <section class="status-hero" :class="{ 'is-empty': health === null }">
    <template v-if="!skeleton">
      <div class="hero-main">
        <template v-if="health !== null">
          <span class="hero-digits">{{ digits }}</span>
          <span class="hero-unit">%</span>
        </template>
        <span v-else class="hero-empty">暂无数据</span>
      </div>
      <div class="hero-side">
        <span class="hero-conclusion" :class="`tone-${conclusionTone}`">{{ conclusion }}</span>
        <span v-if="deltaText" class="hero-delta" :class="`tone-${deltaTone}`">{{ deltaText }}</span>
        <span class="hero-scope">{{ scope }}</span>
      </div>
    </template>
    <template v-else>
      <!-- First-load skeleton, height-anchored to the loaded layout so the
           list below never jumps when data lands. -->
      <div class="hero-main">
        <span class="skel skel-hero" />
      </div>
      <div class="hero-side">
        <span class="skel skel-line" />
        <span class="skel skel-line skel-line-short" />
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useTweenedNumber } from '@/composables/useTweenedNumber'
import { formatPercentDigits } from '@/utils/format'
import { healthDeltaText, healthDeltaTone } from '@/utils/overviewMetrics'

const props = withDefaults(
  defineProps<{
    health: number | null // 0~1, straight from the backend aggregate
    delta: number | null
    conclusion: string
    conclusionTone: 'success' | 'warning' | 'danger' | 'neutral'
    scope: string
    skeleton?: boolean
  }>(),
  { skeleton: false },
)

// The hero figure tweens between poll updates (spec 0018 §15: 500–800ms,
// core numbers only); reduced-motion settles instantly inside the engine.
const tweened = useTweenedNumber(toRef(props, 'health'))
const digits = computed(() => formatPercentDigits(tweened.value))

const deltaText = computed(() => healthDeltaText(props.delta))
const deltaTone = computed(() => healthDeltaTone(props.delta))
</script>

<style scoped>
.status-hero {
  display: flex;
  align-items: flex-end;
  gap: var(--hs-space-6);
  padding: var(--hs-space-6) 0 var(--hs-space-5);
  /* Height anchor (loaded and skeleton share it): hero 72px line ≈ 86px,
     side block ≈ 3 lines ≈ 66px, vertical padding 56px → ≈ 142px floor. */
  min-height: 142px;
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
  gap: var(--hs-space-1);
  padding-bottom: var(--hs-space-2);
}
.hero-conclusion {
  font-size: var(--hs-text-xl);
  font-weight: 600;
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
.skel {
  display: block;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
}
.skel-hero {
  width: 220px;
  height: 72px;
}
.skel-line {
  width: 180px;
  height: 16px;
}
.skel-line-short {
  width: 120px;
  height: 12px;
}
</style>
