<template>
  <!-- Row-level 24h micro strip (GH #115): the time-shape cell of the model
       list — 24 hour-aligned cells, oldest first. Tier mapping and tooltip
       wording come from utils/overviewDots (the single source shared with
       the share material), so the strip is consistent by construction. -->
  <div class="uptime-micro-strip">
    <el-tooltip
      v-for="(dot, i) in dots"
      :key="i"
      :content="dotTooltipText(dot)"
      :show-after="200"
      placement="top"
    >
      <span class="cell" :class="`tier-${dotTier(dot.total, dot.failures)}`" />
    </el-tooltip>
  </div>
</template>

<script setup lang="ts">
import type { OverviewDot } from '@/api/types'
import { dotTier, dotTooltipText } from '@/utils/overviewDots'

withDefaults(defineProps<{ dots: OverviewDot[] }>(), {})
</script>

<style scoped>
.uptime-micro-strip {
  display: flex;
  gap: 2px;
  width: 100%;
  min-width: 0;
}
/* Filled-segment cell (the time-bar exception of the radius scale);
   tier colors are the graphic-grade functional tokens. */
.cell {
  flex: 1 1 0;
  min-width: 0;
  height: 14px;
  border-radius: var(--hs-radius-xs);
  background: var(--hs-border); /* tier-none: no probes this hour */
}
.tier-ok {
  background: var(--hs-success);
}
.tier-partial {
  background: var(--hs-warning);
}
.tier-fail {
  background: var(--hs-danger);
}
</style>
