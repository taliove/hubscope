<template>
  <div>
    <!-- Detail: abnormal rows (status + name + 24h rate, reason, dots) /
         all-healthy statement / empty wording, then the healthy roster
         (GH #92) and the one-sentence 小结. Status words come from the
         display-layer mapping (GH #113), never literals. -->
    <template v-if="emptyText">
      <div class="detail-empty">{{ emptyText }}</div>
    </template>
    <template v-else>
      <template v-if="abnormalEntries.length > 0">
        <div class="detail-title">异常明细</div>
        <div v-for="entry in topAbnormal" :key="entry.endpoint_id" class="detail-item">
          <div class="detail-row">
            <span class="row-status" :class="`st-${statusTone(entry.status)}`">{{ statusLabel(entry.status) }}</span>
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
      </template>
      <!-- All-healthy statement (no abnormal entries): the explicit positive
           conclusion is the anti-fake counterpart of the abnormal list — kept
           WITHOUT the rate-range suffix (GH #92: the roster below carries the
           per-entry rates, a superset of the range text). The healthy word
           itself comes from the display-layer mapping (GH #113). -->
      <div v-else-if="singleModel" class="healthy-line detail-healthy">
        当前状态<span class="healthy-word">{{ stableWord }}</span>{{ rangeText }}
      </div>
      <div v-else class="healthy-line detail-healthy">
        全部 <span class="healthy-num">{{ entries.length }}</span> 个端点<span class="healthy-word">{{ stableWord }}</span>
      </div>

      <!-- Healthy roster (GH #92, share-materials brief): every healthy
           endpoint listed by name — the card must not parade only the
           abnormal ones. Name + 24h rate only (no protocol, no dots: a
           roster, not a second detail section — dots would dilute the
           abnormal list's primacy). Sorted most-fragile-first by the pure
           function; single-model mode has no "其余" and skips the section;
           an all-abnormal scope renders nothing (no fake empty state). -->
      <template v-if="!singleModel && roster.rows.length > 0">
        <div class="detail-title">{{ stableWord }}端点</div>
        <div class="roster-grid">
          <div v-for="entry in roster.rows" :key="entry.endpoint_id" class="roster-item">
            <span class="roster-name" :title="entry.model_id">{{ entry.model_id }}</span>
            <span class="roster-rate" :class="`av-${availabilityTier(entry.success_rate_24h)}`">
              {{ formatPercent(entry.success_rate_24h) }}
            </span>
          </div>
        </div>
        <div v-if="roster.overflow > 0" class="detail-more">
          另有 {{ roster.overflow }} 个{{ stableWord }}端点未列出,详见状态板
        </div>
      </template>
    </template>

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
// StatusCard detail blocks (ticket 59; healthy roster GH #92): the capped
// abnormal list, the all-healthy statement, the healthy roster, and the
// one-sentence 小结. Computes everything from the scoped enabled-entry set
// via the statusCardSummary pure functions; the parent only supplies the
// empty-state wording (it depends on the disabled-inclusive count, which
// lives upstream).
import { computed } from 'vue'
import type { OverviewEntry } from '@/api/types'
import { formatPercent } from '@/utils/format'
import { statusLabel, statusTone } from '@/utils/statusDisplay'
import { availabilityTier, dotTier, healthyRangeText, healthyRoster } from '@/utils/statusCardSummary'
import { SEVERITY_RANK } from '@/utils/severitySort'

const props = defineProps<{
  entries: OverviewEntry[] // scoped ENABLED entries only
  emptyText: string // non-empty renders the empty state instead of the list
  summary: string | null
  // Single-model mode (design ruling): no healthy roster (there is no
  // remainder), and the all-healthy line drops the count ("当前状态稳定",
  // GH #113 three-state wording).
  singleModel?: boolean
}>()

// The all-healthy word comes from the display-layer mapping like every
// other status word (GH #113) — no literals in the template.
const stableWord = statusLabel('stable')

// Cap the abnormal list so a widespread outage cannot produce an unbounded
// tall image; the footer origin is the escape hatch to the live board.
const MAX_DETAIL_ROWS = 10

// Severity table comes from the single source (utils/severitySort, GH #52);
// the detail list keeps its own localeCompare name tie-break.
const abnormalEntries = computed(() =>
  props.entries
    .filter(e => e.status !== 'healthy')
    .sort(
      (a, b) =>
        SEVERITY_RANK[a.status] - SEVERITY_RANK[b.status] || a.model_id.localeCompare(b.model_id),
    ),
)
const topAbnormal = computed(() => abnormalEntries.value.slice(0, MAX_DETAIL_ROWS))
const overflowCount = computed(() => abnormalEntries.value.length - topAbnormal.value.length)
// Healthy roster (GH #92): sort + cap live in the pure function so the
// component stays presentational.
const roster = computed(() => healthyRoster(props.entries))
// Single-model line only: the aggregate usages retired with the roster.
const rangeText = computed(() => healthyRangeText(props.entries))
</script>

<style scoped>
.divider {
  /* Hairline rhythm (GH #121, same line-lightening as GH #118). */
  border-top: 1px solid var(--hs-border-light);
}
.detail-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-secondary);
  margin: var(--hs-space-5) 0 var(--hs-space-2);
}
.detail-item {
  padding: var(--hs-space-2) 0;
}
.detail-item + .detail-item {
  border-top: 1px solid var(--hs-border-light);
}
.detail-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  min-height: 24px;
}
.row-status {
  flex: none;
  /* Three-state words are up to 4 chars (性能下降/服务异常, GH #113) — the
     old 2-char 28px column widens; the reason/dots indent below tracks it. */
  width: 56px;
  font-size: var(--hs-text-sm);
  font-weight: 600;
}
/* Row status words: text channel → *-text grades (GH #69 text/graphics
   split; GH #113 tone slots success/warning/danger). Only abnormal rows
   render here, so success never appears — kept for completeness. */
.st-success {
  color: var(--hs-success-text);
}
.st-warning {
  color: var(--hs-warning-text);
}
.st-danger {
  color: var(--hs-danger-text);
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
/* Rate figures are text: the *-text grade of each slot (GH #69
 * text/graphics split; on the v2 palette the warning/danger bases are
 * graphic-tier and fail as text, GH #121). The per-endpoint dots below
 * keep the bases as graphic fills. */
.av-ok {
  color: var(--hs-success-text);
}
.av-partial {
  color: var(--hs-warning-text);
}
.av-fail {
  color: var(--hs-danger-text);
}
.av-none {
  color: var(--hs-text-placeholder);
}
.row-reason {
  /* Aligns with the name column (56px status + 12px gap); clamped to two
     lines — a static export has no hover to reveal the rest. */
  margin: var(--hs-space-1) 0 0 68px;
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
  margin: var(--hs-space-1) 0 0 68px;
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
.detail-more {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-top: var(--hs-space-2);
}
/* Healthy roster (GH #92): two equal columns — repeat(2, 1fr) resolves to
   (640 − 16) / 2 = 312px on the 720 card and (440 − 16) / 2 = 212px on the
   480 compact variant (full cascade in the share-materials brief arithmetic
   table), so GH #93 gets the narrow layout by reusing this block as-is.
   Rows are compact (4px) so twenty names stay light against the abnormal
   detail above. */
.roster-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: var(--hs-space-4);
  row-gap: var(--hs-space-1);
}
.roster-item {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
  min-width: 0;
}
.roster-name {
  flex: 1;
  min-width: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.roster-rate {
  flex: none;
  font-size: var(--hs-text-sm);
  font-weight: 600;
  text-align: right;
}
.healthy-line {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-top: var(--hs-space-3);
}
.detail-healthy {
  font-size: var(--hs-text-md);
  margin-top: var(--hs-space-5);
}
.healthy-num {
  color: var(--hs-text-primary);
  font-weight: 600;
}
/* GH #69 text/graphics split: the healthy word is text — deepened grade. */
.healthy-word {
  color: var(--hs-success-text);
  font-weight: 600;
}
.detail-empty {
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
  margin-top: var(--hs-space-5);
}
.summary-row {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
  margin-top: var(--hs-space-3);
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
