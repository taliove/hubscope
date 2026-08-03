<template>
  <!-- Model status list (GH #115, spec 0018 §8; GH #131 reference-design
       enhancements; GH #136 seven fixes; GH #139 gray-ground/white-tile
       rework; GH #140 hover fix + protocol tag + grouping regression): the
       advanced list that replaced the EndpointCard matrix. The model NAME is
       the first hierarchy (md/600 ink, middle-truncated, hover shows the
       full name); every metric is auxiliary. GH #136: the name/availability/
       p95 column headers are CLICKABLE sort buttons (the parent owns the
       sort state, persisted to localStorage) — the active column carries an
       ↑/↓ indicator; the availability cell is number-left + the 24-cell
       UptimeMicroStrip right (2026-08-02, the continuous bar retired);
       the action cell is a bare chevron. GH #139: the vendor TILE
       moved back into the name cell (left of the model id) and the vendor
       column renders the family TEXT again — superseding GH #136's
       tile-column layout; the sections themselves are white tiles on the
       gray skeleton ground. The default ordering is availability DESC —
       the GH #136 user ruling that overturned GH #131's「weakest first」
       default. GH #140: the grouping selector returns (flat / by vendor /
       by capability / by protocol) and the section contract activates with
       real group headers. 2026-08-02 (user ruling): the vendor TEXT column
       becomes the PROTOCOL column (plain text, widened 100 → 130 so
       images_generation renders in full) and the GH #140 name-cell
       protocol mini-tag retires with it; the model id is click-to-copy
       (tooltip keeps the full id on hover, ElMessage confirms). -->
  <!-- Narrow card form (2026-08-01 shell drawer batch, useBreakpoint):
       below the 1024px breakpoint the 7-column grid cannot compress, so the
       list switches its FORM (the registered narrow-screen principle —
       form switch, never horizontal-scroll exemption) to stacked cards:
       top line = vendor tile + model id (click-to-copy) + chevron, mid line
       = StatusBadge + protocol word + 已停用 note, stats line = availability
       number+bar and P95. The 24h trend sparkline is the one omission
       (registered in the dashboard surface brief); the column header row
       is not rendered (sorting stays a desktop interaction — the persisted
       sort state still orders the cards). Cards keep the round-7 pure-
       rectangle language: straight hairlines, hover fill, no radius. -->
  <div v-if="isNarrow" class="model-list">
    <section v-for="section in sections" :key="section.key ?? '__flat__'" class="list-section">
      <header v-if="section.key !== null" class="section-header">
        <VendorTile v-if="section.tileFamily" :family="section.tileFamily" />
        <span class="section-key">{{ section.label }}</span>
        <span class="section-meta">{{ section.meta }}</span>
        <!-- Group signal strip + share (2026-08-02 user ruling): the header
             carries the group's probe-weighted 24h aggregate strip (same
             UptimeMicroStrip as the rows) and a share entry that opens the
             StatusCard dialog scoped to this group (parent composes the
             snapshot — the ticket-59 group field leads the scope chips). -->
        <UptimeMicroStrip v-if="section.dots" class="section-strip" :dots="section.dots" />
        <button
          type="button"
          class="section-share"
          aria-label="分享该组状态"
          @click="emit('share-group', section)"
        >
          <el-icon><Share /></el-icon>
        </button>
      </header>

      <div
        v-for="entry in section.entries"
        :key="entry.endpoint_id"
        class="m-card"
        :class="{ 'is-disabled': !entry.enabled }"
        role="button"
        tabindex="0"
        :data-endpoint-id="entry.endpoint_id"
        @click="emit('open', entry)"
        @keydown.enter.prevent="emit('open', entry)"
        @keydown.space.prevent="emit('open', entry)"
      >
        <div class="m-card-top">
          <VendorTile :family="entry.family" />
          <el-tooltip :content="entry.model_id" :show-after="200" placement="top">
            <button
              type="button"
              class="name-text name-copy"
              @click.stop="copyModelId(entry)"
              @keydown.enter.stop
              @keydown.space.stop
            >
              <span class="name-head">{{ splitMiddle(entry.model_id).head }}</span>
              <span class="name-tail">{{ splitMiddle(entry.model_id).tail }}</span>
            </button>
          </el-tooltip>
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
        </div>
        <div class="m-card-mid">
          <StatusBadge :status="entry.status" :reason="entry.status_reason" />
          <span class="m-proto" :title="entry.protocol">{{ entry.protocol }}</span>
          <span v-if="!entry.enabled" class="disabled-tag">已停用</span>
        </div>
        <div class="m-card-stats">
          <span class="cell-rate" :class="`tier-${availabilityRateTier(entry.success_rate_24h)}`">
            <span class="rate-value">{{ formatPercent(entry.success_rate_24h) }}</span>
            <UptimeMicroStrip :dots="entry.dots_24h" />
          </span>
          <span class="m-p95">P95&nbsp;<span class="cell-p95">{{ formatMs(entry.p95_ms) }}</span></span>
        </div>
      </div>
    </section>
  </div>

  <!-- Desktop grid form. -->
  <div v-else class="model-list">
    <section v-for="section in sections" :key="section.key ?? '__flat__'" class="list-section">
      <header v-if="section.key !== null" class="section-header">
        <!-- Group-header vendor tile (GH #140): only family-grouped sections
             carry tileFamily — known vendors render the same uniform 26x26
             tile as the rows (one silhouette everywhere), unknown vendors
             the neutral initials tile. Capability/protocol groups render
             the bare group name. -->
        <VendorTile v-if="section.tileFamily" :family="section.tileFamily" />
        <span class="section-key">{{ section.label }}</span>
        <span class="section-meta">{{ section.meta }}</span>
        <UptimeMicroStrip v-if="section.dots" class="section-strip" :dots="section.dots" />
        <button
          type="button"
          class="section-share"
          aria-label="分享该组状态"
          @click="emit('share-group', section)"
        >
          <el-icon><Share /></el-icon>
        </button>
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
        <span class="col-proto">协议</span>
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
               is superseded). One uniform 26x26 silhouette per vendor
               (vendorIcon.ts single source, three variants GH #140: brand
               ground + white glyph / subtle ground + original-color mark /
               self-grounded disc); unknown vendors fall back to the neutral
               initials tile. The title carries the full family name — the
               tile is now the ONLY vendor seat in the row (2026-08-02: the
               vendor text column became the protocol column). -->
          <VendorTile :family="entry.family" />
          <!-- Model id: tooltip shows the full id on hover; a CLICK copies
               it (2026-08-02 user ruling) — copyText carries the non-secure-
               context fallback (the test line is plain http). The button is
               a nested interactive element inside the row's role=button, so
               click AND keydown stop here: Enter/Space must copy, never
               trigger the row's open-panel handler. -->
          <el-tooltip :content="entry.model_id" :show-after="200" placement="top">
            <button
              type="button"
              class="name-text name-copy"
              @click.stop="copyModelId(entry)"
              @keydown.enter.stop
              @keydown.space.stop
            >
              <span class="name-head">{{ splitMiddle(entry.model_id).head }}</span>
              <span class="name-tail">{{ splitMiddle(entry.model_id).tail }}</span>
            </button>
          </el-tooltip>
          <span v-if="!entry.enabled" class="disabled-tag">已停用</span>
        </span>
        <!-- Protocol column (2026-08-02 user ruling: replaces the vendor
             text column — endpoint = model × protocol (W3), and the protocol
             distinguishes same-id rows better than the family word, which
             the name-cell tile already carries). Plain text in the column
             language (sm secondary, truncation + title), NOT the GH #140
             colored mini-tag — the tag form retired with this change. -->
        <span class="cell-proto" :title="entry.protocol">{{ entry.protocol }}</span>
        <span class="cell-status">
          <!-- No causes in the list (2026-08-02 user ruling): the「· 可用
               性 / · 延迟」suffixes retire from the ROW so the status column
               slims to the word itself (150 → 100); the cause detail lives
               one click away in the detail panel (badge there keeps them). -->
          <StatusBadge :status="entry.status" :reason="entry.status_reason" />
        </span>
        <span class="cell-rate" :class="`tier-${availabilityRateTier(entry.success_rate_24h)}`">
          <!-- Number LEFT + 24-cell signal strip RIGHT (2026-08-02 user
               ruling, reviving UptimeMicroStrip): the continuous 0–100 bar
               retires — the strip shows the past 24 hours cell by cell
               (one cell = one hour, tier-colored by the overviewDots single
               mapping, per-cell tooltip with the hour's exact success
               count). The number keeps the *-text grade and the at-a-glance
               reading. -->
          <span class="rate-value">{{ formatPercent(entry.success_rate_24h) }}</span>
          <UptimeMicroStrip :dots="entry.dots_24h" />
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
import UptimeMicroStrip from '@/components/UptimeMicroStrip.vue'
import VendorTile from '@/components/VendorTile.vue'
import { Share } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { formatMs, formatPercent } from '@/utils/format'
import { availabilityRateTier } from '@/utils/overviewMetrics'
import {
  entryLatencySeries,
  rowSparklineTone,
  type ListSort,
  type ListSortKey,
} from '@/utils/modelList'
import { splitMiddle } from '@/utils/truncate'
import { copyText } from '@/utils/clipboard'
import { useBreakpoint } from '@/composables/useBreakpoint'
import type { OverviewDot, OverviewEntry } from '@/api/types'

// One list section: key null = the flat list (no section header); grouped
// mode (GH #140) passes the group key plus a pre-composed meta line (counts
// wording is composed by the parent from the single display-layer mapping).
// tileFamily is set ONLY for family-grouped sections — the header renders
// the group vendor tile then; capability/protocol groups render the bare
// group name. dots (2026-08-02) is the group's probe-weighted 24h
// aggregate for the header signal strip (flat mode has no header and
// leaves it undefined).
export interface ListSection {
  key: string | null
  label: string
  meta: string
  entries: OverviewEntry[]
  tileFamily?: string | null
  dots?: OverviewDot[]
}

// The parent owns the sort state (persistence + the toolbar note); this
// component only renders the indicator and reports header clicks.
withDefaults(defineProps<{ sections: ListSection[]; sort: ListSort }>(), {})
const emit = defineEmits<{
  open: [entry: OverviewEntry]
  sort: [key: ListSortKey]
  'share-group': [section: ListSection]
}>()

// Narrow form switch (2026-08-01 shell drawer batch): the shared 1024px
// breakpoint drives the card/grid dual render.
const { isNarrow } = useBreakpoint()

// Click-to-copy on the model id (2026-08-02 user ruling): the result walks
// the registered feedback trio (ElMessage for operation results); copyText
// degrades to a hidden-textarea copy on non-secure contexts (plain-http
// test line), and a hard failure says so instead of staying silent.
async function copyModelId(entry: OverviewEntry) {
  const ok = await copyText(entry.model_id)
  if (ok) {
    ElMessage.success(`已复制模型 ID:${entry.model_id}`)
  } else {
    ElMessage.warning('复制失败,请手动选择复制')
  }
}
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
     without touching the container edge.
     overflow: hidden (2026-08-01 squeeze-band hardening): between the
     1024px breakpoint and ~1280px the shared grid's fixed minimum (fixed
     columns 540 + name floor 140 + gaps 72) can exceed the lane, and the
     overflow used to paint past the tile edge and over neighbouring cells;
     now the right edge clips progressively at the tile boundary (the trend
     column absorbs first — its floor is 0, see .list-grid). */
  overflow: hidden;
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
/* Group signal strip (2026-08-02): margin-left:auto pins the strip+share
   pair to the header's right side; a max-width keeps the 24 cells from
   turning into sausages on wide tiles. */
.section-strip {
  flex: 1 1 0;
  max-width: 320px;
  margin-left: auto;
}
/* Group share entry: same quiet-icon-button language as the row chevron
   (secondary ink, brand on hover). */
.section-share {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  padding: var(--hs-space-1);
  border: none;
  border-radius: var(--hs-radius-sm);
  background: none;
  color: var(--hs-text-secondary);
  font-size: 14px;
  cursor: pointer;
  transition: color var(--hs-transition);
}
.section-share:hover {
  color: var(--hs-brand);
}
.section-share:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}
/* Column template shared by the head and every row — alignment is a
   construction property of the list, never per-row luck. GH #139 cascade
   (content-box, 1200px content lane; section tile = border 1px×2 +
   padding 8px×2 → inner lane 1182px; row padding 12px×2 → grid lane
   1158px): fixed = protocol 130 + status 100 + rate 210 + p95 100 + action
   40 = 580px; gaps = 6 × 12px (space-3) = 72px; flex remainder 1158 − 580
   − 72 = 506px splits name 1.8fr ≈ 325 / trend 1fr ≈ 181.
   GH #139: the vendor column grew 44 → 100 (family text returns; the
   26px tile + 8px gap now live inside the name cell, leaving ≈317px for
   the model id); the GH #136 vendor tile column (44px) is superseded.
   2026-08-02: the vendor TEXT column becomes the PROTOCOL column (user
   ruling — endpoint = model × protocol, W3; the protocol word
   distinguishes same-id rows, the family word was redundant with the
   name-cell tile) and widens 100 → 130 so images_generation renders in
   full; the GH #140 protocol mini-tag inside the name cell retires (the
   column carries the protocol now). Same round: the STATUS column slims
   150 → 100 (the「· 可用性 / · 延迟」cause suffixes leave the list — the
   badge shows the bare word; causes stay in the detail panel), and the
   RATE column grows 150 → 210: the continuous 0–100 bar is replaced by
   the 24-cell signal strip (number ≈55 + gap 8 + strip ≈147 → cell ≈4px).
   2026-08-01 squeeze-band hardening (round-9, breakpoint moved to 1024):
   the NAME floor is 140 (tile + a truncated id stay readable — the model
   identity is the first hierarchy and must never clip away) and the trend
   floor is 0 (was 150): between 1024px and ~1280px the fixed minimum
   (580 + 140 + 72 = 792px) can exceed the lane, and the trend track is
   the designated shock absorber — it shrinks toward 0 first so the P95 /
   availability numbers stay legible longest; whatever still overflows
   clips at the section tile edge (.list-section overflow), never paints
   over neighbouring cells. */
.list-grid {
  display: grid;
  grid-template-columns: minmax(140px, 1.8fr) 130px 100px 210px 100px minmax(0, 1fr) 40px;
  align-items: center;
  gap: var(--hs-space-3);
}
.list-head {
  padding: 0 var(--hs-space-3) var(--hs-space-2);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  border-bottom: 1px solid var(--hs-border);
}
/* Squeeze-band hardening (2026-08-01): header labels never wrap (「模型」
   used to stack into two lines when its track collapsed) — they clip
   inline instead, mirroring the rows' progressive-clip behavior. */
.list-head > * {
  overflow: hidden;
  white-space: nowrap;
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
.col-proto {
  overflow: hidden;
  white-space: nowrap;
}
.col-action {
  text-align: right;
}
/* Row geometry (2026-08-01 round-6 device feedback): the list is a pure
   RECTANGLE language — no radius anywhere on the row. A border-bottom
   painted on a box with radius-lg curls upward at both corners, tracing a
   U-shaped outline that made every row read as a bordered mini-card, and
   the first row's curled arms collided with the column-header rule; the
   round-6 first cut scoped the radius to the hover fill, and the round-7
   ruling retired it there too — the sharp fill matches the Leaderboard
   brand-soft row fill exactly. */
.list-row {
  padding: var(--hs-space-3);
  border-bottom: 1px solid var(--hs-border-light);
  cursor: pointer;
  transition:
    background-color var(--hs-transition),
    border-color var(--hs-transition);
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
   white-on-white fill), which is now resolved by this caliber.
   Round-7 refinement: the fill is a SHARP rectangle (no radius — user
   ruling) and the row's own hairline fades while hovered, so the fill
   reads clean between the two straight separators of its neighbors. */
.list-row:hover {
  background: var(--hs-bg-hover);
  border-bottom-color: transparent;
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
  /* Squeeze-band hardening (2026-08-01): overflow used to paint the model
     id and the protocol tag OVER the vendor/status columns when the name
     track collapsed below the content minimum (tile 26 + tag ≈70–120 are
     flex:none). Now the cell clips at its track edge and its content wraps
     before it ever clips — the desktop inline layout is untouched
     (everything fits, so nothing wraps there). The tag itself retired on
     2026-08-02 (protocol column), the wrap guard stays for the 已停用
     note. */
  overflow: hidden;
  flex-wrap: wrap;
  row-gap: var(--hs-space-1);
}
.name-text {
  display: flex;
  min-width: 0;
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
/* Click-to-copy (2026-08-02): the model id is a real button (keyboard
   operable) reset onto the name typography — a nested interactive inside
   the row's role=button, its click/keydown stop at itself. Hover shifts
   the ink toward brand as the copy affordance; focus walks the single
   focus language. */
.name-copy {
  background: none;
  border: none;
  padding: 0;
  text-align: left;
  cursor: pointer;
  border-radius: var(--hs-radius-xs);
  transition: color var(--hs-transition);
}
.name-copy:hover {
  color: var(--hs-brand);
}
.name-copy:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
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
/* The GH #140 protocol mini-tag inside the name cell RETIRED on
   2026-08-02 (user ruling): the protocol now has its own column, so the
   name cell is tile + id + 已停用 note only. The colored tag form lives
   on in the other protocol.ts consumers (detail page, EndpointTable,
   StatusCard chips). */
.disabled-tag {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.cell-proto {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
/* The uniform vendor tile (GH #136/#139/#140) lives in VendorTile.vue —
   extracted in the 2026-08-01 narrow-card batch; the group header, the
   desktop row, and the narrow card share that single implementation. */
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
/* The continuous 0–100 inline bar (GH #131/#136) RETIRED on 2026-08-02 —
   the availability visual is the 24-cell UptimeMicroStrip now (one cell =
   one hour, tier-colored by the overviewDots single mapping). The number
   keeps its tier *-text grade; the strip carries the graphic-grade tier
   colors per cell (graphic/text division). */
.cell-rate.tier-success .rate-value {
  color: var(--hs-success-text);
}
.cell-rate.tier-warning .rate-value {
  color: var(--hs-warning-text);
}
.cell-rate.tier-danger .rate-value {
  color: var(--hs-danger-text);
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
  /* The trend track is the squeeze-band shock absorber (floor 0) — clip
     the sparkline at the track edge rather than letting it paint out. */
  overflow: hidden;
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

/* --- Narrow card form (2026-08-01 shell drawer batch) ----------------------
   Stacked cards inside the same white section tile. The geometry mirrors
   the desktop rows one-to-one: straight hairlines between cards, the last
   card drops its line, hover = plain fill with its own hairline faded —
   the round-7 pure-rectangle language, no radius anywhere. */
.m-card {
  padding: var(--hs-space-3);
  border-bottom: 1px solid var(--hs-border-light);
  cursor: pointer;
  transition:
    background-color var(--hs-transition),
    border-color var(--hs-transition);
}
.list-section .m-card:last-child {
  border-bottom: none;
}
.m-card:hover {
  background: var(--hs-bg-hover);
  border-bottom-color: transparent;
}
.m-card:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}
.m-card-top {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
/* The name lane flex-fills so the chevron pins right. */
.m-card-top .name-text {
  flex: 1 1 auto;
}
.m-card-top .detail-chevron {
  margin-left: auto;
  flex: none;
}
.m-card-mid {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  margin-top: var(--hs-space-2);
  min-width: 0;
}
/* Protocol word on the card's mid line (xs secondary): mirrors the
   desktop protocol column (2026-08-02 — it was the family word before
   that column became the protocol column; vendor identity on the card is
   carried by the top-line tile). */
.m-proto {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.m-card-stats {
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  margin-top: var(--hs-space-2);
}
.m-card-stats .cell-rate {
  flex: 1 1 0;
}
.m-p95 {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
</style>
