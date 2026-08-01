<template>
  <!-- Model status list (GH #115, spec 0018 §8; GH #131 reference-design
       enhancements; GH #136 seven fixes; GH #139 gray-ground/white-tile
       rework; GH #140 hover fix + protocol tag + grouping regression): the
       advanced list that replaced the EndpointCard matrix. The model NAME is
       the first hierarchy (md/600 ink, middle-truncated, hover shows the
       full name); every metric is auxiliary. GH #136: the name/availability/
       p95 column headers are CLICKABLE sort buttons (the parent owns the
       sort state, persisted to localStorage) — the active column carries an
       ↑/↓ indicator; the availability cell is number-left + bar-right
       inline; the action cell is a bare chevron. GH #139: the vendor TILE
       moved back into the name cell (left of the model id) and the vendor
       column renders the family TEXT again — superseding GH #136's
       tile-column layout; the sections themselves are white tiles on the
       gray skeleton ground. The default ordering is availability DESC —
       the GH #136 user ruling that overturned GH #131's「weakest first」
       default. GH #140: the name cell carries a PROTOCOL MINI-TAG after
       the model id (same model_id on several protocols reads apart); the
       grouping selector returns (flat / by vendor / by capability / by
       protocol) and the section contract activates with real group
       headers. -->
  <div class="model-list">
    <section v-for="section in sections" :key="section.key ?? '__flat__'" class="list-section">
      <header v-if="section.key !== null" class="section-header">
        <!-- Group-header vendor tile (GH #140): only family-grouped sections
             carry tileFamily — known vendors render the same uniform 26x26
             tile as the rows (one silhouette everywhere), unknown vendors
             the neutral initials tile. Capability/protocol groups render
             the bare group name. -->
        <span
          v-if="section.tileFamily"
          class="vendor-tile"
          :class="{ 'has-icon': vendorIcon(section.tileFamily) }"
          :style="
            vendorIcon(section.tileFamily)
              ? { background: vendorTileBackground(vendorIcon(section.tileFamily)!) }
              : undefined
          "
          :title="section.tileFamily"
        >
          <svg
            v-if="vendorIcon(section.tileFamily)"
            viewBox="0 0 24 24"
            role="img"
            :aria-label="section.tileFamily"
          >
            <defs v-if="vendorIcon(section.tileFamily)!.gradients">
              <linearGradient
                v-for="g in vendorIcon(section.tileFamily)!.gradients"
                :key="g.id"
                :id="g.id"
                :x1="g.x1"
                :y1="g.y1"
                :x2="g.x2"
                :y2="g.y2"
              >
                <stop v-for="s in g.stops" :key="s.offset" :offset="s.offset" :stop-color="s.color" />
              </linearGradient>
            </defs>
            <circle
              v-for="(c, i) in vendorIcon(section.tileFamily)!.circles ?? []"
              :key="`c${i}`"
              :cx="c.cx"
              :cy="c.cy"
              :r="c.r"
              :fill="c.fill"
            />
            <path
              v-for="(p, i) in vendorIcon(section.tileFamily)!.paths"
              :key="i"
              :d="p.d"
              :fill="p.fill"
            />
          </svg>
          <template v-else>{{ familyInitials(section.tileFamily) }}</template>
        </span>
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
          <!-- Vendor tile (GH #139: moved back into the name cell, left of
               the model id — the reference design; GH #136's tile column
               is superseded and the vendor column returns to text). One
               uniform 26x26 silhouette per vendor (vendorIcon.ts single
               source, three variants GH #140: brand ground + white glyph /
               subtle ground + original-color mark / self-grounded disc);
               unknown vendors fall back to the neutral initials tile. The
               title carries the full family name. -->
          <span
            class="vendor-tile"
            :class="{ 'has-icon': vendorIcon(entry.family) }"
            :style="
              vendorIcon(entry.family)
                ? { background: vendorTileBackground(vendorIcon(entry.family)!) }
                : undefined
            "
            :title="entry.family"
          >
            <svg
              v-if="vendorIcon(entry.family)"
              viewBox="0 0 24 24"
              role="img"
              :aria-label="entry.family"
            >
              <defs v-if="vendorIcon(entry.family)!.gradients">
                <linearGradient
                  v-for="g in vendorIcon(entry.family)!.gradients"
                  :key="g.id"
                  :id="g.id"
                  :x1="g.x1"
                  :y1="g.y1"
                  :x2="g.x2"
                  :y2="g.y2"
                >
                  <stop v-for="s in g.stops" :key="s.offset" :offset="s.offset" :stop-color="s.color" />
                </linearGradient>
              </defs>
              <circle
                v-for="(c, i) in vendorIcon(entry.family)!.circles ?? []"
                :key="`c${i}`"
                :cx="c.cx"
                :cy="c.cy"
                :r="c.r"
                :fill="c.fill"
              />
              <path
                v-for="(p, i) in vendorIcon(entry.family)!.paths"
                :key="i"
                :d="p.d"
                :fill="p.fill"
              />
            </svg>
            <template v-else>{{ familyInitials(entry.family) }}</template>
          </span>
          <el-tooltip :content="entry.model_id" :show-after="200" placement="top">
            <span class="name-text">
              <span class="name-head">{{ splitMiddle(entry.model_id).head }}</span>
              <span class="name-tail">{{ splitMiddle(entry.model_id).tail }}</span>
            </span>
          </el-tooltip>
          <!-- Protocol mini-tag (GH #140): the same model_id on several
               protocols reads apart inline (endpoint = model × protocol,
               W3). A self-made light tag — NOT el-tag — consuming the
               protocol.ts single mapping for the color slot; the word is
               the protocol value itself (never translated, §5 protocol
               vocabulary). -->
          <span class="proto-tag" :class="`t-${protocolTagType(entry.protocol)}`">{{ entry.protocol }}</span>
          <span v-if="!entry.enabled" class="disabled-tag">已停用</span>
        </span>
        <!-- Vendor column as TEXT (GH #139, reference design: name cell =
             tile + model id, vendor column = family word): sm secondary,
             truncated with the full name on the title. -->
        <span class="cell-vendor" :title="entry.family">{{ entry.family }}</span>
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
import { vendorIcon, vendorTileBackground } from '@/utils/vendorIcon'
import { protocolTagType } from '@/utils/protocol'
import { splitMiddle } from '@/utils/truncate'
import type { OverviewEntry } from '@/api/types'

// One list section: key null = the flat list (no section header); grouped
// mode (GH #140) passes the group key plus a pre-composed meta line (counts
// wording is composed by the parent from the single display-layer mapping).
// tileFamily is set ONLY for family-grouped sections — the header renders
// the group vendor tile then; capability/protocol groups render the bare
// group name.
export interface ListSection {
  key: string | null
  label: string
  meta: string
  entries: OverviewEntry[]
  tileFamily?: string | null
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
  /* White region tile on the gray skeleton ground (GH #139): bg-card +
     1px border + radius-lg, same light-container syntax as the widgets.
     The 8px inner padding keeps the row hover FILL (GH #140: bg-hover,
     no lift) inside the tile and lets the last row drop its hairline
     without touching the container edge. */
  padding: var(--hs-space-2);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
}
.section-header {
  display: flex;
  align-items: center;
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
   construction property of the list, never per-row luck. GH #139 cascade
   (content-box, 1200px content lane; section tile = border 1px×2 +
   padding 8px×2 → inner lane 1182px; row padding 12px×2 → grid lane
   1158px): fixed = vendor 100 + status 150 + rate 150 + p95 100 + action
   40 = 540px; gaps = 6 × 12px (space-3) = 72px; flex remainder 1158 − 540
   − 72 = 546px splits name 1.8fr ≈ 351 / trend 1fr ≈ 195 (floor 150).
   GH #139: the vendor column grew 44 → 100 (family text returns; the
   26px tile + 8px gap now live inside the name cell, leaving ≈317px for
   the model id); the GH #136 vendor tile column (44px) is superseded.
   GH #140: the name cell also carries the protocol mini-tag (xs, padding
   4px×8px, ≈70px for chat protocols / ≈120px for images_generation at
   most) after the model id — the model-id text lane absorbs it (≈200–
   250px) and the middle truncation keeps the distinguishing tail visible;
   the grid template itself does not change. */
.list-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.8fr) 100px 150px 150px 100px minmax(150px, 1fr) 40px;
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
  transition: background-color var(--hs-transition);
}
/* Inside the white section tile (GH #139) the last row drops its hairline
   — the container edge already closes the list. */
.list-section .list-row:last-child {
  border-bottom: none;
}
/* Hover feedback (GH #140, user round-5 ruling): a plain bg-hover FILL —
   the GH #136/spec-0018 hover lift (translateY −2px + shadow-md +
   white-on-white fill) is RETIRED: on the white section tile the white
   fill was invisible and the floating shadow broke through the vendor
   tile (「穿帮」). The gray fill layers naturally inside the white tile
   and around the brand tiles; semantics.css zeroes the transition under
   reduced motion. This replaces the GH #139 check LOW-2 known item (the
   white-on-white fill), which is now resolved by this caliber. */
.list-row:hover {
  background: var(--hs-bg-hover);
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
/* Protocol mini-tag (GH #140): a self-made light tag — NOT el-tag (the
   name cell needs a quieter, smaller unit than the el-tag geometry). The
   color slot consumes the protocol.ts single mapping (protocolTagType):
   success/warning/info read as CONTRACT-FAMILY distinction colors, never
   as health signals (§5 protocol-tag semantics); the word is the raw
   protocol value. Soft ground + text-grade ink; flex:none so the tag
   survives and the model id truncates first. */
.proto-tag {
  flex: none;
  font-size: var(--hs-text-xs);
  line-height: 1;
  padding: var(--hs-space-1) var(--hs-space-2);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-info-soft);
  color: var(--hs-info);
}
.proto-tag.t-success {
  background: var(--hs-success-soft);
  color: var(--hs-success-text);
}
.proto-tag.t-warning {
  background: var(--hs-warning-soft);
  color: var(--hs-warning-text);
}
.proto-tag.t-info {
  background: var(--hs-info-soft);
  color: var(--hs-info);
}
.disabled-tag {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.cell-vendor {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
/* Uniform vendor tile (GH #136; GH #139 render seat moved into the name
   cell; GH #140 three variants): one fixed 26x26 square for EVERY vendor —
   control-grade radius, neutral hover-surface ground + secondary initials
   for unknown vendors. The ground for KNOWN vendors is the inline
   vendorTileBackground: brand hex (brand variant) / --hs-bg-subtle
   (subtle variant, original-color marks) / transparent (none variant,
   hunyuan's self-grounded disc). A 3-char CJK family name (~36px) would
   otherwise spill past the fixed box (GH #131 check LOW-1). */
.vendor-tile {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  flex: none;
  overflow: hidden;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
}
/* Known vendor: the ground is the inline variant background
   (vendorTileBackground, vendorIcon.ts); the 16px glyph centers inside —
   the GH #134 transparent-ground form retired with the uniform tile. */
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
