<template>
  <!-- Whole-card quick-view trigger (2026-07-29 /impeccable animate, surface
       brief a11y 节): the card opens the EndpointQuickViewDialog in place, so
       it exposes role="button" + aria-haspopup="dialog"; Enter/Space run the
       same open as click. The card holds no nested interactive controls
       (tooltips only), so the button role is safe. data-endpoint-id lets the
       dashboard return focus after the dialog closes (polling re-renders must
       not strand keyboard users). -->
  <el-card
    shadow="never"
    class="endpoint-card"
    :class="`card-${entry.status}`"
    role="button"
    tabindex="0"
    aria-haspopup="dialog"
    :data-endpoint-id="entry.endpoint_id"
    @click="emit('open', entry)"
    @keydown.enter="emit('open', entry)"
    @keydown.space.prevent="emit('open', entry)"
  >
    <div class="card-head">
      <span class="model-id" :title="entry.model_id">
        <span class="model-head">{{ modelSplit.head }}</span>
        <span v-if="modelSplit.tail" class="model-tail">{{ modelSplit.tail }}</span>
      </span>
      <el-tag v-if="showProtocolTag" :type="protocolTagType(entry.protocol)" size="small">
        {{ entry.protocol }}
      </el-tag>
    </div>

    <div class="card-status">
      <el-tooltip :content="entry.status_reason" placement="top" :show-after="200">
        <span class="status-wrap">
          <StatusBadge :status="entry.status" :causes="entry.degrade_causes" size="md" />
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
        <span class="metric-value metric-p50">{{ formatMs(entry.p50_ms) }}</span>
      </div>
      <div class="metric">
        <span class="metric-label">P95</span>
        <span class="metric-value metric-p95">{{ formatMs(entry.p95_ms) }}</span>
      </div>
    </div>

    <EndpointUptimePanel :dots="entry.dots_24h" :baseline-ms="entry.baseline_p50_ms" />

    <div class="card-foot">最近探测:{{ formatTime(entry.last_probe_at) }}</div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OverviewEntry } from '@/api/types'
import StatusBadge from './StatusBadge.vue'
import EndpointUptimePanel from './EndpointUptimePanel.vue'
import { formatPercent, formatMs, formatTime } from '@/utils/format'
import { protocolTagType } from '@/utils/protocol'
import { splitMiddle } from '@/utils/truncate'

// One card of the status matrix: a single Endpoint with its 24h summary.
// Clicking opens the quick-view dialog (the parent owns the dialog; 2026-07-30
// the card-flight morph is retired — the dialog enters quietly, the card
// never moves). showProtocolTag defaults to true; a uniform-protocol group
// section collapses the per-card tag and shows one tag in the group header
// instead (GH #54) — flat mode and mixed-protocol groups keep the card tag.
const props = withDefaults(
  defineProps<{
    entry: OverviewEntry
    showProtocolTag?: boolean
  }>(),
  { showProtocolTag: true },
)
const emit = defineEmits<{ (e: 'open', entry: OverviewEntry): void }>()

// Middle truncation (GH #54): the head takes the CSS ellipsis while the
// tail (usually the version/variant part) stays fully visible. Split is
// width-driven — no JS character budgeting.
const modelSplit = computed(() => splitMiddle(props.entry.model_id))

// Stability score badge: colored by score band, gray when there is no data.
// The "评分 " prefix labels the bare number (GH #54); this is the stability
// score, not an eval score — formatScore does not govern it.
const scoreText = computed(() => (props.entry.score === null ? '暂无评分' : `评分 ${props.entry.score}`))
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
</script>

<style scoped>
.endpoint-card {
  /* Status indicator: a 3px vertical bar on the leading edge (eyes scan
     left to right, status comes first). Color mapping unchanged (§3). */
  border-left: 3px solid transparent;
  cursor: pointer;
  transition: box-shadow var(--hs-transition);
}
.endpoint-card:hover {
  box-shadow: var(--hs-shadow-md);
}
/* Keyboard focus = the board's single focus language (1px brand inset ring);
   box-shadow coexists with the 3px status border-left, no layout shift. */
.endpoint-card:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--hs-brand), var(--hs-shadow-md);
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
  /* Middle truncation (GH #54): the parent is a shrinkable flex row; the
     head span eats the ellipsis, the tail span never truncates. */
  display: flex;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.model-head {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.model-tail {
  flex: none;
  white-space: nowrap;
}
.card-head .el-tag {
  flex: none;
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
/* Metric hierarchy (GH #54): P50 is the primary latency signal, P95 is the
   secondary tail — they must not carry equal visual weight. The 24h success
   rate keeps the base md size. Order unchanged: rate / P50 / P95. */
.metric-p50 {
  font-size: var(--hs-text-lg);
  font-weight: 600;
}
.metric-p95 {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.card-foot {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
</style>
