<template>
  <!-- Model status list (GH #115, spec 0018 §8; GH #131 reference-design
       enhancements; GH #136 seven fixes): the advanced list that replaced
       the EndpointCard matrix. The model NAME is the first hierarchy
       (md/600 ink, middle-truncated, hover shows the full name); every
       metric is auxiliary. GH #136: the name/availability/p95 column
       headers are CLICKABLE sort buttons (the parent owns the sort state,
       persisted to localStorage) — the active column carries an ↑/↓
       indicator; the vendor column renders the uniform brand TILE instead
       of the family text (title carries the full name); the availability
       cell is number-left + bar-right inline; the action cell is a bare
       chevron. The default ordering is availability DESC — the GH #136
       user ruling that overturned GH #131's「weakest first」default. -->
  <div class="model-list">
    <section v-for="section in sections" :key="section.key ?? '__flat__'" class="list-section">
      <header v-if="section.key !== null" class="section-header">
        <span class="section-key">{{ section.label }}</span>
        <span class="section-meta">{{ section.meta }}</span>
      </header>

      <div class="list-head list-grid">
        <button
          type="button"
          class="col-sort"
          :class="{ 'is-active': sort.key === 'name' }"
          :aria-label="`按模型名称排序${sort.key === 'name' ? (sort.dir === 'desc' ? ',当前降序' : ',当前升序') : ''}`"
          @click="emit('sort', 'name')"
        >
          模型<span v-if="sort.key === 'name'" class="sort-arrow" aria-hidden="true">{{ sort.dir === 'desc' ? '↓' : '↑' }}</span>
        </button>
        <span class="col-vendor">供应商</span>
        <span>状态</span>
        <button
          type="button"
          class="col-sort"
          :class="{ 'is-active': sort.key === 'rate' }"
          :aria-label="`按 24h 可用率排序${sort.key === 'rate' ? (sort.dir === 'desc' ? ',当前降序' : ',当前升序') : ''}`"
          @click="emit('sort', 'rate')"
        >
          24h 可用率<span v-if="sort.key === 'rate'" class="sort-arrow" aria-hidden="true">{{ sort.dir === 'desc' ? '↓' : '↑' }}</span>
        </button>
        <button
          type="button"
          class="col-sort"
          :class="{ 'is-active': sort.key === 'p95' }"
          :aria-label="`按 P95 延迟排序${sort.key === 'p95' ? (sort.dir === 'desc' ? ',当前降序' : ',当前升序') : ''}`"
          @click="emit('sort', 'p95')"
        >
          P95 延迟<span v-if="sort.key === 'p95'" class="sort-arrow" aria-hidden="true">{{ sort.dir === 'desc' ? '↓' : '↑' }}</span>
        </button>
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
        <span class="cell-vendor">
          <!-- Uniform vendor tile (GH #136): every known vendor renders the
               same 26x26 silhouette — solid brand ground + white glyph
               (vendorIcon.ts single source, multi-path marks like kimi
               carry per-path fills); unknown vendors fall back to the
               neutral initials tile. The family TEXT retired from this
               column — the tile's title carries the full name. -->
          <span
            class="vendor-tile"
            :class="{ 'has-icon': vendorIcon(entry.family) }"
            :style="vendorIcon(entry.family) ? { background: vendorIcon(entry.family)!.tile } : undefined"
            :title="entry.family"
          >
            <svg
              v-if="vendorIcon(entry.family)"
              viewBox="0 0 24 24"
              role="img"
              :aria-label="entry.family"
            >
              <path
                v-for="(p, i) in vendorIcon(entry.family)!.paths"
                :key="i"
                :d="p.d"
                :fill="p.fill"
              />
            </svg>
            <template v-else>{{ familyInitials(entry.family) }}</template>
          </span>
        </span>
        <span class="cell-status">
          <StatusBadge :status="entry.status" :causes="entry.degrade_causes" :reason="entry.status_reason" />
        </span>
        <span class="cell-rate" :class="`tier-${availabilityRateTier(entry.success_rate_24h)}`">
          <!-- Number LEFT + bar RIGHT (GH #136): the tier-colored number
               leads, the constant-scale (0–100) bar fills the remaining
               column width on the same line. Track bg-hover, fill the
               tier's GRAPHIC-grade token (text keeps the *-text grade —
               the graphic/text division). No data = empty gray track
               +「-」. No animation (GH #131: bars stay still). -->
          <span class="rate-value">{{ formatPercent(entry.success_rate_24h) }}</span>
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
          <!-- Chevron affordance (GH #136): the「详情」text button retired —
               a bare chevron-right carries the drill-down affordance; the
               accessible name stays explicit. -->
          <button
            type="button"
            class="detail-chevron"
            aria-label="查看详情"
            @click.stop="emit('open', entry)"
          >
            <svg
              viewBox="0 0 24 24"
              width="18"
              height="18"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="m9 18 6-6-6-6" />
            </svg>
          </button>
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
  type ListSort,
  type ListSortKey,
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

// The parent owns the sort state (persistence + the toolbar note); this
// component only renders the indicator and reports header clicks.
withDefaults(defineProps<{ sections: ListSection[]; sort: ListSort }>(), {})
const emit = defineEmits<{ open: [entry: OverviewEntry]; sort: [key: ListSortKey] }>()
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
   construction property of the list, never per-row luck. GH #136 cascade
   (content-box, 1200px content lane, row padding 12px×2 → grid lane
   1176px): fixed = vendor 44 + status 150 + rate 150 + p95 100 + action
   40 = 484px; gaps = 6 × 12px (space-3) = 72px; flex remainder 1176 − 484
   − 72 = 620px splits name 1.8fr ≈ 399 / trend 1fr ≈ 221 (floor 150).
   Vendor shrank 120 → 44 (tile-only column), rate grew 120 → 150 (number
   + inline bar), action shrank 56 → 40 (bare chevron). */
.list-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.8fr) 44px 150px 150px 100px minmax(150px, 1fr) 40px;
  align-items: center;
  gap: var(--hs-space-3);
}
.list-head {
  padding: 0 var(--hs-space-3) var(--hs-space-2);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  border-bottom: 1px solid var(--hs-border);
}
/* Sortable column header (GH #136): a button reset onto the header text
   style — the head spans and the buttons share one typographic lane. The
   active column shows the direction arrow (brand); hover lifts the ink.
   Vendor / status / trend / action stay plain spans (no total order). */
.col-sort {
  display: inline-flex;
  align-items: center;
  gap: var(--hs-space-1);
  justify-self: start;
  background: none;
  border: none;
  padding: 0;
  font: inherit;
  color: inherit;
  cursor: pointer;
}
.col-sort:hover {
  color: var(--hs-text-primary);
}
.sort-arrow {
  color: var(--hs-brand);
  font-weight: 600;
}
.col-vendor {
  overflow: hidden;
  white-space: nowrap;
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
.cell-vendor {
  min-width: 0;
}
/* Uniform vendor tile (GH #136): one fixed 26x26 square for EVERY vendor
   — control-grade radius, neutral hover-surface ground + secondary
   initials for unknown vendors. A 3-char CJK family name (~36px) would
   otherwise spill past the fixed box (GH #131 check LOW-1). */
.vendor-tile {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  overflow: hidden;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
}
/* Known vendor: the ground is the inline brand tile color (vendorIcon.ts);
   the 16px white glyph centers inside — the GH #134 transparent-ground
   form retired with the uniform tile. */
.vendor-tile.has-icon {
  color: var(--hs-bg-card);
}
.vendor-tile.has-icon svg {
  display: block;
  width: 16px;
  height: 16px;
}
.cell-rate {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
.rate-value {
  flex: none;
  font-size: var(--hs-text-md);
  font-variant-numeric: tabular-nums;
}
/* Inline tier bar (GH #131; GH #136 number-left + bar-right): 0–100
   constant scale, 4px fill over a bg-hover track; the segmented-strip
   radius (radius-xs) marks it as a time/scale bar element. The bar takes
   the column width left over by the number (24px floor keeps it a bar,
   not a dot). The fill never transitions (GH #131: no bar animation). */
.rate-bar {
  flex: 1 1 0;
  min-width: 24px;
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
/* Chevron button (GH #136): secondary ink, brand on hover — a quiet
   affordance, not a link. */
.detail-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  padding: var(--hs-space-1);
  border-radius: var(--hs-radius-sm);
  color: var(--hs-text-secondary);
  cursor: pointer;
}
.detail-chevron:hover {
  color: var(--hs-brand);
}
</style>
