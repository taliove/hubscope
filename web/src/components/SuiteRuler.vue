<template>
  <!-- SuiteRuler displays suite labels aligned with the stacked bar segments
       (ticket 77). Each label sits above its corresponding segment, proportional
       to its weighted width. Consumed by Leaderboard above the rows. -->
  <div class="ruler-wrap">
    <!-- Left spacer: matches the rank + model name + family tag width -->
    <div class="ruler-spacer"></div>
    <!-- Ruler track: segments mirror the ScoreStackBar widths -->
    <div class="ruler-track">
      <div
        v-for="seg in segments"
        :key="seg.key"
        class="ruler-seg"
        :style="{ width: seg.widthPct + '%' }"
      >
        <span v-if="seg.showLabel" class="ruler-label" :title="seg.name">{{ seg.name }}</span>
      </div>
    </div>
    <!-- Right spacer: matches the score + delta columns -->
    <div class="ruler-spacer-right"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ReportSuite } from '@/api/types'
import { effectiveWeight } from '@/utils/stackSegments'

const props = defineProps<{
  suites: ReportSuite[]
  weights: Record<string, number>
  // Reference row for width calculation: we use the first row's scores to
  // compute proportional widths. In practice all rows have the same suite
  // set, so any row works as a reference.
  referenceScores: Record<string, number | null>
}>()

interface RulerSegment {
  key: string
  name: string
  widthPct: number
  showLabel: boolean
}

// Build ruler segments: proportional widths matching the ScoreStackBar logic,
// but we don't need score values — only the weight-normalized width.
const segments = computed((): RulerSegment[] => {
  const scored = props.suites.filter((s) => props.referenceScores[s.key] != null)
  const wsum = scored.reduce((acc, s) => acc + effectiveWeight(props.weights, s.key), 0)
  if (wsum <= 0) return []

  return scored.map((s) => {
    const score = props.referenceScores[s.key] as number
    const weight = effectiveWeight(props.weights, s.key)
    const widthPct = (score * weight) / wsum
    // Show label if segment is wide enough (simplified: always show for now)
    const showLabel = widthPct > 8 // roughly 8% of track width
    return {
      key: s.key,
      name: s.name,
      widthPct,
      showLabel,
    }
  })
})
</script>

<style scoped>
.ruler-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 4px;
  padding: 0 4px;
}
.ruler-spacer {
  /* Matches: rank(24px) + gap(12px) + model(220px) + gap(12px) + tag + gap(12px) */
  width: calc(24px + 12px + 220px + 12px + 60px + 12px);
  flex-shrink: 0;
}
.ruler-track {
  display: flex;
  flex: 1;
  min-width: 0;
  height: 20px;
  align-items: center;
}
.ruler-seg {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  box-sizing: border-box;
  overflow: hidden;
}
.ruler-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  text-align: center;
  padding: 0 4px;
}
.ruler-spacer-right {
  /* Matches: score(56px) + gap(12px) + delta(72px) */
  width: calc(56px + 12px + 72px);
  flex-shrink: 0;
}
</style>
