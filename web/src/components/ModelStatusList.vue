<template>
  <!-- Model status list (GH #115, spec 0018 §8; GH #131 reference-design
       enhancements): the advanced list that replaces the EndpointCard
       matrix. The model NAME is the first hierarchy (vendor chip + md/600
       ink, middle-truncated, hover shows the full name); every metric is
       auxiliary. Rows arrive availability-ranked from the parent
       (sortEntriesByAvailability — the「(按可用率排序)」headline note must
       be literally true), so the weakest models lead the first viewport.
       GH #131 cells: availability = number + inline constant-scale tier bar;
       trend = per-row latency sparkline tinted by display state (the 24h
       micro dot strip retired from this column — its time shape survives in
       the detail panel and the share material). -->
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
          <!-- Vendor chip (GH #131 + reference replica part 2): the real
               vendor SVG mark when the family maps (vendorIcon.ts single
               source, brand colors are exempt external assets); unknown
               vendors fall back to the neutral initials chip. Mapped chips
               drop the soft ground — the mark stands alone per the
               reference. The full family name sits one column over. -->
          <span
            class="vendor-chip"
            :class="{ 'has-icon': vendorIcon(entry.family) }"
            :title="entry.family"
          >
            <svg
              v-if="vendorIcon(entry.family)"
              viewBox="0 0 24 24"
              role="img"
              :aria-label="entry.family"
            >
              <path :d="vendorIcon(entry.family)!.path" :fill="vendorIcon(entry.family)!.color" />
            </svg>
            <template v-else>{{ familyInitials(entry.family) }}</template>
          </span>
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
          <span class="rate-value">{{ formatPercent(entry.success_rate_24h) }}</span>
          <!-- Inline constant-scale (0–100) tier bar: track bg-hover, fill
               the tier's GRAPHIC-grade token (text keeps the *-text grade —
               the graphic/text division). No data = empty gray track +「-」.
               No animation (GH #131: bars and sparklines stay still). -->
          <span class="rate-bar" aria-hidden="true">
            <span class="rate-fill" :style="{ width: `${availabilityBarWidth(entry.success_rate_24h)}%` }" />
          </span>
        </span>
        <span class="cell-p95">{{ formatMs(entry.p95_ms) }}</span>
        <span class="cell-trend">
          <TrendSparkline
            :values="entryLatencySeries(entry.dots_24h)"
            :tone="rowSparklineTone(entry)"
          />
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
import TrendSparkline from '@/components/TrendSparkline.vue'
import { formatMs, formatPercent } from '@/utils/format'
import { availabilityRateTier } from '@/utils/overviewMetrics'
import {
  availabilityBarWidth,
  entryLatencySeries,
  familyInitials,
  rowSparklineTone,
} from '@/utils/modelList'
import { vendorIcon } from '@/utils/vendorIcon'
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
   flexes first; the trend sparkline keeps a readable floor. The rate
   column (120px) carries the number + inline bar stack (GH #131). */
.list-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.8fr) 120px 150px 120px 100px minmax(150px, 1fr) 56px;
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
/* Vendor chip (GH #131): a fixed neutral square — the control-grade radius,
   hover surface ground, secondary ink. */
.vendor-chip {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  /* GH #131 check LOW-1: a 3-char CJK family name (~36px) would otherwise
     spill past the fixed box. */
  overflow: hidden;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
}
/* Mapped vendor (reference replica part 2): the soft ground drops away and
   the 16px brand mark stands alone. The primary-ink color only reaches
   currentColor monochrome marks (openai) — explicit brand fills ignore it;
   text-primary keeps the near-black mark visible on both themes. */
.vendor-chip.has-icon {
  background: transparent;
  color: var(--hs-text-primary);
}
.vendor-chip.has-icon svg {
  display: block;
  width: 16px;
  height: 16px;
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
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
  min-width: 0;
}
.rate-value {
  font-size: var(--hs-text-md);
  font-variant-numeric: tabular-nums;
}
/* Inline tier bar (GH #131): 0–100 constant scale, 4px fill over a
   bg-hover track; the segmented-strip radius (radius-xs) marks it as a
   time/scale bar element. The fill never transitions (GH #131: no bar
   animation). */
.rate-bar {
  display: block;
  height: 4px;
  border-radius: var(--hs-radius-xs);
  background: var(--hs-bg-hover);
  overflow: hidden;
}
.rate-fill {
  display: block;
  height: 100%;
  border-radius: var(--hs-radius-xs);
}
/* Rate words consume the *-text grade; the bar fill consumes the tier's
   graphic-grade body token (graphic/text division, GH #131). */
.cell-rate.tier-success .rate-value {
  color: var(--hs-success-text);
}
.cell-rate.tier-success .rate-fill {
  background: var(--hs-success);
}
.cell-rate.tier-warning .rate-value {
  color: var(--hs-warning-text);
}
.cell-rate.tier-warning .rate-fill {
  background: var(--hs-warning);
}
.cell-rate.tier-danger .rate-value {
  color: var(--hs-danger-text);
}
.cell-rate.tier-danger .rate-fill {
  background: var(--hs-danger);
}
.cell-rate.tier-none .rate-value {
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
/* Row density: the sparkline's default 32px widget height compresses to a
   20px row lane (the child root takes the parent's scope, no :deep). */
.cell-trend .trend-sparkline {
  height: 20px;
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
