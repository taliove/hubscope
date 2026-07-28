<template>
  <el-card shadow="never" class="endpoint-card" :class="`card-${entry.status}`" @click="goDetail">
    <div class="card-head">
      <span class="model-id" :title="entry.model_id">{{ entry.model_id }}</span>
      <el-tag :type="entry.protocol === 'anthropic' ? 'success' : 'warning'" size="small">
        {{ entry.protocol }}
      </el-tag>
    </div>

    <div class="card-status">
      <el-tooltip :content="entry.status_reason" placement="top" :show-after="200">
        <span class="status-wrap">
          <StatusBadge :status="entry.status" :causes="entry.degrade_causes" />
        </span>
      </el-tooltip>
      <el-tooltip placement="top" :show-after="200">
        <template #content>
          <div class="score-reasons">{{ scoreTooltip }}</div>
        </template>
        <span class="score-badge" :class="scoreClass">{{ scoreText }}</span>
      </el-tooltip>
      <el-tag v-if="!entry.enabled" type="info" size="small">已停用</el-tag>
    </div>

    <div class="card-metrics">
      <div class="metric">
        <span class="metric-label">24h 成功率</span>
        <span class="metric-value">{{ formatPercent(entry.success_rate_24h) }}</span>
      </div>
      <div class="metric">
        <span class="metric-label">P50</span>
        <span class="metric-value">{{ formatMs(entry.p50_ms) }}</span>
      </div>
      <div class="metric">
        <span class="metric-label">P95</span>
        <span class="metric-value">{{ formatMs(entry.p95_ms) }}</span>
      </div>
    </div>

    <div class="card-dots">
      <span class="dots-label">24h</span>
      <span class="dots-strip">
        <el-tooltip
          v-for="dot in entry.dots_24h"
          :key="dot.bucket_start"
          :content="dotTooltip(dot)"
          placement="top"
          :show-after="100"
        >
          <span class="dot" :class="dotClass(dot)" />
        </el-tooltip>
      </span>
    </div>

    <div class="card-foot">最近探测:{{ formatTime(entry.last_probe_at) }}</div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { OverviewEntry, OverviewDot } from '@/api/types'
import StatusBadge from './StatusBadge.vue'
import { formatPercent, formatMs, formatTime, formatBucketTime } from '@/utils/format'

// One card of the status matrix: a single Endpoint with its 24h summary.
// Clicking navigates to the endpoint detail page.
const props = defineProps<{ entry: OverviewEntry }>()
const router = useRouter()

function goDetail() {
  router.push(`/endpoints/${props.entry.endpoint_id}`)
}

// Stability score badge: colored by score band, gray when there is no data.
const scoreText = computed(() => (props.entry.score === null ? '暂无评分' : String(props.entry.score)))
const scoreClass = computed(() => {
  const score = props.entry.score
  if (score === null) return 'score-none'
  if (score >= 90) return 'score-good'
  if (score >= 70) return 'score-warn'
  return 'score-bad'
})
const scoreTooltip = computed(() => {
  if (props.entry.score === null) return '暂无探测数据,无法评分'
  const reasons = props.entry.score_reasons
  return reasons.length > 0 ? reasons.join('\n') : '无扣分项'
})

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
.endpoint-card {
  /* Status indicator: a 3px vertical bar on the leading edge (eyes scan
     left to right, status comes first). Color mapping unchanged (§3). */
  border-left: 3px solid transparent;
  cursor: pointer;
  transition: box-shadow 0.15s ease;
}
.endpoint-card:hover {
  box-shadow: var(--hs-shadow-md);
}
.card-healthy {
  border-left-color: var(--hs-success);
}
.card-degraded {
  border-left-color: var(--hs-warning);
}
.card-down {
  border-left-color: var(--hs-danger);
}
.card-failing {
  border-left-color: var(--hs-status-failing);
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}
.model-id {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card-status {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}
.status-wrap {
  cursor: help;
}
.score-badge {
  font-size: var(--hs-text-xs);
  font-weight: 600;
  padding: 1px 6px;
  border-radius: var(--hs-radius-sm);
  cursor: help;
  white-space: nowrap;
}
.score-good {
  color: var(--hs-success);
  background: var(--hs-success-soft);
}
.score-warn {
  color: var(--hs-warning);
  background: var(--hs-warning-soft);
}
.score-bad {
  color: var(--hs-danger);
  background: var(--hs-danger-soft);
}
.score-none {
  color: var(--hs-text-placeholder);
  background: var(--hs-info-soft);
  font-weight: 400;
}
.score-reasons {
  white-space: pre-line;
}
.card-metrics {
  display: flex;
  gap: 16px;
  margin-bottom: 12px;
}
.metric {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.metric-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.metric-value {
  font-size: var(--hs-text-md);
  line-height: 1.2;
  color: var(--hs-text-primary);
  margin-top: 2px;
}
.card-dots {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
}
.dots-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.dots-strip {
  display: flex;
  align-items: center;
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
.card-foot {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
</style>
