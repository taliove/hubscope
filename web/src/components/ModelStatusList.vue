<template>
  <!-- Model status list (GH #115, spec 0018 §8): the advanced list that
       replaces the EndpointCard matrix. The model NAME is the first
       hierarchy (md/600 ink, middle-truncated, hover shows the full name);
       every metric is auxiliary. Rows arrive severity-ranked from the
       parent (sortEntriesBySeverity — the board's single rank table), so
       abnormal models lead the first viewport. Grouping renders as light
       list sections (the collapse machinery of the old group headers is
       retired with the section component). -->
  <div class="model-list">
    <section v-for="section in sections" :key="section.key ?? '__flat__'" class="list-section">
      <header v-if="section.key !== null" class="section-header">
        <span class="section-key">{{ section.label }}</span>
        <span class="section-meta">{{ section.meta }}</span>
      </header>

      <div class="list-head list-grid">
        <span>模型</span>
        <span>供应商</span>
        <span>状态</span>
        <span>24h 可用率</span>
        <span>P95 延迟</span>
        <span>24h 趋势</span>
        <span class="col-action">操作</span>
      </div>

      <div
        v-for="entry in section.entries"
        :key="entry.endpoint_id"
        class="list-row list-grid"
        :class="{ 'is-disabled': !entry.enabled }"
        role="button"
        tabindex="0"
        :data-endpoint-id="entry.endpoint_id"
        @click="emit('open', entry)"
        @keydown.enter.prevent="emit('open', entry)"
        @keydown.space.prevent="emit('open', entry)"
      >
        <span class="cell-name">
          <el-tooltip :content="entry.model_id" :show-after="200" placement="top">
            <span class="name-text">
              <span class="name-head">{{ splitMiddle(entry.model_id).head }}</span>
              <span class="name-tail">{{ splitMiddle(entry.model_id).tail }}</span>
            </span>
          </el-tooltip>
          <span v-if="!entry.enabled" class="disabled-tag">已停用</span>
        </span>
        <span class="cell-family" :title="entry.family">{{ entry.family }}</span>
        <span class="cell-status">
          <StatusBadge :status="entry.status" :causes="entry.degrade_causes" :reason="entry.status_reason" />
        </span>
        <span class="cell-rate" :class="`tier-${availabilityRateTier(entry.success_rate_24h)}`">
          {{ formatPercent(entry.success_rate_24h) }}
        </span>
        <span class="cell-p95">{{ formatMs(entry.p95_ms) }}</span>
        <span class="cell-trend">
          <UptimeMicroStrip :dots="entry.dots_24h" />
        </span>
        <span class="cell-action">
          <button type="button" class="detail-link" @click.stop="emit('open', entry)">详情</button>
        </span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import StatusBadge from '@/components/StatusBadge.vue'
import UptimeMicroStrip from '@/components/UptimeMicroStrip.vue'
import { formatMs, formatPercent } from '@/utils/format'
import { availabilityRateTier } from '@/utils/overviewMetrics'
import { splitMiddle } from '@/utils/truncate'
import type { OverviewEntry } from '@/api/types'

// One list section: key null = the flat list (no section header); grouped
// mode passes the group key plus a pre-composed meta line (counts wording
// is composed by the parent from the single display-layer mapping).
export interface ListSection {
  key: string | null
  label: string
  meta: string
  entries: OverviewEntry[]
}

withDefaults(defineProps<{ sections: ListSection[] }>(), {})
const emit = defineEmits<{ open: [entry: OverviewEntry] }>()
</script>

<style scoped>
.model-list {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-5);
}
.list-section {
  display: flex;
  flex-direction: column;
}
.section-header {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-3);
  padding: 0 var(--hs-space-2) var(--hs-space-2);
}
.section-key {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.section-meta {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
/* Column template shared by the head and every row — alignment is a
   construction property of the list, never per-row luck. The name column
   flexes first; the trend strip keeps a readable floor. */
.list-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.8fr) 120px 150px 100px 100px minmax(150px, 1fr) 56px;
  align-items: center;
  gap: var(--hs-space-3);
}
.list-head {
  padding: 0 var(--hs-space-3) var(--hs-space-2);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  border-bottom: 1px solid var(--hs-border);
}
.col-action {
  text-align: right;
}
.list-row {
  padding: var(--hs-space-3);
  border-bottom: 1px solid var(--hs-border-light);
  border-radius: var(--hs-radius-lg);
  cursor: pointer;
  transition:
    transform var(--hs-transition),
    box-shadow var(--hs-transition),
    background-color var(--hs-transition);
}
/* Hover lift (spec 0018 §15: 2–4px + light shadow); semantics.css zeroes
   the transition under reduced motion. */
.list-row:hover {
  transform: translateY(-2px);
  box-shadow: var(--hs-shadow-md);
  background: var(--hs-bg-card);
}
.list-row:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}
.cell-name {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
.name-text {
  display: flex;
  min-width: 0;
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
/* Middle truncation (truncate.ts): the head ellipsizes, the fixed tail
   keeps the distinguishing suffix visible. */
.name-head {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.name-tail {
  flex: none;
  white-space: pre;
}
.is-disabled .name-text {
  color: var(--hs-text-secondary);
  font-weight: 400;
}
.disabled-tag {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.cell-family {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cell-rate {
  font-size: var(--hs-text-md);
  font-variant-numeric: tabular-nums;
}
/* Rate words consume the *-text grade (graphic/text division). */
.cell-rate.tier-success {
  color: var(--hs-success-text);
}
.cell-rate.tier-warning {
  color: var(--hs-warning-text);
}
.cell-rate.tier-danger {
  color: var(--hs-danger-text);
}
.cell-rate.tier-none {
  color: var(--hs-text-placeholder);
}
.cell-p95 {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  font-variant-numeric: tabular-nums;
}
.cell-trend {
  min-width: 0;
}
.cell-action {
  text-align: right;
}
.detail-link {
  background: none;
  border: none;
  padding: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  cursor: pointer;
}
.detail-link:hover {
  color: var(--hs-brand-hover);
}
</style>
