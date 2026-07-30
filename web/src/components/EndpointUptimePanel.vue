<template>
  <!-- The interactive surface's single "24h uptime strip + latency curve"
       unit (ui-guidelines §5 EndpointUptimePanel): consumed by EndpointCard
       and EndpointQuickViewDialog — never inline a third copy. The StatusCard
       static material keeps its own implementation by exemption (a snapshot
       render contract, ticket 76 precedent). -->
  <div class="card-dots">
    <span class="dots-label">24h</span>
    <span class="dots-strip">
      <el-tooltip
        v-for="dot in dots"
        :key="dot.bucket_start"
        :content="dotTooltip(dot)"
        placement="top"
        :show-after="100"
      >
        <span class="dot" :class="dotClass(dot)" />
      </el-tooltip>
    </span>
  </div>

  <LatencySparkline :dots="dots" :baseline-ms="baselineMs" />
</template>

<script setup lang="ts">
import type { OverviewDot } from '@/api/types'
import LatencySparkline from './LatencySparkline.vue'
import { formatBucketTime } from '@/utils/format'

// Geometry, tooltip wording and dot tiering moved here verbatim from
// EndpointCard with the extraction (2026-07-29) — one copy only.
defineProps<{ dots: OverviewDot[]; baselineMs: number | null }>()

// 24h stability dot coloring: gray = no probes, green = success rate ≥95%,
// red = all failed, yellow = below 95%.
function dotClass(dot: OverviewDot): string {
  if (dot.total === 0) return 'dot-none'
  if (dot.failures === dot.total) return 'dot-fail'
  return (dot.total - dot.failures) / dot.total >= 0.95 ? 'dot-ok' : 'dot-partial'
}

function dotTooltip(dot: OverviewDot): string {
  const label = `${formatBucketTime(dot.bucket_start)} 时段`
  if (dot.total === 0) return `${label} · 无数据`
  return `${label} · 成功 ${dot.total - dot.failures}/${dot.total}`
}
</script>

<style scoped>
.card-dots {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
}
.dots-label {
  /* Fixed label width shared with LatencySparkline's 延迟 label so both
     strips start at the same x. */
  width: 26px;
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.dots-strip {
  display: flex;
  align-items: center;
  /* The 2px gap is the twin of SPARKLINE_GAP_PX in utils/latencySparkline.ts
     (the sparkline derives its bucket centers from it) — keep them in sync. */
  gap: 2px;
  flex: 1;
  min-width: 0;
}
/* Flexible slots: 24 dots always fit the card, never overflow into a
   horizontal scrollbar. Flex goes on the direct children (el-tooltip
   trigger wrappers), the dot fills its slot. */
.dots-strip > * {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.dot {
  /* Segmented uptime bar: each slot fills its flex cell so the strip reads
     as one continuous 24h timeline (status-page convention), not loose dots. */
  width: 100%;
  height: 10px;
  border-radius: var(--hs-radius-xs);
  display: inline-block;
}
.dot-none {
  background: var(--hs-border);
}
.dot-ok {
  background: var(--hs-success);
}
.dot-partial {
  background: var(--hs-warning);
}
.dot-fail {
  background: var(--hs-danger);
}
</style>
