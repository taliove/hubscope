<template>
  <div class="status-card">
    <div class="brand-bar" />
    <div class="brand-section">
      <img src="/logo.png" alt="HubScope" class="brand-logo" />
      <span class="brand-title">HubScope 服务状态</span>
    </div>

    <div class="card-body">
      <!-- Scope subtitle: the anti-fake line. Every active filter shows up as
           a chip (none omitted); no filters → the plain "全部端点" line. -->
      <div v-if="scopeChips.length > 0" class="scope-row">
        <span v-for="chip in scopeChips" :key="chip.label" class="scope-chip">
          <span class="chip-label">{{ chip.label }}</span>
          <span class="chip-value" :class="chip.tone ? `value-${chip.tone}` : ''" :title="chip.value">
            {{ chip.value }}
          </span>
        </span>
      </div>
      <div v-else class="scope-plain">全部端点</div>

      <div class="conclusion-block" :class="`tone-${conclusionTone}`">
        <span v-if="hasFailing" class="alert-dot" />
        <span class="conclusion-text">{{ conclusion }}</span>
        <span v-if="hasFailing" class="failing-chip">含 {{ counts.failing }} 个告警</span>
      </div>

      <div class="time-row">
        生成于 {{ timeText }}
        <span v-if="disabledCount > 0" class="disabled-note">另有 {{ disabledCount }} 个已停用</span>
      </div>

      <div class="divider" />

      <!-- Detail: abnormal list (capped) / all-healthy line / empty wording. -->
      <template v-if="isEmpty">
        <div class="detail-empty">{{ emptyDetailText }}</div>
      </template>
      <template v-else-if="abnormalEntries.length > 0">
        <div class="detail-title">异常明细</div>
        <div v-for="entry in topAbnormal" :key="entry.endpoint_id" class="detail-row">
          <span class="row-status" :class="`st-${entry.status}`">{{ STATUS_LABELS[entry.status] }}</span>
          <span class="row-name" :title="`${entry.model_id} · ${entry.protocol}`">
            {{ entry.model_id }} · {{ entry.protocol }}
          </span>
        </div>
        <div v-if="overflowCount > 0" class="detail-more">
          另有 {{ overflowCount }} 个异常端点未列出,详见状态板
        </div>
      </template>
      <div v-else class="detail-healthy">全部 {{ enabledEntries.length }} 个端点正常</div>
    </div>

    <div class="card-footer">{{ origin }}</div>
  </div>
</template>

<script setup lang="ts">
// StatusCard: the vertical share-image template for the Dashboard status
// board (ticket 49, ui-guidelines §5). A designed brand artifact, not a page
// screenshot. Purely presentational: it renders the snapshot it is given and
// never fetches. The conclusion covers the FILTERED set passed in, and the
// scope chips make that range explicit — never present a filtered subset as
// the global picture (mirror of ADR 0007 anti-fake semantics).
// Static medium rules: no animations (the failing blink freezes into a solid
// dot + text chip), no hover (truncation thresholds stay conservative).
import { computed } from 'vue'
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'
import { formatTime } from '@/utils/format'
import { STATUS_LABELS, countByStatus, toneOf, conclusionText, type HealthTone } from '@/utils/healthConclusion'

const props = defineProps<{
  entries: OverviewEntry[] // filtered snapshot, disabled endpoints included
  keyword: string
  protocol: Protocol | ''
  status: EndpointStatus | ''
  generatedAt: string // ISO timestamp of the snapshot moment
  origin: string
}>()

// Cap the abnormal list so a widespread outage cannot produce an unbounded
// tall image; the footer origin is the escape hatch to the live board.
const MAX_DETAIL_ROWS = 10

interface ScopeChip {
  label: string
  value: string
  tone?: EndpointStatus
}

// Conclusion math counts enabled endpoints only (same rule as HealthBanner);
// disabled ones surface as a trailing note, never inside the conclusion.
const enabledEntries = computed(() => props.entries.filter(e => e.enabled))
const disabledCount = computed(() => props.entries.length - enabledEntries.value.length)
const isEmpty = computed(() => enabledEntries.value.length === 0)

const counts = computed(() => countByStatus(enabledEntries.value))
const tone = computed<HealthTone>(() => toneOf(counts.value))
const conclusionTone = computed<HealthTone | 'empty'>(() => (isEmpty.value ? 'empty' : tone.value))
const conclusion = computed(() => conclusionText(tone.value, counts.value, isEmpty.value))
const hasFailing = computed(() => !isEmpty.value && counts.value.failing > 0)

const SEVERITY_ORDER: Record<EndpointStatus, number> = { failing: 0, down: 1, degraded: 2, healthy: 3 }

const abnormalEntries = computed(() =>
  enabledEntries.value
    .filter(e => e.status !== 'healthy')
    .sort(
      (a, b) =>
        SEVERITY_ORDER[a.status] - SEVERITY_ORDER[b.status] || a.model_id.localeCompare(b.model_id),
    ),
)
const topAbnormal = computed(() => abnormalEntries.value.slice(0, MAX_DETAIL_ROWS))
const overflowCount = computed(() => abnormalEntries.value.length - topAbnormal.value.length)

const scopeChips = computed<ScopeChip[]>(() => {
  const chips: ScopeChip[] = []
  if (props.keyword) chips.push({ label: '模型', value: props.keyword })
  if (props.protocol) chips.push({ label: '协议', value: props.protocol })
  if (props.status) chips.push({ label: '状态', value: STATUS_LABELS[props.status], tone: props.status })
  return chips
})

// "YYYY-MM-DD HH:mm" out of the shared formatTime helper.
const timeText = computed(() => {
  const full = formatTime(props.generatedAt)
  return full.length >= 16 ? full.slice(0, 16) : full
})

const emptyDetailText = computed(() =>
  props.entries.length === 0 ? '暂无匹配的 Endpoint' : `${props.entries.length} 个端点均已停用`,
)
</script>

<style scoped>
.status-card {
  width: 720px;
  border-radius: var(--hs-radius);
  border: 1px solid var(--hs-border);
  background: var(--hs-bg-card);
  box-shadow: var(--hs-shadow-card);
  overflow: hidden;
  text-align: left;
}
.brand-bar {
  height: 4px;
  background: var(--hs-brand);
}
.brand-section {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 40px;
  background: var(--hs-brand-soft);
}
.brand-logo {
  width: 32px;
  height: 32px;
}
.brand-title {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.card-body {
  padding: 24px 40px 0;
}
.scope-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.scope-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  padding: 2px 8px;
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-card);
  font-size: var(--hs-text-sm);
}
.chip-label {
  color: var(--hs-text-secondary);
}
.chip-value {
  color: var(--hs-text-primary);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.value-healthy {
  color: var(--el-color-success);
}
.value-degraded {
  color: var(--el-color-warning);
}
.value-down {
  color: var(--el-color-danger);
}
.value-failing {
  color: var(--hs-status-failing);
}
.scope-plain {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-bottom: 16px;
}
.conclusion-block {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 20px;
  border-radius: var(--hs-radius);
  margin-bottom: 8px;
}
.tone-healthy {
  background: var(--el-color-success-light-9);
}
.tone-healthy .conclusion-text {
  color: var(--el-color-success);
}
.tone-degraded {
  background: var(--el-color-warning-light-9);
}
.tone-degraded .conclusion-text {
  color: var(--el-color-warning);
}
.tone-abnormal {
  background: var(--el-color-danger-light-9);
}
.tone-abnormal .conclusion-text {
  color: var(--el-color-danger);
}
/* Empty (zero matches / all disabled) stays neutral: no status tint, so
   "no data" can never read as "全部正常" (same rule as the banner). */
.tone-empty {
  background: var(--hs-bg-page);
  border: 1px solid var(--hs-border);
}
.tone-empty .conclusion-text {
  color: var(--hs-text-secondary);
}
.conclusion-text {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  line-height: 1.5;
}
/* Static equivalent of the failing blink: solid orange-red dot + text chip
   (ui-guidelines §3 static-media rule). */
.alert-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-status-failing);
}
.failing-chip {
  font-size: var(--hs-text-sm);
  color: var(--hs-status-failing);
  border: 1px solid var(--hs-status-failing);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-card);
  padding: 0 6px;
}
.time-row {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-bottom: 24px;
}
.disabled-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  margin-left: 8px;
}
.divider {
  border-top: 1px solid var(--hs-border);
}
.detail-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-secondary);
  margin: 24px 0 8px;
}
.detail-row {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 28px;
}
.row-status {
  flex: none;
  width: 28px;
  font-size: var(--hs-text-sm);
  font-weight: 600;
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
.detail-more {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-top: 8px;
}
.detail-healthy {
  font-size: var(--hs-text-md);
  color: var(--el-color-success);
  margin-top: 24px;
}
.detail-empty {
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
  margin-top: 24px;
}
.card-footer {
  margin: 24px 40px 0;
  padding: 16px 0 24px;
  border-top: 1px solid var(--hs-border);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
</style>
