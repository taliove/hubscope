<template>
  <!-- UptimeMicroStrip (2026-08-02 revived by user ruling): the in-row 24h
       segmented signal strip — 24 cells, one per hour (oldest first), tier
       colors from the overviewDots single source (ok = success / partial =
       warning / fail = danger / none = border gray, graphic grade). Tier
       mapping and per-cell tooltip wording are isomorphic to the share-card
       material strip by construction (same dotTier / dotTooltipText). Pure
       display: the row carries the drill-down, the strip only informs.
       Each cell carries an el-tooltip quick-show (200ms, the GH #86
       discipline — the native title's ~1s system delay is not controllable)
       with the hour's exact success count. -->
  <span class="uptime-strip" role="img" :aria-label="ariaLabel">
    <el-tooltip
      v-for="(dot, i) in dots"
      :key="i"
      :content="dotTooltipText(dot)"
      :show-after="200"
      placement="top"
    >
      <span class="strip-slot">
        <span class="strip-cell" :class="`seg-${dotTier(dot.total, dot.failures)}`" />
      </span>
    </el-tooltip>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { dotTier, dotTooltipText } from '@/utils/overviewDots'
import type { OverviewDot } from '@/api/types'

const props = defineProps<{ dots: OverviewDot[] }>()

// Whole-strip summary for assistive tech (the per-cell facts live on the
// cell titles): counts per tier over the 24h window.
const ariaLabel = computed(() => {
  const counts = { ok: 0, partial: 0, fail: 0, none: 0 }
  for (const dot of props.dots) {
    counts[dotTier(dot.total, dot.failures)] += 1
  }
  return `过去 24 小时逐小时可用性:正常 ${counts.ok} 小时,部分异常 ${counts.partial} 小时,全部失败 ${counts.fail} 小时,无数据 ${counts.none} 小时`
})
</script>

<style scoped>
.uptime-strip {
  display: flex;
  flex: 1 1 0;
  min-width: 0;
  gap: 2px;
  height: 14px;
}
.strip-slot {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.strip-cell {
  width: 100%;
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
</style>
