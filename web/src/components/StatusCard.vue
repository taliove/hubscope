<template>
  <div class="status-card" :class="{ 'compact': compact, 'endpoint-small-card': isEndpointSmallCard }">
    <div class="brand-bar" />
    <div v-if="isEndpointSmallCard" class="brand-slim">
      <BrandMark class="brand-mark" />
      <Wordmark class="brand-wordmark" />
    </div>
    <div v-else class="brand-section">
      <BrandMark class="brand-mark" />
      <Wordmark class="brand-wordmark" />
      <span v-if="!compact" class="brand-title">服务状态</span>
      <span v-else class="brand-title">服务状态</span>
    </div>

    <div class="card-body">
      <!-- Scope: the anti-fake line. Single-model mode states the exact
           subject (model · protocol · Hub chips); a group share leads with
           the group chip; every active filter shows up as a chip (none
           omitted); neither → the plain "全部端点" line. Endpoint small card
           (single-model compact) omits scope chips: Hub is internal topology,
           complete range declaration stays in the full card. -->
      <div v-if="isSingleModel && !isEndpointSmallCard" class="scope-row">
        <span class="scope-chip">
          <span class="chip-label">模型</span>
          <span class="chip-value" :title="singleEntry.model_id">{{ singleEntry.model_id }}</span>
        </span>
        <span class="scope-chip">
          <span class="chip-label">协议</span>
          <span class="chip-value" :title="singleEntry.protocol">{{ singleEntry.protocol }}</span>
        </span>
        <span class="scope-chip">
          <span class="chip-label">Hub</span>
          <span class="chip-value" :title="hubName">{{ hubName }}</span>
        </span>
      </div>
      <div v-else-if="scopeChips.length > 0" class="scope-row">
        <span v-for="chip in scopeChips" :key="chip.label" class="scope-chip">
          <span class="chip-label">{{ chip.label }}</span>
          <span class="chip-value" :class="chip.tone ? `value-${chip.tone}` : ''" :title="chip.value">
            {{ chip.value }}
          </span>
        </span>
      </div>
      <div v-else class="scope-plain">全部端点</div>

      <!-- Hero panel: single-model mode mounts the reworked single-model
           panel (statement instead of verdict + distribution); the aggregate
           panel is untouched for global/group shares. Endpoint small card
           (single-model compact) uses inline indicators instead of the hero
           panel. -->
      <template v-if="isEndpointSmallCard">
        <!-- Model row: model name + protocol tag -->
        <div class="model-row">
          <span class="model-name" :title="singleEntry.model_id">{{ singleEntry.model_id }}</span>
          <el-tag size="small" :type="protocolTagType(singleEntry.protocol)">
            {{ singleEntry.protocol }}
          </el-tag>
        </div>

        <!-- Statement row: status statement with failing double-encoding -->
        <div class="statement-row">
          <span v-if="smallCardStatement.failingChip" class="alert-dot-small" />
          <span class="statement-text-small" :class="`vc-${smallCardStatement.tone}`">
            {{ smallCardStatement.text }}
          </span>
          <span v-if="smallCardStatement.failingChip" class="failing-chip-small">
            {{ smallCardStatement.failingChip }}
          </span>
        </div>

        <!-- Indicators row: availability + latency -->
        <div class="indicators-row">
          <div class="indicator-col">
            <span class="indicator-label">24h 可用率</span>
            <span class="indicator-big" :class="`av-${availabilityTier(smallCardAvailability)}`">
              <template v-if="smallCardAvailability !== null">
                {{ formatPercentDigits(smallCardAvailability) }}<span class="indicator-unit">%</span>
              </template>
              <template v-else>-</template>
            </span>
          </div>
          <div class="indicator-col">
            <span class="indicator-label">平均延迟</span>
            <span class="indicator-latency" :class="{ 'av-none': singleEntry.p50_ms === null }">
              {{ formatMs(singleEntry.p50_ms) }}
            </span>
          </div>
        </div>

        <!-- Mini 24h dots: 8px height with axis labels -->
        <div class="mini-uptime-section">
          <div class="mini-uptime-strip">
            <span v-for="(dot, i) in singleEntry.dots_24h" :key="i" class="mini-uptime-slot">
              <span class="mini-uptime-seg" :class="`seg-${dotTier(dot.total, dot.failures)}`" />
            </span>
          </div>
          <div class="mini-uptime-axis">
            <span>24 小时前</span>
            <span>现在</span>
          </div>
        </div>
      </template>
      <StatusCardSingleModelMetrics
        v-else-if="isSingleModel"
        :entry="singleEntry"
        :eval-summary="evalSummary ?? null"
        :compact="compact"
      />
      <StatusCardMetrics v-else :entries="enabledEntries" :is-empty="isEmpty" :compact="compact" />

      <div v-if="!isEndpointSmallCard" class="divider" />

      <StatusCardDetail
        v-if="!isEndpointSmallCard"
        :entries="enabledEntries"
        :empty-text="emptyDetailText"
        :summary="summary"
        :single-model="isSingleModel"
      />
    </div>

    <div class="card-footer">
      <span>
        生成于 {{ timeText }}
        <span v-if="disabledCount > 0" class="disabled-note">另有 {{ disabledCount }} 个已停用</span>
      </span>
      <span class="footer-origin">{{ origin }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
// StatusCard: the vertical share-image template for the Dashboard status
// board (ticket 56, redesigned in ticket 59 — ui-guidelines §5). A designed
// brand artifact, not a page screenshot. Purely presentational: it renders
// the snapshot it is given and never fetches. Every number is computed from
// the same enabled-entry set the scope chips describe (see
// statusCardSummary.ts for why backend aggregates are not passed through) —
// never present a filtered subset as the global picture (mirror of ADR 0007
// anti-fake semantics). The hero panel and detail blocks live in child
// components; this file owns the brand, scope and footer.
// Static medium rules: no animations (the failing blink freezes into a solid
// dot + text chip), no hover reliance (truncation thresholds stay
// conservative).
import { computed } from 'vue'
import type { EndpointStatus, ModelEvalSummary, OverviewEntry, Protocol } from '@/api/types'
import type { GroupDimension } from '@/utils/statusCardSnapshot'
import { formatTimeMinute, formatPercentDigits, formatMs } from '@/utils/format'
import { STATUS_LABELS, countByStatus } from '@/utils/healthConclusion'
import {
  scopedAvailability,
  singleModelSummaryText,
  summaryText,
  singleModelStatement,
  availabilityTier,
  dotTier,
} from '@/utils/statusCardSummary'
import { protocolTagType } from '@/utils/protocol'
import StatusCardMetrics from '@/components/StatusCardMetrics.vue'
import StatusCardSingleModelMetrics from '@/components/StatusCardSingleModelMetrics.vue'
import StatusCardDetail from '@/components/StatusCardDetail.vue'
import BrandMark from '@/components/BrandMark.vue'
import Wordmark from '@/components/Wordmark.vue'

const props = defineProps<{
  entries: OverviewEntry[] // scoped snapshot, disabled endpoints included
  keyword: string
  protocol: Protocol | ''
  status: EndpointStatus | ''
  group: { dimension: GroupDimension; key: string } | null
  generatedAt: string // ISO timestamp of the snapshot moment
  origin: string
  // Single-model mode markers (design ruling, ticket 60.5 wiring): hubName
  // is the flag — a filtered snapshot that happens to hold one entry has no
  // hubName and stays on the aggregate layout.
  hubName?: string
  evalSummary?: ModelEvalSummary | null
  // Compact variant (GH #93): 480px width adaptation. Endpoint small card =
  // single-model + compact (six-section compact structure).
  compact?: boolean
}>()

const DIMENSION_LABELS: Record<GroupDimension, string> = {
  family: '厂商',
  capability: '能力',
  protocol: '协议',
}

interface ScopeChip {
  label: string
  value: string
  tone?: EndpointStatus
}

// Counts feed the summary only; the hero panel computes its own availability
// and verdict from the same enabled set (cheap, and keeps the parent's
// responsibilities narrow). Disabled endpoints surface as a trailing note,
// never inside the conclusion.
const enabledEntries = computed(() => props.entries.filter(e => e.enabled))
const disabledCount = computed(() => props.entries.length - enabledEntries.value.length)
const isEmpty = computed(() => enabledEntries.value.length === 0)

// Single-model mode (design ruling): exactly one entry AND a non-empty
// hubName — the hub name is the marker that the snapshot was built for one
// model, so a filter that narrows the board to a single endpoint (or an
// empty hubName) never flips the layout.
const isSingleModel = computed(() => props.entries.length === 1 && Boolean(props.hubName))
const singleEntry = computed(() => props.entries[0])

// Endpoint small card (GH #93): single-model + compact. Six-section compact
// structure for mobile sharing.
const isEndpointSmallCard = computed(() => isSingleModel.value && props.compact)

// Small card computeds (only used when isEndpointSmallCard is true)
const smallCardAvailability = computed(() => scopedAvailability([singleEntry.value]))
const smallCardStatement = computed(() => singleModelStatement(singleEntry.value, smallCardAvailability.value))

const counts = computed(() => countByStatus(enabledEntries.value))
const summary = computed(() =>
  isSingleModel.value
    ? singleModelSummaryText(singleEntry.value, scopedAvailability([singleEntry.value]))
    : summaryText(counts.value, enabledEntries.value, isEmpty.value),
)

const scopeChips = computed<ScopeChip[]>(() => {
  const chips: ScopeChip[] = []
  if (props.group) {
    chips.push({ label: '分组', value: `${DIMENSION_LABELS[props.group.dimension]} · ${props.group.key}` })
  }
  if (props.keyword) chips.push({ label: '模型', value: props.keyword })
  if (props.protocol) chips.push({ label: '协议', value: props.protocol })
  if (props.status) chips.push({ label: '状态', value: STATUS_LABELS[props.status], tone: props.status })
  return chips
})

// "YYYY-MM-DD HH:mm" via the shared minute-precision helper (GH #57).
const timeText = computed(() => formatTimeMinute(props.generatedAt))

// Empty-state wording ('' when the scope has enabled entries).
const emptyDetailText = computed(() => {
  if (!isEmpty.value) return ''
  return props.entries.length === 0 ? '暂无匹配的 Endpoint' : `${props.entries.length} 个端点均已停用`
})
</script>

<style scoped>
.status-card {
  width: 720px;
  border-radius: var(--hs-radius-lg);
  /* Static card: layering comes from the 1px border, not shadow
     (ui-guidelines §2 shadow semantics — shadows only for clickable/overlay). */
  border: 1px solid var(--hs-border);
  background: var(--hs-bg-card);
  overflow: hidden;
  text-align: left;
}
/* Compact variant: 480px width (GH #93). Content-box (no global reset):
   width 480 = content, outer box = 482 (1px border × 2). */
.status-card.compact {
  width: 480px;
}
.brand-bar {
  height: 4px;
  background: var(--hs-brand);
}
/* Canvas horizontal margins (40px on the 720 card, 20px on the 480 compact
   and the small card) are material design constants, not grid spacing — they
   intentionally stay px literals and cross-reference the share-materials
   brief cascade table (GH #95 spacing tokenization: everything else that
   lands on the 4px grid consumes --hs-space-*). */
.brand-section {
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  padding: var(--hs-space-4) 40px;
  background: var(--hs-brand-soft);
}
.compact .brand-section {
  padding: var(--hs-space-3) 20px;
}
.brand-mark {
  font-size: 32px;
  flex-shrink: 0;
}
.compact .brand-mark {
  font-size: 24px;
}
/* Endpoint small card: slim brand row (no soft background, smaller sizing) */
.brand-slim {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  padding: var(--hs-space-2) 20px;
}
.brand-slim .brand-mark {
  font-size: 16px;
}
.brand-slim .brand-wordmark {
  font-size: var(--hs-text-sm);
}
.brand-wordmark {
  font-size: var(--hs-text-xl);
  flex-shrink: 0;
}
.compact .brand-wordmark {
  font-size: var(--hs-text-lg);
}
.brand-title {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.compact .brand-title {
  font-size: var(--hs-text-xl);
}
.card-body {
  padding: var(--hs-space-5) 40px 0;
}
.compact .card-body {
  padding: var(--hs-space-4) 20px 0;
}
.scope-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--hs-space-2);
  margin-bottom: var(--hs-space-4);
}
.scope-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--hs-space-1);
  max-width: 100%;
  /* Vertical 2px stays off-grid by design (brief item 3/7: chips-row spec,
     kept in lockstep with EvalCard scope chips). */
  padding: 2px var(--hs-space-2);
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
.compact .chip-value {
  max-width: 160px;
}
/* GH #69 text/graphics split: chip state values are text — success as text
   consumes the deepened text grade, never the base green. */
.value-healthy {
  color: var(--hs-success-text);
}
.value-degraded {
  color: var(--hs-warning);
}
.value-down {
  color: var(--hs-danger);
}
.value-failing {
  color: var(--hs-status-failing);
}
.scope-plain {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-bottom: var(--hs-space-4);
}
.divider {
  border-top: 1px solid var(--hs-border);
}
.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--hs-space-4);
  margin: var(--hs-space-5) 40px 0;
  padding: var(--hs-space-4) 0 var(--hs-space-5);
  border-top: 1px solid var(--hs-border);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.compact .card-footer {
  margin: var(--hs-space-4) 20px 0;
  padding: var(--hs-space-3) 0 var(--hs-space-4);
}
.footer-origin {
  flex: none;
}
.disabled-note {
  margin-left: var(--hs-space-2);
}

/* Endpoint small card sections (GH #93): six-section compact structure for
   single-model + compact. */
.model-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  margin-bottom: var(--hs-space-3);
}
.model-name {
  flex: 1;
  min-width: 0;
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.statement-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  margin-bottom: var(--hs-space-3);
}
.statement-text-small {
  font-size: var(--hs-text-sm);
  font-weight: 600;
}
.alert-dot-small {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-status-failing);
}
.failing-chip-small {
  font-size: var(--hs-text-xs);
  color: var(--hs-status-failing);
  border: 1px solid var(--hs-status-failing);
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-card);
  padding: 0 var(--hs-space-2);
}
.indicators-row {
  display: flex;
  gap: var(--hs-space-4);
  margin-bottom: var(--hs-space-4);
}
.indicator-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.indicator-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  margin-bottom: var(--hs-space-1);
}
.indicator-big {
  font-size: var(--hs-text-display);
  font-weight: 600;
  line-height: 1.2;
}
.indicator-unit {
  font-size: var(--hs-text-md);
  font-weight: 400;
  color: var(--hs-text-secondary);
  margin-left: var(--hs-space-1);
}
.indicator-latency {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
}
.mini-uptime-section {
  margin-bottom: var(--hs-space-4);
}
.mini-uptime-strip {
  display: flex;
  gap: 2px;
}
.mini-uptime-slot {
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
}
.mini-uptime-seg {
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
.mini-uptime-axis {
  display: flex;
  justify-content: space-between;
  margin-top: var(--hs-space-1);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
/* Statement-row tone colors (check GH #93 CRITICAL-1): scoped styles from
   StatusCardSingleModelMetrics do not reach this template, so the small
   card defines its own set. Text-scenario success consumes the deepened
   text grade, not the base green (GH #69 text/graphics split). */
.vc-healthy {
  color: var(--hs-success-text);
}
.vc-degraded {
  color: var(--hs-warning);
}
.vc-abnormal {
  color: var(--hs-danger);
}
</style>
