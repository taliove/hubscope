<template>
  <!-- SuiteLegend displays a color legend for the eval board's suite colors
       (ticket 77). Shows one badge per suite with its color and name. Consumed
       by Leaderboard above the rows and by EvalCard in exported material. -->
  <div class="legend">
    <span class="legend-label">图例：</span>
    <div class="legend-items">
      <div v-for="(suite, index) in suites" :key="suite.key" class="legend-item">
        <span class="legend-color" :class="`color-suite-${index + 1}`"></span>
        <span class="legend-name">{{ suite.name }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ReportSuite } from '@/api/types'

defineProps<{
  suites: ReportSuite[]
}>()
</script>

<style scoped>
.legend {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.legend-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  font-weight: 600;
}
.legend-items {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.legend-item {
  display: flex;
  align-items: center;
  gap: 4px;
}
.legend-color {
  width: 12px;
  height: 12px;
  border-radius: var(--hs-radius-xs);
  flex-shrink: 0;
}
.legend-name {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-regular);
}
/* Suite color classes (must match ScoreStackBar). */
.color-suite-1 {
  background: var(--hs-suite-1);
}
.color-suite-2 {
  background: var(--hs-suite-2);
}
.color-suite-3 {
  background: var(--hs-suite-3);
}
.color-suite-4 {
  background: var(--hs-suite-4);
}
.color-suite-5 {
  background: var(--hs-suite-5);
}
.color-suite-6 {
  background: var(--hs-suite-6);
}
</style>
