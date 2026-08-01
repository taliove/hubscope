<template>
  <div class="dashboard">
    <!-- Visible page header (2026-08-01, reference-design replica): the h1
         follows the 「页面 h1 = 侧边栏标签」 convention — the old sr-only
         exception retires with this ticket; the visible heading carries the
         a11y tree, no duplicate. -->
    <header class="page-header">
      <h1 class="page-title">状态概览</h1>
      <p class="page-lede">全局视角，掌握 AI 服务运行健康状态</p>
    </header>

    <!-- Hero (GH #115, spec 0018 §6; GH #129: reference-design composition —
         label above the figure, soft conclusion chip, arrowed delta, refresh
         meta top-right, 24h trend chart right). The conclusion math comes
         from the shared healthConclusion module (same words as the share
         material); a null health index is folded into the empty branch so
         no-probe windows never read as 全部稳定运行. -->
    <StatusHero
      :health="healthScore24h"
      :delta="healthScoreDelta"
      :conclusion="heroConclusion"
      :conclusion-tone="heroTone"
      :scope="heroScope"
      :trend-categories="heroTrend.categories"
      :trend-values="heroTrend.values"
      :updated-at="generatedAt"
      :refresh-interval-ms="refreshIntervalMs"
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

    <!-- List toolbar (GH #131, reference design; GH #136 sort rework): the
         section title carries the sort note — DYNAMIC since GH #136, it
         restates the current column ordering so the label stays literally
         true after the user re-sorts (a label the data does not honor is
         an anti-fake violation). The filters right-align on the same row:
         keyword + vendor + display status. -->
    <div class="list-toolbar">
      <h2 class="list-heading">
        模型状态
        <span class="list-note">{{ listSortNoteText }}</span>
      </h2>
      <div class="filter-row">
        <el-input
          v-model="keyword"
          placeholder="按模型名过滤"
          clearable
          class="filter-keyword"
        />
        <span class="filter-label">供应商：</span>
        <el-select v-model="familyFilter" placeholder="全部" clearable class="filter-select">
          <!-- Options derive from the unfiltered entry set (familyOptions) so
               an active filter never collapses its own option list. -->
          <el-option v-for="f in vendorOptions" :key="f" :label="f" :value="f" />
        </el-select>
        <span class="filter-label">状态：</span>
        <el-select v-model="statusFilter" placeholder="全部" clearable class="filter-select">
          <!-- Options come from the single display-layer mapping (GH #113):
               the three display states, light → heavy; down + failing filter
               together under 异常. No status word literals here. -->
          <el-option v-for="s in statusOptions" :key="s" :label="statusLabel(s)" :value="s" />
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
      <ModelStatusList
        v-if="listSections.some(s => s.entries.length > 0)"
        :sections="listSections"
        :sort="listSort"
        @open="openDetail"
        @sort="onListSort"
      />
      <el-empty
        v-else-if="entries.length > 0"
        description="暂无匹配的 Endpoint"
      />
      <el-empty v-else description="暂无监控端点，请先在模型管理中添加" />
    </template>

    <!-- Recent events (GH #132, UI v2 O5): LOGIN-ONLY section — the
         component gates both rendering and fetching on `authed`, so an
         anonymous board issues zero alerts requests. It refetches on the
         overview poll tick (generatedAt) instead of owning a timer. -->
    <RecentEvents :authed="authed" :entries="entries" :refresh-tick="generatedAt" />
  </div>
</template>

<script setup lang="ts">
// DashboardView (rebuilt for UI v2, GH #115, spec 0018 §6/§7/§8): hero +
// metric widgets + model status list. The old world — HealthBanner,
// EndpointCard matrix, OverviewGroupSection, UptimeStrip, quick-view dialog
// — is retired wholesale with this ticket.
import { ref, computed, nextTick, onMounted } from 'vue'
import { Share } from '@element-plus/icons-vue'
import { useOverview, POLL_INTERVAL_MS } from '@/composables/useOverview'
import { fetchAuthStatus } from '@/api/auth'
import StatusHero from '@/components/StatusHero.vue'
import MetricWidgets from '@/components/MetricWidgets.vue'
import ModelStatusList, { type ListSection } from '@/components/ModelStatusList.vue'
import ModelDetailPanel from '@/components/ModelDetailPanel.vue'
import StatusShareDialog from '@/components/StatusShareDialog.vue'
import RecentEvents from '@/components/RecentEvents.vue'
import type { StatusCardSnapshot } from '@/utils/statusCardSnapshot'
import { DISPLAY_SEVERITY_ORDER, statusLabel, toDisplayStatus, type DisplayStatus } from '@/utils/statusDisplay'
import {
  familyOptions,
  listSortNote,
  loadListSort,
  nextListSort,
  saveListSort,
  sortListEntries,
  type ListSort,
  type ListSortKey,
} from '@/utils/modelList'
import { countByStatus, toneOf, conclusionText } from '@/utils/healthConclusion'
import { aggregateDots24h } from '@/utils/overviewDots'
import {
  heroScopeText,
  hourlyAvailabilitySeries,
  hourlyProbeSeries,
  hourlyLatencySeries,
  hourlyFailureSeries,
  heroTrendSeries,
  abnormalModelCounts,
} from '@/utils/overviewMetrics'
import { meanP50Ms } from '@/utils/statusCardSummary'
import type { OverviewEntry } from '@/api/types'

const {
  entries,
  generatedAt,
  loading,
  error,
  enabledEndpoints,
  availability24h,
  healthScore24h,
  healthScoreDelta,
  probes24h,
  start,
} = useOverview()

// The hero's refresh meta reads the poll cadence from the useOverview
// constant (GH #129) — a single declaration, never a second literal.
const refreshIntervalMs = POLL_INTERVAL_MS

const keyword = ref('')
// Vendor (供应商) filter of the reference toolbar (GH #131): matches the
// entry's family classification; the options derive from the unfiltered set.
const familyFilter = ref('')
// The status filter speaks DISPLAY states (GH #113): the domain status
// machine keeps four states, but the board renders three — down and
// failing filter together under 'incident' (异常).
const statusFilter = ref<DisplayStatus | ''>('')
// Filter-select options: the three display states, light → heavy (the
// severity order reversed), words from the single mapping.
const statusOptions = [...DISPLAY_SEVERITY_ORDER].reverse()

// First load only: hero/widgets/list render skeletons until the first
// response lands. Poll failures keep the last good data (error alert on
// top), so skeleton never replaces a populated board.
const initialLoading = computed(() => loading.value && entries.value.length === 0 && error.value === null)

// --- Hero -------------------------------------------------------------------

const enabledEntries = computed(() => entries.value.filter(e => e.enabled))

const heroConclusion = computed(() => {
  const counts = countByStatus(enabledEntries.value)
  // A null health index (no probes in the window) folds into the empty
  // branch: no data must never read as 全部稳定运行 (anti-fake).
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
// Hero 24h trend chart (GH #129): the same probe-weighted hourly
// availability of the enabled set, derived by the single pure function
// (labels + 0–100 display-scale values, nulls preserved as line breaks).
const heroTrend = computed(() => heroTrendSeries(aggregateDots.value))
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
    protocol: '',
    family: familyFilter.value || undefined,
    status: statusFilter.value,
    group: null,
    generatedAt: new Date().toISOString(),
  }
  shareVisible.value = true
}

// --- List ---------------------------------------------------------------------

// Vendor-filter options come from the UNFILTERED set (familyOptions) so an
// active vendor filter never collapses its own option list.
const vendorOptions = computed(() => familyOptions(entries.value))

// Apply the three filters; an empty filter matches everything. The status
// filter matches by DISPLAY state (toDisplayStatus), so 'incident' catches
// down and failing entries together. The result is ranked by the ACTIVE
// column sort (GH #136, sortListEntries): the default is availability DESC
// — strongest first, the user ruling that overturned GH #131's
//「weakest first」default; a null value sinks below every rated row and
// disabled rows last under every key/direction.
const filteredEntries = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return sortListEntries(
    entries.value.filter(entry => {
      if (kw && !entry.model_id.toLowerCase().includes(kw)) return false
      if (familyFilter.value && entry.family !== familyFilter.value) return false
      if (statusFilter.value && toDisplayStatus(entry.status) !== statusFilter.value) return false
      return true
    }),
    listSort.value,
  )
})

// Column sort state (GH #136): owned HERE, not inside the list — the
// toolbar note must restate the current ordering, so the note owner holds
// the state (the list stays presentational: sort prop in, click event
// out). Initialized from localStorage (`hs:list-sort`, the `hs:dark`
// family); every change persists best-effort.
const listSort = ref<ListSort>(loadListSort())
const listSortNoteText = computed(() => listSortNote(listSort.value))

function onListSort(key: ListSortKey) {
  listSort.value = nextListSort(listSort.value, key)
  saveListSort(listSort.value)
}

// The reference toolbar retires the grouping selector (GH #131): the list
// renders one flat section ranked by the active column sort. ModelStatusList
// keeps its section contract intact — grouping returns as a view-level
// change only.
const listSections = computed<ListSection[]>(() => [
  { key: null, label: '', meta: '', entries: filteredEntries.value },
])

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

// --- Recent events auth gate (GH #132) ---------------------------------------

// Session state for the 近期事件 section, checked locally on mount — the
// AppSidebar precedent: deliberately no shared auth store. The view
// remounts on every navigation to '/', so a fresh login is picked up by
// the remount; a failed check reads as anonymous (the section stays off).
const authed = ref(false)

async function refreshAuth() {
  try {
    authed.value = (await fetchAuthStatus()).authenticated
  } catch {
    authed.value = false
  }
}

onMounted(() => {
  start()
  void refreshAuth()
})
</script>

<style scoped>
/* Visible page header (reference design): h1 at the 3xl page-title tier +
   a one-line lede (md secondary). Replaces the old sr-only h1 — the visible
   heading serves the a11y tree directly. */
.page-header {
  /* Region seam rhythm (GH #139): the header sits directly on the gray
     skeleton ground, spaced from the hero tile by the shared space-4
     seam. */
  margin-bottom: var(--hs-space-4);
}
.page-title {
  margin: 0;
  font-size: var(--hs-text-3xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.page-lede {
  margin: var(--hs-space-1) 0 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
}
.dashboard {
  max-width: 1200px;
  margin: 0 auto;
  /* Public-page breathing room (spec 0018 IA: 32–48px whitespace). */
  padding: var(--hs-space-6) var(--hs-space-6) var(--hs-space-8);
}
/* List toolbar (GH #131): the section title on the left, the filter
   controls right-aligned on the same row; wraps on narrow widths. */
.list-toolbar {
  display: flex;
  align-items: center;
  gap: var(--hs-space-4);
  flex-wrap: wrap;
  margin-bottom: var(--hs-space-4);
}
.list-heading {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
  margin: 0;
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.list-note {
  font-size: var(--hs-text-xs);
  font-weight: 400;
  color: var(--hs-text-secondary);
}
.filter-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  flex-wrap: wrap;
  margin-left: auto;
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
