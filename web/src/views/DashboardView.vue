<template>
  <div class="dashboard">
    <!-- Semantic page heading (a11y): screen readers get a real h1;
         visually hidden — zero visual change. -->
    <h1 class="visually-hidden">HubScope 服务状态总览</h1>

    <!-- Hero (GH #115, spec 0018 §6): health index + delta + conclusion +
         scope. The conclusion math comes from the shared healthConclusion
         module (same words as the share material); a null health index is
         folded into the empty branch so no-probe windows never read as
         全部稳定. -->
    <StatusHero
      :health="healthScore24h"
      :delta="healthScoreDelta"
      :conclusion="heroConclusion"
      :conclusion-tone="heroTone"
      :scope="heroScope"
      :skeleton="initialLoading"
    />

    <!-- Widget row (spec 0018 §7): four Apple-Widget-style metric cells. -->
    <MetricWidgets
      :availability="availability24h"
      :delta="healthScoreDelta"
      :probes="probes24h"
      :mean-latency-ms="meanLatency"
      :abnormal="abnormal"
      :availability-series="availabilitySeries"
      :probe-series="probeSeries"
      :latency-series="latencySeries"
      :failure-series="failureSeries"
      :skeleton="initialLoading"
    />

    <!-- Filters: model keyword, protocol, display status, grouping. -->
    <div class="filter-row">
      <el-input
        v-model="keyword"
        placeholder="按模型名过滤"
        clearable
        class="filter-keyword"
      />
      <span class="filter-label">协议：</span>
      <el-select v-model="protocolFilter" placeholder="全部" clearable class="filter-select">
        <!-- Options come from the single protocol vocabulary (utils/protocol.ts)
             so new protocols appear here automatically. -->
        <el-option v-for="p in PROTOCOLS" :key="p" :label="p" :value="p" />
      </el-select>
      <span class="filter-label">状态：</span>
      <el-select v-model="statusFilter" placeholder="全部" clearable class="filter-select">
        <!-- Options come from the single display-layer mapping (GH #113):
             the three display states, light → heavy; down + failing filter
             together under 服务异常. No status word literals here. -->
        <el-option v-for="s in statusOptions" :key="s" :label="statusLabel(s)" :value="s" />
      </el-select>
      <el-select v-model="grouping" class="filter-select">
        <el-option label="按厂商分组" value="family" />
        <el-option label="按能力分组" value="capability" />
        <el-option label="按协议" value="protocol" />
        <el-option label="不分组" value="none" />
      </el-select>
      <!-- Share the filtered picture as a Status Card PNG; disabled until the
           first load lands (an empty board is not shareable). -->
      <el-button
        class="share-btn"
        :disabled="loading && entries.length === 0"
        @click="openShare"
      >
        <el-icon><Share /></el-icon>
        分享状态
      </el-button>
    </div>

    <StatusShareDialog v-model:visible="shareVisible" :snapshot="shareSnapshot" />

    <!-- Side detail panel (GH #116, spec 0018 §10): the row click opens the
         frozen-snapshot sheet instead of deep-linking. No share entry
         inside the panel — the share dialog only ever opens from this
         page's filter row or the full detail page, so a dialog never
         stacks on the sheet. -->
    <ModelDetailPanel :entry="panelEntry" @close="closePanel" />

    <el-alert
      v-if="error"
      :title="`刷新失败：${error}`"
      type="error"
      :closable="false"
      class="error-alert"
    />

    <!-- First-load skeleton rows (subsequent polls keep the list on screen —
         local refresh, never a full re-mount). -->
    <div v-if="initialLoading" class="list-skeleton">
      <div v-for="i in 6" :key="i" class="skel-row" />
    </div>

    <template v-else>
      <ModelStatusList v-if="listSections.some(s => s.entries.length > 0)" :sections="listSections" @open="openDetail" />
      <el-empty
        v-else-if="entries.length > 0"
        description="暂无匹配的 Endpoint"
      />
      <el-empty v-else description="暂无监控端点，请先在模型管理中添加" />
    </template>
  </div>
</template>

<script setup lang="ts">
// DashboardView (rebuilt for UI v2, GH #115, spec 0018 §6/§7/§8): hero +
// metric widgets + model status list. The old world — HealthBanner,
// EndpointCard matrix, OverviewGroupSection, UptimeStrip, quick-view dialog
// — is retired wholesale with this ticket.
import { ref, computed, nextTick, onMounted } from 'vue'
import { Share } from '@element-plus/icons-vue'
import { useOverview } from '@/composables/useOverview'
import StatusHero from '@/components/StatusHero.vue'
import MetricWidgets from '@/components/MetricWidgets.vue'
import ModelStatusList, { type ListSection } from '@/components/ModelStatusList.vue'
import ModelDetailPanel from '@/components/ModelDetailPanel.vue'
import StatusShareDialog from '@/components/StatusShareDialog.vue'
import type { StatusCardSnapshot } from '@/utils/statusCardSnapshot'
import { PROTOCOLS } from '@/utils/protocol'
import { sortEntriesBySeverity, sortGroupSections } from '@/utils/severitySort'
import { DISPLAY_SEVERITY_ORDER, statusLabel, toDisplayStatus, type DisplayStatus } from '@/utils/statusDisplay'
import { countByStatus, toneOf, conclusionText } from '@/utils/healthConclusion'
import { aggregateDots24h } from '@/utils/overviewDots'
import {
  heroScopeText,
  hourlyAvailabilitySeries,
  hourlyProbeSeries,
  hourlyLatencySeries,
  hourlyFailureSeries,
  abnormalModelCounts,
} from '@/utils/overviewMetrics'
import { meanP50Ms } from '@/utils/statusCardSummary'
import type { Protocol, OverviewEntry } from '@/api/types'

const {
  entries,
  byFamily,
  byCapability,
  byProtocol,
  loading,
  error,
  enabledEndpoints,
  availability24h,
  healthScore24h,
  healthScoreDelta,
  probes24h,
  start,
} = useOverview()

const keyword = ref('')
const protocolFilter = ref<Protocol | ''>('')
// The status filter speaks DISPLAY states (GH #113): the domain status
// machine keeps four states, but the board renders three — down and
// failing filter together under 'incident' (服务异常).
const statusFilter = ref<DisplayStatus | ''>('')
// Filter-select options: the three display states, light → heavy (the
// severity order reversed), words from the single mapping.
const statusOptions = [...DISPLAY_SEVERITY_ORDER].reverse()
// Grouping dimension of the list; vendor family by default.
const grouping = ref<'family' | 'capability' | 'protocol' | 'none'>('family')

// First load only: hero/widgets/list render skeletons until the first
// response lands. Poll failures keep the last good data (error alert on
// top), so skeleton never replaces a populated board.
const initialLoading = computed(() => loading.value && entries.value.length === 0 && error.value === null)

// --- Hero -------------------------------------------------------------------

const enabledEntries = computed(() => entries.value.filter(e => e.enabled))

const heroConclusion = computed(() => {
  const counts = countByStatus(enabledEntries.value)
  // A null health index (no probes in the window) folds into the empty
  // branch: no data must never read as 全部稳定 (anti-fake).
  const empty = enabledEntries.value.length === 0 || healthScore24h.value === null
  return conclusionText(toneOf(counts), counts, empty)
})

const heroTone = computed<'success' | 'warning' | 'danger' | 'neutral'>(() => {
  if (enabledEntries.value.length === 0 || healthScore24h.value === null) return 'neutral'
  const tone = toneOf(countByStatus(enabledEntries.value))
  return tone === 'abnormal' ? 'danger' : tone === 'degraded' ? 'warning' : 'success'
})

const heroScope = computed(() => heroScopeText(enabledEndpoints.value))

// --- Widgets ------------------------------------------------------------------

// Aggregated dots of the enabled set feed the availability/request/failure
// sparklines (probe-weighted, overviewDots discipline); the latency
// sparkline derives from the same entries (success-weighted hourly means).
const aggregateDots = computed(() => aggregateDots24h(enabledEntries.value))
const availabilitySeries = computed(() => hourlyAvailabilitySeries(aggregateDots.value))
const probeSeries = computed(() => hourlyProbeSeries(aggregateDots.value))
const failureSeries = computed(() => hourlyFailureSeries(aggregateDots.value))
const latencySeries = computed(() => hourlyLatencySeries(enabledEntries.value))
// The registered scope-consistent latency mean (batch-59 caliber).
const meanLatency = computed(() => meanP50Ms(enabledEntries.value))
const abnormal = computed(() => abnormalModelCounts(entries.value))

// --- Share --------------------------------------------------------------------

// Share dialog state. The snapshot freezes the filtered set and filter
// conditions at open time so polling cannot change the card mid-preview.
const shareVisible = ref(false)
const shareSnapshot = ref<StatusCardSnapshot | null>(null)

function openShare() {
  shareSnapshot.value = {
    entries: [...filteredEntries.value],
    keyword: keyword.value.trim(),
    protocol: protocolFilter.value,
    status: statusFilter.value,
    group: null,
    generatedAt: new Date().toISOString(),
  }
  shareVisible.value = true
}

// --- List ---------------------------------------------------------------------

// Apply the three filters; an empty filter matches everything. The status
// filter matches by DISPLAY state (toDisplayStatus), so 'incident' catches
// down and failing entries together. The result is severity-ranked
// (GH #52) so abnormal models lead the first viewport.
const filteredEntries = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return sortEntriesBySeverity(
    entries.value.filter(entry => {
      if (kw && !entry.model_id.toLowerCase().includes(kw)) return false
      if (protocolFilter.value && entry.protocol !== protocolFilter.value) return false
      if (statusFilter.value && toDisplayStatus(entry.status) !== statusFilter.value) return false
      return true
    }),
  )
})

// Grouped mode: one light list section per group, severity-ranked by the
// group's most severe ENABLED entry (the board's single rank table). The
// meta line counts by DISPLAY state (down + failing read together as
// 服务异常) with words from the single mapping — never a literal.
const listSections = computed<ListSection[]>(() => {
  if (grouping.value === 'none') {
    return [{ key: null, label: '', meta: '', entries: filteredEntries.value }]
  }
  const groups =
    grouping.value === 'family'
      ? byFamily.value
      : grouping.value === 'capability'
        ? byCapability.value
        : byProtocol.value
  const keyOf = (e: OverviewEntry) =>
    grouping.value === 'family' ? e.family : grouping.value === 'capability' ? e.capability : e.protocol
  return sortGroupSections(
    groups.map(group => ({
      group,
      entries: filteredEntries.value.filter(e => keyOf(e) === group.key),
    })),
  ).map(section => {
    const counts = abnormalModelCounts(section.entries)
    const meta =
      counts.total === 0
        ? `${section.entries.length} 个端点`
        : `${section.entries.length} 个端点 · ${statusLabel('incident')} ${counts.incident} · ${statusLabel('degraded')} ${counts.degraded}`
    return { key: section.group.key, label: section.group.key, meta, entries: section.entries }
  })
})

// --- Detail panel (GH #116) -------------------------------------------------

// The panel's frozen snapshot: a spread copy taken at click time. The
// overview poll replaces the entries array wholesale, so the clicked
// object is naturally frozen — the copy documents the freeze intent and
// guards against any future in-place mutation of entries.
const panelEntry = ref<OverviewEntry | null>(null)

// Row click opens the side detail panel (T6 replaces the T5 interim
// deep-link). The full detail page stays one click away inside the panel.
function openDetail(entry: OverviewEntry) {
  panelEntry.value = { ...entry }
}

// Unified close path (ESC / scrim / close button all emit the same close).
// Focus returns to the trigger row; after the panel's own deep-link the
// row is already unmounted and the querySelector no-ops by construction.
function closePanel() {
  const id = panelEntry.value?.endpoint_id
  panelEntry.value = null
  if (id !== undefined) {
    nextTick(() => {
      document.querySelector<HTMLElement>(`[data-endpoint-id="${id}"]`)?.focus()
    })
  }
}

onMounted(start)
</script>

<style scoped>
/* Visually hidden but present for assistive tech (standard sr-only pattern;
   never display:none — that would drop it from the a11y tree). */
.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  white-space: nowrap;
  border: 0;
}
.dashboard {
  max-width: 1200px;
  margin: 0 auto;
  /* Public-page breathing room (spec 0018 IA: 32–48px whitespace). */
  padding: var(--hs-space-6) var(--hs-space-6) var(--hs-space-8);
}
.filter-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  margin-bottom: var(--hs-space-4);
  flex-wrap: wrap;
}
.filter-keyword {
  width: 220px;
}
.filter-label {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.filter-select {
  width: 140px;
}
.share-btn {
  margin-left: auto;
}
.error-alert {
  margin-bottom: var(--hs-space-4);
}
.list-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.skel-row {
  height: 46px;
  border-radius: var(--hs-radius-lg);
  background: var(--hs-bg-hover);
}
</style>
