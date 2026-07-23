<template>
  <div>
    <!-- Detail: two-line abnormal rows (status + name + 24h rate, then the
         reason) / all-healthy line / empty wording. -->
    <template v-if="emptyText">
      <div class="detail-empty">{{ emptyText }}</div>
    </template>
    <template v-else-if="abnormalEntries.length > 0">
      <div class="detail-title">异常明细</div>
      <div v-for="entry in topAbnormal" :key="entry.endpoint_id" class="detail-item">
        <div class="detail-row">
          <span class="row-status" :class="`st-${entry.status}`">{{ STATUS_LABELS[entry.status] }}</span>
          <span class="row-name" :title="`${entry.model_id} · ${entry.protocol}`">
            {{ entry.model_id }} · {{ entry.protocol }}
          </span>
          <span class="row-rate" :class="`av-${availabilityTier(entry.success_rate_24h)}`">
            {{ formatPercent(entry.success_rate_24h) }}
          </span>
        </div>
        <div v-if="entry.status_reason" class="row-reason">{{ entry.status_reason }}</div>
        <!-- Per-endpoint 24h dots: shows WHEN it degraded, not just that it
             did — the single rate number above can't separate "blew up an
             hour ago" from "half-dead all day". Compact (8px) and axis-less
             so ten rows stay readable. -->
        <div class="row-dots">
          <span v-for="(dot, i) in entry.dots_24h" :key="i" class="dot-slot">
            <span class="dot" :class="`seg-${dotTier(dot.total, dot.failures)}`" />
          </span>
        </div>
      </div>
      <div v-if="overflowCount > 0" class="detail-more">
        另有 {{ overflowCount }} 个异常端点未列出,详见状态板
      </div>
      <div v-if="healthyCount > 0" class="healthy-line">
        其余 <span class="healthy-num">{{ healthyCount }}</span> 个端点<span class="healthy-word">正常</span>{{ rangeText }}
      </div>
    </template>
    <div v-else class="healthy-line detail-healthy">
      全部 <span class="healthy-num">{{ entries.length }}</span> 个端点<span class="healthy-word">正常</span>{{ rangeText }}
    </div>

    <!-- One-sentence summary: visually subordinate to the conclusion (two
         steps smaller, no color), always ends in an action verb, and never
         papers over an abnormal state. -->
    <template v-if="summary">
      <div class="divider" />
      <div class="summary-row">
        <span class="summary-label">小结</span>
        <span class="summary-text">{{ summary }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
// StatusCard detail blocks (ticket 59): the capped abnormal list, the
// healthy-side summary line, and the one-sentence 小结. Computes everything
// from the scoped enabled-entry set via the statusCardSummary pure
// functions; the parent only supplies the empty-state wording (it depends
// on the disabled-inclusive count, which lives upstream).
import { computed } from 'vue'
import type { EndpointStatus, OverviewEntry } from '@/api/types'
import { formatPercent } from '@/utils/format'
import { STATUS_LABELS } from '@/utils/healthConclusion'
import { availabilityTier, dotTier, healthyRangeText } from '@/utils/statusCardSummary'

const props = defineProps<{
  entries: OverviewEntry[] // scoped ENABLED entries only
  emptyText: string // non-empty renders the empty state instead of the list
  summary: string | null
}>()

// Cap the abnormal list so a widespread outage cannot produce an unbounded
// tall image; the footer origin is the escape hatch to the live board.
const MAX_DETAIL_ROWS = 10

const SEVERITY_ORDER: Record<EndpointStatus, number> = { failing: 0, down: 1, degraded: 2, healthy: 3 }

const abnormalEntries = computed(() =>
  props.entries
    .filter(e => e.status !== 'healthy')
    .sort(
      (a, b) =>
        SEVERITY_ORDER[a.status] - SEVERITY_ORDER[b.status] || a.model_id.localeCompare(b.model_id),
    ),
)
const topAbnormal = computed(() => abnormalEntries.value.slice(0, MAX_DETAIL_ROWS))
const overflowCount = computed(() => abnormalEntries.value.length - topAbnormal.value.length)
const healthyCount = computed(() => props.entries.filter(e => e.status === 'healthy').length)
const rangeText = computed(() => healthyRangeText(props.entries))
</script>

<style scoped>
.divider {
  border-top: 1px solid var(--hs-border);
}
.detail-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-secondary);
  margin: 24px 0 8px;
}
.detail-item {
  padding: 8px 0;
}
.detail-item + .detail-item {
  border-top: 1px solid var(--hs-border);
}
.detail-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 24px;
}
.row-status {
  flex: none;
  width: 28px;
  font-size: var(--hs-text-sm);
  font-weight: 600;
}
.st-healthy {
  color: var(--el-color-success);
}
.st-degraded {
  color: var(--el-color-warning);
}
.st-down {
  color: var(--el-color-danger);
}
.st-failing {
  color: var(--hs-status-failing);
}
.row-name {
  flex: 1;
  /* flex items default to min-width: auto; without this the ellipsis never
     kicks in and long names get hard-clipped by the card's overflow. */
  min-width: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row-rate {
  flex: none;
  font-size: var(--hs-text-sm);
  font-weight: 600;
}
.av-ok {
  color: var(--el-color-success);
}
.av-partial {
  color: var(--el-color-warning);
}
.av-fail {
  color: var(--el-color-danger);
}
.av-none {
  color: var(--hs-text-placeholder);
}
.row-reason {
  /* Aligns with the name column (28px status + 12px gap); clamped to two
     lines — a static export has no hover to reveal the rest. */
  margin: 2px 0 0 40px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.row-dots {
  /* Same left indent as the reason so the timeline aligns under the name. */
  display: flex;
  gap: 2px;
  margin: 4px 0 0 40px;
}
.dot-slot {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.dot {
  width: 100%;
  height: 8px;
  border-radius: var(--hs-radius-xs);
}
.seg-ok {
  background: var(--el-color-success);
}
.seg-partial {
  background: var(--el-color-warning);
}
.seg-fail {
  background: var(--el-color-danger);
}
.seg-none {
  background: var(--hs-border);
}
.detail-more {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-top: 8px;
}
.healthy-line {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-top: 12px;
}
.detail-healthy {
  font-size: var(--hs-text-md);
  margin-top: 24px;
}
.healthy-num {
  color: var(--hs-text-primary);
  font-weight: 600;
}
.healthy-word {
  color: var(--el-color-success);
  font-weight: 600;
}
.detail-empty {
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
  margin-top: 24px;
}
.summary-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-top: 12px;
}
.summary-label {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.summary-text {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
</style>
