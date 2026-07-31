<template>
  <div>
    <!-- Detail: abnormal rows (status + name + 24h rate, reason, dots) /
         all-healthy statement / empty wording, then the healthy roster
         (GH #92) and the one-sentence 小结. -->
    <template v-if="emptyText">
      <div class="detail-empty">{{ emptyText }}</div>
    </template>
    <template v-else>
      <template v-if="abnormalEntries.length > 0">
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
      </template>
      <!-- All-healthy statement (no abnormal entries): the explicit positive
           conclusion is the anti-fake counterpart of the abnormal list — kept
           WITHOUT the rate-range suffix (GH #92: the roster below carries the
           per-entry rates, a superset of the range text). -->
      <div v-else-if="singleModel" class="healthy-line detail-healthy">
        当前状态<span class="healthy-word">正常</span>{{ rangeText }}
      </div>
      <div v-else class="healthy-line detail-healthy">
        全部 <span class="healthy-num">{{ entries.length }}</span> 个端点<span class="healthy-word">正常</span>
      </div>

      <!-- Healthy roster (GH #92, share-materials brief): every healthy
           endpoint listed by name — the card must not parade only the
           abnormal ones. Name + 24h rate only (no protocol, no dots: a
           roster, not a second detail section — dots would dilute the
           abnormal list's primacy). Sorted most-fragile-first by the pure
           function; single-model mode has no "其余" and skips the section;
           an all-abnormal scope renders nothing (no fake empty state). -->
      <template v-if="!singleModel && roster.rows.length > 0">
        <div class="detail-title">正常端点</div>
        <div class="roster-grid">
          <div v-for="entry in roster.rows" :key="entry.endpoint_id" class="roster-item">
            <span class="roster-name" :title="entry.model_id">{{ entry.model_id }}</span>
            <span class="roster-rate" :class="`av-${availabilityTier(entry.success_rate_24h)}`">
              {{ formatPercent(entry.success_rate_24h) }}
            </span>
          </div>
        </div>
        <div v-if="roster.overflow > 0" class="detail-more">
          另有 {{ roster.overflow }} 个正常端点未列出,详见状态板
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
import { STATUS_LABELS } from '@/utils/healthConclusion'
import { availabilityTier, dotTier, healthyRangeText, healthyRoster } from '@/utils/statusCardSummary'
import { SEVERITY_RANK } from '@/utils/severitySort'

const props = defineProps<{
  entries: OverviewEntry[] // scoped ENABLED entries only
  emptyText: string // non-empty renders the empty state instead of the list
  summary: string | null
  // Single-model mode (design ruling): no healthy roster (there is no
  // remainder), and the all-healthy line drops the count ("当前状态正常").
  singleModel?: boolean
}>()

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
  color: var(--hs-success);
}
.st-degraded {
  color: var(--hs-warning);
}
.st-down {
  color: var(--hs-danger);
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
/* GH #69: success as TEXT always consumes the deepened text grade (the base
   green is graphics-only); dots (.seg-ok) keep the base as graphic fills. */
.av-ok {
  color: var(--hs-success-text);
}
.av-partial {
  color: var(--hs-warning);
}
.av-fail {
  color: var(--hs-danger);
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
  margin-top: 8px;
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
  column-gap: 16px;
  row-gap: 4px;
}
.roster-item {
  display: flex;
  align-items: baseline;
  gap: 8px;
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
  color: var(--hs-success);
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
