<template>
  <div class="status-card">
    <div class="brand-bar" />
    <div class="brand-section">
      <BrandMark class="brand-mark" />
      <span class="brand-title">HubScope 服务状态</span>
    </div>

    <div class="card-body">
      <!-- Scope: the anti-fake line. A group share always leads with the
           group chip ("分组:厂商 · Anthropic"); every active filter shows up
           as a chip (none omitted); neither → the plain "全部端点" line. -->
      <div v-if="scopeChips.length > 0" class="scope-row">
        <span v-for="chip in scopeChips" :key="chip.label" class="scope-chip">
          <span class="chip-label">{{ chip.label }}</span>
          <span class="chip-value" :class="chip.tone ? `value-${chip.tone}` : ''" :title="chip.value">
            {{ chip.value }}
          </span>
        </span>
      </div>
      <div v-else class="scope-plain">全部端点</div>

      <!-- Hero panel: availability leads, verdict + distribution ride
           underneath (see StatusCardMetrics). The top does not foreground
           the abnormal endpoints, but the verdict and counts stay honest. -->
      <StatusCardMetrics :entries="enabledEntries" :is-empty="isEmpty" />

      <div class="divider" />

      <StatusCardDetail :entries="enabledEntries" :empty-text="emptyDetailText" :summary="summary" />
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
import type { EndpointStatus, OverviewEntry, Protocol } from '@/api/types'
import type { GroupDimension } from '@/utils/statusCardSnapshot'
import { formatTime } from '@/utils/format'
import { STATUS_LABELS, countByStatus } from '@/utils/healthConclusion'
import { summaryText } from '@/utils/statusCardSummary'
import StatusCardMetrics from '@/components/StatusCardMetrics.vue'
import StatusCardDetail from '@/components/StatusCardDetail.vue'
import BrandMark from '@/components/BrandMark.vue'

const props = defineProps<{
  entries: OverviewEntry[] // scoped snapshot, disabled endpoints included
  keyword: string
  protocol: Protocol | ''
  status: EndpointStatus | ''
  group: { dimension: GroupDimension; key: string } | null
  generatedAt: string // ISO timestamp of the snapshot moment
  origin: string
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

const counts = computed(() => countByStatus(enabledEntries.value))
const summary = computed(() => summaryText(counts.value, enabledEntries.value, isEmpty.value))

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

// "YYYY-MM-DD HH:mm" out of the shared formatTime helper.
const timeText = computed(() => {
  const full = formatTime(props.generatedAt)
  return full.length >= 16 ? full.slice(0, 16) : full
})

// Empty-state wording ('' when the scope has enabled entries).
const emptyDetailText = computed(() => {
  if (!isEmpty.value) return ''
  return props.entries.length === 0 ? '暂无匹配的 Endpoint' : `${props.entries.length} 个端点均已停用`
})
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
.brand-mark {
  font-size: 32px;
  flex-shrink: 0;
}
.brand-title {
  font-size: var(--hs-text-2xl);
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
.divider {
  border-top: 1px solid var(--hs-border);
}
.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 16px;
  margin: 24px 40px 0;
  padding: 16px 0 24px;
  border-top: 1px solid var(--hs-border);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.footer-origin {
  flex: none;
}
.disabled-note {
  margin-left: 8px;
}
</style>
