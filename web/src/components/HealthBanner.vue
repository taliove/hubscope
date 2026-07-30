<template>
  <!-- Hero command band (GH #73, surface brief Hero 指挥台带节): the single
       first-screen surface merging the GH #53 banner card and the stats
       strip — conclusion + abnormal chips + status counts + 24h availability.
       Whole-band activation (a11y harden 2026-07-29): only the abnormal
       state is clickable — then the root exposes role="button" + tabindex
       and Enter/Space trigger the same inspect path as click. Non-clickable
       states carry no role and stay out of the tab order. -->
  <div
    class="health-banner"
    :class="[isInitialLoading || isEmpty ? 'banner-loading' : `banner-${tone}`, { 'banner-clickable': clickable }]"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="onClick"
    @keydown="onBandKeydown"
  >
    <!-- Fixed-height skeleton for the first load so the band never jumps. -->
    <div v-if="isInitialLoading" class="banner-skeleton" aria-hidden="true">
      <div class="skeleton-line skeleton-title" />
      <div class="skeleton-line skeleton-sub" />
    </div>
    <div v-else class="band-body">
      <div class="band-left">
        <div class="conclusion-row">
          <!-- Failing endpoints reuse the StatusBadge alert blink (W5 semantics);
               the blink lives ONLY here — chips and counts never animate. -->
          <span v-if="hasFailing" class="alert-dot" title="存在告警端点" />
          <span class="conclusion">{{ conclusion }}</span>
        </div>
        <!-- Which endpoints are abnormal (GH #53): most severe first, capped
             at MAX_ABNORMAL_CHIPS with a neutral "+N" overflow. No dots, no
             blink — the alert-dot above owns the animation. -->
        <div v-if="abnormal.chips.length > 0" class="chips">
          <button
            v-for="chip in abnormal.chips"
            :key="chip.endpoint_id"
            type="button"
            class="chip"
            :title="`${chip.model_id} · ${chip.protocol} · ${STATUS_LABELS[chip.status]}`"
            @click.stop="onChipInspect(chip.status)"
          >
            <span class="chip-status" :class="`chip-status-${chip.status}`">{{ STATUS_LABELS[chip.status] }}</span>
            <span class="chip-model">{{ chip.model_id }}</span>
          </button>
          <span v-if="abnormal.overflow > 0" class="chip-overflow">+{{ abnormal.overflow }}</span>
        </div>
        <!-- Counts row (GH #73, migrated from the stats strip; behavior kept
             verbatim): total + four status counts (SEVERITY_ORDER heavy →
             light, the board's single severity caliber, GH #55) + disabled.
             Status items toggle the filter / click again to clear; the active
             item gets a 1px brand inset ring on a transparent ground. Items
             are real <button>s (keyboard Enter/Space + focus ring). Clicks
             stop at the item so they never trigger the whole-band inspect. -->
        <div class="counts-row">
          <button
            type="button"
            class="count-item count-clickable"
            :class="{ 'count-active': statusFilter === '' }"
            @click.stop="emit('toggle-status', '')"
          >
            总数 <span class="count-num">{{ entries.length }}</span>
          </button>
          <button
            v-for="status in SEVERITY_ORDER"
            :key="status"
            type="button"
            class="count-item count-clickable"
            :class="{ 'count-active': statusFilter === status }"
            @click.stop="emit('toggle-status', status)"
          >
            <StatusBadge :status="status" />
            <span class="count-num">{{ statusCounts[status] }}</span>
          </button>
          <span v-if="disabledCount > 0" class="count-item count-disabled">
            已停用 <span class="count-num">{{ disabledCount }}</span>
          </span>
        </div>
      </div>
      <!-- Right column: display-tier 24h availability big number (ink — never
           tinted: the band ground + conclusion already double-encode
           severity) plus the meta line (stale chip + updated-at + poll
           cadence; the cadence segment truncates first). margin-left:auto
           pins it right; narrow screens wrap it below the left column. -->
      <div class="band-right">
        <div class="availability">
          <span class="availability-label">24h 可用率</span>
          <span class="availability-line">
            <span class="availability-value" :class="{ 'availability-null': availability === null }">
              <template v-if="availability !== null">
                {{ formatPercentDigits(availability) }}<span class="availability-unit">%</span>
              </template>
              <template v-else>-</template>
            </span>
            <span v-if="availability === null" class="availability-note">24h 内无探测数据</span>
          </span>
        </div>
        <div class="meta">
          <span v-if="stale" class="stale-note">数据非最新</span>
          <span class="meta-text">
            <template v-if="updatedAt">
              <span class="meta-updated">更新于 {{ updatedAt }}</span>
              <span class="meta-sep" aria-hidden="true">·</span>
            </template>
            <span class="meta-cadence">每 {{ pollSeconds }} 秒自动刷新</span>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// Hero command band: the first visual layer of the Dashboard, one flat
// surface (GH #73 — the GH #53 banner card and the stats strip merged).
// Left column: one-sentence conclusion (all healthy / N degraded / N
// abnormal) + abnormal-endpoint chips + the status-counts row (click to
// filter, click again to clear — the same statusFilter ref as the filter-row
// select, GH #55 dual-control discipline). Right column: display-tier 24h
// availability + meta (stale chip / updated-at / cadence). Four states per
// spec 0003 §5.1: healthy / degraded / abnormal / loading. The band always
// reflects the global picture and never the active filters.
import { computed } from 'vue'
import type { OverviewEntry, EndpointStatus } from '@/api/types'
import StatusBadge from '@/components/StatusBadge.vue'
import { formatPercentDigits, formatClockMinute } from '@/utils/format'
import {
  STATUS_LABELS,
  abnormalChips,
  conclusionText,
  countByStatus,
  toneOf,
  type HealthCounts,
} from '@/utils/healthConclusion'
import { SEVERITY_ORDER } from '@/utils/severitySort'
import { scopedAvailability } from '@/utils/statusCardSummary'
import { POLL_INTERVAL_MS } from '@/composables/useOverview'

const props = defineProps<{
  entries: OverviewEntry[]
  generatedAt: string | null
  loading: boolean
  stale: boolean // last refresh failed; showing previous successful data
  // Counts row (migrated strip): all-entries status counts, the disabled
  // count, and the active status filter (dual-controlled with the
  // filter-row select — DashboardView owns the single ref).
  statusCounts: HealthCounts
  disabledCount: number
  statusFilter: EndpointStatus | ''
}>()

const emit = defineEmits<{
  (e: 'inspect', status: EndpointStatus): void
  // Counts-row click: '' (the 总数 item) clears the filter; a status payload
  // toggles it (click again clears). DashboardView owns the toggle math.
  (e: 'toggle-status', status: EndpointStatus | ''): void
}>()

// Only the very first load (no data yet) shows the skeleton; subsequent
// poll refreshes keep the previous numbers on screen.
const isInitialLoading = computed(() => props.loading && props.entries.length === 0)
// Loaded but zero endpoints (or a first-load failure with nothing to show):
// render a neutral "no data" state instead of a misleading "全部正常".
const isEmpty = computed(() => !isInitialLoading.value && props.entries.length === 0)

// Conclusion math only counts enabled endpoints; disabled ones are the
// counts row's "已停用" item and never enter the conclusion (spec 0003 §5.1).
const enabledEntries = computed(() => props.entries.filter(e => e.enabled))

// Conclusion math comes from the shared healthConclusion module (same words
// and thresholds as the StatusCard); only enabled endpoints enter the counts.
const counts = computed(() => countByStatus(enabledEntries.value))

const tone = computed(() => toneOf(counts.value))

const hasFailing = computed(() => counts.value.failing > 0)
// Only the abnormal state is clickable as a whole band (apply status filter
// + scroll); chips and counts stay clickable in the degraded state too.
const clickable = computed(() => !isEmpty.value && tone.value === 'abnormal')

const conclusion = computed(() => conclusionText(tone.value, counts.value, isEmpty.value))

// Probe-weighted 24h availability over the enabled entries — identical to the
// backend aggregate by construction (scopedAvailability, statusCardSummary).
const availability = computed(() => scopedAvailability(enabledEntries.value))

// Which endpoints are abnormal, most severe first (single severity rank).
const abnormal = computed(() => abnormalChips(enabledEntries.value))

const pollSeconds = POLL_INTERVAL_MS / 1000

// Bare "HH:mm" freshness reading via the shared clock helper (no slicing).
const updatedAt = computed(() => (props.generatedAt ? formatClockMinute(props.generatedAt) : null))

function onClick() {
  if (!clickable.value) return
  // Filter to the more urgent abnormal status first.
  emit('inspect', hasFailing.value ? 'failing' : 'down')
}

// Whole-band keyboard activation fires ONLY when the band root itself is the
// event target — child buttons (chips, count items) handle their own
// Enter/Space; guarding here keeps their default click activation intact
// (a .prevent modifier on the root would suppress it) and prevents a child
// keydown from double-firing the band inspect.
function onBandKeydown(event: KeyboardEvent) {
  if (event.target !== event.currentTarget) return
  if (event.key !== 'Enter' && event.key !== ' ') return
  event.preventDefault()
  onClick()
}

// Chip click = the same inspect path as the whole-band click (status filter
// + scroll to the matrix), stopped so it never triggers the band click.
function onChipInspect(status: EndpointStatus) {
  emit('inspect', status)
}
</script>

<style scoped>
/* Band form (GH #73): a flat full-width band — no radius, no border box —
   closed by a 1px hairline; tone-soft grounds (GH #53) carry the severity
   double encoding (signal-wall metaphor, plan verdict 2026-07-30). */
.health-banner {
  padding: var(--hs-space-4);
  margin-bottom: var(--hs-space-4);
  border-bottom: 1px solid var(--hs-border-light);
  transition: box-shadow var(--hs-transition);
}
.banner-clickable {
  cursor: pointer;
}
.banner-clickable:hover {
  box-shadow: var(--hs-shadow-md);
}
/* Keyboard focus = the board's single focus language (1px brand inset ring);
   paired with shadow-md like the chips below. */
.banner-clickable:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--hs-brand), var(--hs-shadow-md);
}
/* Initial-load skeleton and the empty state sit on a neutral ground
   (spec 0003 §5.1), never on a status tint — green would falsely signal
   "all healthy" before data. No border box: the hairline above closes it. */
.banner-loading {
  background: var(--hs-bg-page);
}
.banner-healthy {
  background: var(--hs-success-soft);
}
/* Healthy conclusion text consumes the deepened text grade (graphic/text
   division, GH #71): the --hs-success body fails AA on tinted grounds. */
.banner-healthy .conclusion {
  color: var(--hs-success-text);
}
.banner-degraded {
  background: var(--hs-warning-soft);
}
.banner-degraded .conclusion {
  color: var(--hs-warning);
}
.banner-abnormal {
  background: var(--hs-danger-soft);
}
.banner-abnormal .conclusion {
  color: var(--hs-danger);
}
/* Two columns; the right one wraps below the left on narrow widths. */
.band-body {
  display: flex;
  flex-wrap: wrap;
  gap: var(--hs-space-2) var(--hs-space-5);
}
.band-left {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.band-right {
  flex: 0 0 auto;
  margin-left: auto;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
.conclusion-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
}
.conclusion {
  font-size: var(--hs-text-display);
  font-weight: 600;
  line-height: 1.5;
}
/* Same pulse as StatusBadge's failing dot (sole animated state, §3);
   --hs-blink (semantics.css) goes still under prefers-reduced-motion. */
.alert-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-status-failing);
  animation: var(--hs-blink);
}
/* Meta line (right column, under availability). The cadence segment
   truncates first when space runs out. */
.meta {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
.meta-text {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  white-space: nowrap;
  min-width: 0;
}
.meta-updated {
  flex: none;
}
.meta-sep {
  flex: none;
}
.meta-cadence {
  overflow: hidden;
  text-overflow: ellipsis;
}
.stale-note {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
  padding: 0 var(--hs-space-2);
}
.availability {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
  flex: none;
}
.availability-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.availability-line {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
}
.availability-value {
  font-size: var(--hs-text-display);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.availability-null {
  color: var(--hs-text-placeholder);
}
.availability-unit {
  font-size: var(--hs-text-md);
  font-weight: 400;
  color: var(--hs-text-secondary);
  margin-left: var(--hs-space-1);
}
.availability-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
/* Abnormal chip: borderless surface card on the tinted ground — no dot, no
   blink, no status fill (W5 mirror: the alert-dot owns the sole animation).
   Borderless bg-card + radius reads as a clean card on any tone-soft ground
   (user feedback 2026-07-29: the outline was ugly even after the surface
   fill); hover lifts with shadow-md — shadow means "clickable" per the
   elevation rule. */
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--hs-space-2);
  max-width: 240px;
  padding: var(--hs-space-1) var(--hs-space-2);
  font: inherit;
  background: var(--hs-bg-card);
  border: none;
  border-radius: var(--hs-radius-sm);
  cursor: pointer;
  transition: box-shadow var(--hs-transition);
}
.chip:hover {
  box-shadow: var(--hs-shadow-md);
}
/* Keyboard focus mirrors the counts row's selected ring (audit 2026-07-29):
   borderless chips otherwise leave keyboard users without a focus cue. */
.chip:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--hs-brand), var(--hs-shadow-md);
}
.chip-status {
  flex: none;
  font-size: var(--hs-text-sm);
  font-weight: 600;
}
.chip-status-failing {
  color: var(--hs-status-failing);
}
.chip-status-down {
  color: var(--hs-danger);
}
.chip-status-degraded {
  color: var(--hs-warning);
}
.chip-model {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Inert overflow count: text-only, deliberately un-framed so it never reads
   as a disabled chip next to the clickable, surface-backed ones. */
.chip-overflow {
  flex: none;
  padding: var(--hs-space-1) 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
}
/* Counts row (migrated stats strip, styles kept verbatim under count-*
   names): one slim row of counts, status items toggle the filter. */
.counts-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-4);
  flex-wrap: wrap;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
}
.count-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.count-clickable {
  /* Button reset (a11y harden 2026-07-29): the clickable items are real
     buttons — strip the UA button chrome, inherit row typography. */
  font: inherit;
  color: inherit;
  background: none;
  border: none;
  cursor: pointer;
  padding: var(--hs-space-1) var(--hs-space-2);
  border-radius: var(--hs-radius-sm);
  transition: color var(--hs-transition), background-color var(--hs-transition), box-shadow var(--hs-transition);
}
.count-clickable:hover {
  color: var(--hs-brand-hover);
}
/* Selected filter = brand outline + transparent ground + brand text
   (user feedback 2026-07-29): brand keeps the "active selection" language,
   an inset ring avoids layout shift, and no fill block fights the status
   words' semantic colors. */
.count-active {
  color: var(--hs-brand);
  background-color: transparent;
  box-shadow: inset 0 0 0 1px var(--hs-brand);
}
/* Keyboard focus mirrors the selected ring (a11y harden 2026-07-29) — the
   single focus language of the board: 1px brand inset ring. */
.count-clickable:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--hs-brand);
}
.count-num {
  font-weight: 600;
  color: var(--hs-text-primary);
}
.count-disabled {
  color: var(--hs-text-secondary);
}
/* Skeleton mirrors the loaded band height (conclusion 42 + chips 28 +
   counts 28 + two 8px gaps = 114px) so first paint does not shift content
   below. Registered trade-off (check GH #73 LOW-1): the 114px anchor is
   the chips-present layout (degraded/abnormal); a healthy first load has
   no chips row, so the loaded band lands ~30px shorter and content below
   rises slightly. One fixed height cannot match all four states (the
   state is unknown while loading) — the anchor favors the abnormal
   states, where layout stability matters most. */
.banner-skeleton {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
  min-height: 114px;
}
.skeleton-line {
  border-radius: var(--hs-radius-sm);
  background: var(--hs-border-light);
  animation: pulse 1.2s ease-in-out infinite;
}
.skeleton-title {
  width: 220px;
  height: 28px;
}
.skeleton-sub {
  width: 340px;
  max-width: 70%;
  height: 34px;
}
@keyframes pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}

@media (prefers-reduced-motion: reduce) {
  .skeleton-line {
    animation: none;
  }
}
</style>
