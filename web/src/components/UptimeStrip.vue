<template>
  <!-- Group-level 24h segmented availability bar (spec 0017, GH #64;
       ui-guidelines §5 UptimeStrip registration): one flex slot per hour,
       filled so the strip reads as one continuous timeline (§2 segmented-fill
       convention). Purely presentational — the parent feeds aggregated dots
       from utils/overviewDots (probe-weighted, never a per-endpoint average).
       Width is governed by the parent: the group strip row fixes it at 360px
       left-aligned (GH #87), shrinkable on narrow viewports. -->
  <div class="uptime-strip">
    <el-tooltip
      v-for="(dot, i) in dots"
      :key="dot.bucket_start || i"
      :content="dotTooltipText(dot)"
      placement="top"
      :show-after="100"
    >
      <span class="uptime-seg" :class="`seg-${dotTier(dot.total, dot.failures)}`" />
    </el-tooltip>
  </div>
</template>

<script setup lang="ts">
import type { OverviewDot } from '@/api/types'
import { dotTier, dotTooltipText } from '@/utils/overviewDots'

defineProps<{ dots: OverviewDot[] }>()
</script>

<style scoped>
.uptime-strip {
  display: flex;
  gap: 2px;
}
/* Flexible slots: 24 segments always fit, never overflow into a horizontal
   scrollbar (§4). Flex goes on the el-tooltip trigger wrappers, the segment
   fills its slot — same construction as the EndpointCard dots strip. */
.uptime-strip > * {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.uptime-seg {
  width: 100%;
  /* GH #85 slim band: group-level strips are a whole-group scan band at
     6px; the EndpointCard in-card strip stays 10px (endpoint-granularity
     evidence) — the two dot heights are deliberately tiered. */
  height: 6px;
  border-radius: var(--hs-radius-xs);
  display: inline-block;
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
</style>
