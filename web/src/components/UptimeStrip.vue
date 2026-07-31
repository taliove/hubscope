<template>
  <!-- Group-level 24h segmented availability bar (spec 0017, GH #64;
       ui-guidelines §5 UptimeStrip registration): one flex slot per hour,
       filled so the strip reads as one continuous timeline (§2 segmented-fill
       convention). Purely presentational — the parent feeds aggregated dots
       from utils/overviewDots (probe-weighted, never a per-endpoint average).
       Width is governed by the parent: the group strip row fixes it at 220px
       left-aligned (GH #88, corrected 246 → 220 same day — check MEDIUM-1:
       hugging the typical-viewport in-card strip slot, not a pixel-exact
       match; card width is elastic), shrinkable on narrow viewports. -->
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
  /* GH #88: same segment spec as the EndpointCard in-card strip (10px) —
     GH #85's 6px/10px tiering is revoked; the "lamp" reading came from the
     segment aspect ratio, not the height. With the parent's 220px strip
     width each slot is ≈ (220 − 23×2) / 24 ≈ 7.2px — near-square dots,
     hugging the typical-viewport in-card slots (≈7.2px at 1200px; ≈6.4px
     at the 272px card floor — segment width is approximate, only height /
     color / gap / radius are strictly identical), so a group's strip reads
     as the aggregation of its cards' strips (cross-level consistency). */
  height: 10px;
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
