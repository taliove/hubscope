<template>
  <div
    class="health-banner"
    :class="[isInitialLoading || isEmpty ? 'banner-loading' : `banner-${tone}`, { 'banner-clickable': clickable }]"
    @click="onClick"
  >
    <!-- Fixed-height skeleton for the first load so the banner never jumps. -->
    <div v-if="isInitialLoading" class="banner-skeleton" aria-hidden="true">
      <div class="skeleton-line skeleton-title" />
      <div class="skeleton-line skeleton-sub" />
    </div>
    <template v-else>
      <div class="conclusion-row">
        <!-- Failing endpoints reuse the StatusBadge alert blink (W5 semantics);
             the blink lives ONLY here — chips below never animate. -->
        <span v-if="hasFailing" class="alert-dot" title="存在告警端点" />
        <span class="conclusion">{{ conclusion }}</span>
        <!-- Meta area pins to the row's right end; the cadence segment
             truncates first when space runs out. -->
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
      <div class="detail-row">
        <!-- 24h availability big number (display tier, ink — never tinted:
             the banner ground + conclusion already double-encode severity). -->
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
        <!-- Which endpoints are abnormal (GH #53): most severe first, capped
             at MAX_ABNORMAL_CHIPS with a neutral "+N" overflow. No dots, no
             blink, no fill — the alert-dot above owns the animation. -->
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
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
// Global health banner: the first visual layer of the Dashboard. Row 1 is the
// one-sentence conclusion (all healthy / N degraded / N abnormal) plus the
// right-aligned meta (stale chip + updated-at + poll cadence); row 2 is the
// display-tier 24h availability big number plus the abnormal-endpoint chips
// (GH #53 — the banner answers "WHICH endpoints", status counts live only in
// the stats strip below). Four states per spec 0003 §5.1: healthy / degraded
// / abnormal / loading. The banner always reflects the global picture and
// never the active filters.
import { computed } from 'vue'
import type { OverviewEntry, EndpointStatus } from '@/api/types'
import { formatPercentDigits, formatTime } from '@/utils/format'
import {
  STATUS_LABELS,
  abnormalChips,
  conclusionText,
  countByStatus,
  toneOf,
} from '@/utils/healthConclusion'
import { scopedAvailability } from '@/utils/statusCardSummary'
import { POLL_INTERVAL_MS } from '@/composables/useOverview'

const props = defineProps<{
  entries: OverviewEntry[]
  generatedAt: string | null
  loading: boolean
  stale: boolean // last refresh failed; showing previous successful data
}>()

const emit = defineEmits<{ (e: 'inspect', status: EndpointStatus): void }>()

// Only the very first load (no data yet) shows the skeleton; subsequent
// poll refreshes keep the previous numbers on screen.
const isInitialLoading = computed(() => props.loading && props.entries.length === 0)
// Loaded but zero endpoints (or a first-load failure with nothing to show):
// render a neutral "no data" state instead of a misleading "全部正常".
const isEmpty = computed(() => !isInitialLoading.value && props.entries.length === 0)

// Banner math only counts enabled endpoints; disabled ones are the strip's
// "已停用" item and never enter the conclusion (spec 0003 §5.1).
const enabledEntries = computed(() => props.entries.filter(e => e.enabled))

// Conclusion math comes from the shared healthConclusion module (same words
// and thresholds as the StatusCard); only enabled endpoints enter the counts.
const counts = computed(() => countByStatus(enabledEntries.value))

const tone = computed(() => toneOf(counts.value))

const hasFailing = computed(() => counts.value.failing > 0)
// Only the abnormal state is clickable as a whole card (apply status filter
// + scroll); chips stay clickable in the degraded state too.
const clickable = computed(() => !isEmpty.value && tone.value === 'abnormal')

const conclusion = computed(() => conclusionText(tone.value, counts.value, isEmpty.value))

// Probe-weighted 24h availability over the enabled entries — identical to the
// backend aggregate by construction (scopedAvailability, statusCardSummary).
const availability = computed(() => scopedAvailability(enabledEntries.value))

// Which endpoints are abnormal, most severe first (single severity rank).
const abnormal = computed(() => abnormalChips(enabledEntries.value))

const pollSeconds = POLL_INTERVAL_MS / 1000

// "HH:mm" sliced out of the shared formatTime helper ("YYYY-MM-DD HH:mm:ss").
const updatedAt = computed(() => {
  if (!props.generatedAt) return null
  const full = formatTime(props.generatedAt)
  return full.length >= 16 ? full.slice(11, 16) : full
})

function onClick() {
  if (!clickable.value) return
  // Filter to the more urgent abnormal status first.
  emit('inspect', hasFailing.value ? 'failing' : 'down')
}

// Chip click = the same inspect path as the whole-card click (status filter
// + scroll to the matrix), stopped so it never triggers the card click.
function onChipInspect(status: EndpointStatus) {
  emit('inspect', status)
}
</script>

<style scoped>
.health-banner {
  border-radius: var(--hs-radius-lg);
  padding: 16px 20px;
  margin-bottom: 16px;
  transition: box-shadow 0.15s ease;
}
.banner-clickable {
  cursor: pointer;
}
.banner-clickable:hover {
  box-shadow: var(--hs-shadow-md);
}
/* Initial-load skeleton sits on a neutral ground (spec 0003 §5.1), never on
   a status tint — green would falsely signal "all healthy" before data. */
.banner-loading {
  background: var(--hs-bg-page);
  border: 1px solid var(--hs-border);
}
.banner-healthy {
  background: var(--hs-success-soft);
}
.banner-healthy .conclusion {
  color: var(--hs-success);
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
/* Same pulse as StatusBadge's failing dot (sole animated state, §3). */
.alert-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-status-failing);
  animation: hs-blink 0.8s ease-in-out infinite;
}
/* Meta area: right end of row 1. The cadence segment truncates first. */
.meta {
  margin-left: auto;
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
  padding: 0 6px;
}
/* Row 2: availability big number + abnormal chips. */
.detail-row {
  display: flex;
  align-items: flex-end;
  gap: var(--hs-space-2) var(--hs-space-5);
  flex-wrap: wrap;
  margin-top: var(--hs-space-2);
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
  flex: 1;
}
/* Abnormal chip: outline only — no dot, no blink, no fill (W5 mirror: the
   alert-dot owns the sole animation). */
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--hs-space-2);
  max-width: 240px;
  padding: var(--hs-space-1) var(--hs-space-2);
  font: inherit;
  background: transparent;
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
  cursor: pointer;
  transition: border-color var(--hs-transition);
}
.chip:hover {
  border-color: var(--hs-brand);
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
.chip-overflow {
  flex: none;
  padding: var(--hs-space-1) var(--hs-space-2);
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
}
/* Skeleton mirrors the loaded layout heights (28px display conclusion +
   availability block) so first paint does not shift content below. */
.banner-skeleton {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
  min-height: 104px;
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
