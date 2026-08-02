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
         no-probe windows never read as 全部稳定. -->
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

    <!-- List toolbar (GH #131, reference design; GH #136 sort rework; GH
         #140 grouping regression): the section title carries the sort note
         — DYNAMIC since GH #136, it restates the current column ordering
         so the label stays literally true after the user re-sorts (a label
         the data does not honor is an anti-fake violation; in grouped mode
         it describes the IN-GROUP ordering, the group ranking being the
         severity rank). The controls right-align on the same row: keyword
         + vendor + display status + grouping. -->
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
        <span class="filter-field">
          <span class="filter-label">供应商：</span>
          <el-select v-model="familyFilter" placeholder="全部" clearable class="filter-select">
            <!-- Options derive from the unfiltered entry set (familyOptions) so
                 an active filter never collapses its own option list. -->
            <el-option v-for="f in vendorOptions" :key="f" :label="f" :value="f" />
          </el-select>
        </span>
        <span class="filter-field">
          <span class="filter-label">状态：</span>
          <el-select v-model="statusFilter" placeholder="全部" clearable class="filter-select">
            <!-- Options come from the single display-layer mapping (GH #113):
                 the three display states, light → heavy; down + failing filter
                 together under 异常. No status word literals here. -->
            <el-option v-for="s in statusOptions" :key="s" :label="statusLabel(s)" :value="s" />
          </el-select>
        </span>
        <!-- Grouping selector (GH #140 regression, undoing the GH #131
             retirement): flat / by vendor / by capability / by protocol,
             default flat. Options from the modelList single Record source;
             grouping is a VIEW organization — the share snapshot scope
             (filter chips) does not change. -->
        <el-select v-model="grouping" class="filter-select">
          <el-option v-for="g in LIST_GROUPINGS" :key="g" :label="LIST_GROUPING_LABELS[g]" :value="g" />
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
        @share-group="openGroupShare"
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
import { ref, computed, nextTick, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Share } from '@element-plus/icons-vue'
import { useOverview, POLL_INTERVAL_MS } from '@/composables/useOverview'
import { fetchAuthStatus } from '@/api/auth'
import StatusHero from '@/components/StatusHero.vue'
import MetricWidgets from '@/components/MetricWidgets.vue'
import ModelStatusList, { type ListSection } from '@/components/ModelStatusList.vue'
import ModelDetailPanel from '@/components/ModelDetailPanel.vue'
import StatusShareDialog from '@/components/StatusShareDialog.vue'
import RecentEvents from '@/components/RecentEvents.vue'
import type { StatusCardSnapshot, GroupDimension } from '@/utils/statusCardSnapshot'
import { DISPLAY_SEVERITY_ORDER, statusLabel, toDisplayStatus, type DisplayStatus } from '@/utils/statusDisplay'
import {
  familyOptions,
  groupListSections,
  LIST_GROUPING_DEFAULT,
  LIST_GROUPING_LABELS,
  LIST_GROUPINGS,
  LIST_SORT_DEFAULT,
  listSectionMeta,
  listSortNote,
  listSortToQuery,
  loadListSort,
  nextListSort,
  parseListSortQuery,
  saveListSort,
  sortListEntries,
  type ListGrouping,
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

// --- URL deep-link state (2026-08-02) -----------------------------------------
// Every toolbar condition + the grouping + the active sort mirrors into the
// query string, so any board view is shareable/bookmarkable by copying the
// address bar. On mount the URL WINS (a pasted link must reproduce the exact
// view); the sort falls back to localStorage when the URL carries no param.
const route = useRoute()
const router = useRouter()
const initialQuery = route.query

const keyword = ref(typeof initialQuery.q === 'string' ? initialQuery.q : '')
// Vendor (供应商) filter of the reference toolbar (GH #131): matches the
// entry's family classification; the options derive from the unfiltered set.
const familyFilter = ref(typeof initialQuery.family === 'string' ? initialQuery.family : '')
// The status filter speaks DISPLAY states (GH #113): the domain status
// machine keeps four states, but the board renders three — down and
// failing filter together under 'incident' (异常).
const statusFilter = ref<DisplayStatus | ''>(
  DISPLAY_SEVERITY_ORDER.includes(initialQuery.status as DisplayStatus)
    ? (initialQuery.status as DisplayStatus)
    : '',
)
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

// Group share (2026-08-02): the group header's share button opens the same
// dialog with the snapshot scoped to THAT group — entries are the group's
// (already filter-scoped) rows, and the group chip leads the scope chips
// so a subset never reads as the global picture (the ticket-59 `group`
// snapshot field, revived with real group headers).
function openGroupShare(section: ListSection) {
  shareSnapshot.value = {
    entries: [...section.entries],
    keyword: keyword.value.trim(),
    protocol: '',
    family: familyFilter.value || undefined,
    status: statusFilter.value,
    group:
      grouping.value !== LIST_GROUPING_DEFAULT && section.key !== null
        ? { dimension: grouping.value as GroupDimension, key: section.key }
        : null,
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
// out). Initialized from the URL `sort` param when present (a pasted link
// reproduces the exact view, 2026-08-02), else localStorage
// (`hs:list-sort`, the `hs:dark` family); every change persists
// best-effort.
const listSort = ref<ListSort>(parseListSortQuery(initialQuery.sort) ?? loadListSort())
const listSortNoteText = computed(() => listSortNote(listSort.value))

function onListSort(key: ListSortKey) {
  listSort.value = nextListSort(listSort.value, key)
  saveListSort(listSort.value)
}

// Grouping dimension of the list (GH #140 regression): the selector the
// GH #131 reference toolbar retired returns with the FLAT list as the
// default. View-level organization only — sorting/filtering/share all
// keep their current calibers (the snapshot scope chips describe filters;
// grouping is not a filter). Initialized from the URL `group` param
// (2026-08-02 deep-link).
const grouping = ref<ListGrouping>(
  LIST_GROUPINGS.includes(initialQuery.group as ListGrouping)
    ? (initialQuery.group as ListGrouping)
    : LIST_GROUPING_DEFAULT,
)

// Mirror the view state into the query string (router.replace — no history
// spam; query changes never remount the page, App.vue keys on path).
// Defaults stay OUT of the query so a clean URL is the default view. A
// 200ms debounce keeps keyword typing to one replace per pause.
let querySyncTimer: ReturnType<typeof setTimeout> | null = null
watch(
  [keyword, familyFilter, statusFilter, grouping, listSort],
  () => {
    if (querySyncTimer) clearTimeout(querySyncTimer)
    querySyncTimer = setTimeout(() => {
      const query: Record<string, string> = {}
      const kw = keyword.value.trim()
      if (kw) query.q = kw
      if (familyFilter.value) query.family = familyFilter.value
      if (statusFilter.value) query.status = statusFilter.value
      if (grouping.value !== LIST_GROUPING_DEFAULT) query.group = grouping.value
      const sortQuery = listSortToQuery(listSort.value)
      if (sortQuery !== listSortToQuery(LIST_SORT_DEFAULT)) query.sort = sortQuery
      void router.replace({ query })
    }, 200)
  },
)
onBeforeUnmount(() => {
  if (querySyncTimer) clearTimeout(querySyncTimer)
})

// The list sections: flat mode renders ONE section ranked by the active
// column sort; grouped mode (GH #140) buckets the same filtered entries —
// group ranking = the most severe enabled entry's severity rank
// (severitySort single rank table via modelList.groupListSections),
// in-group ordering = the active column sort (bucket rules kept), so the
// toolbar sort note stays literally true inside every group. The meta
// line's count words come from the display-layer single mapping
// (listSectionMeta — never a literal); family groups carry tileFamily so
// the header renders the group vendor tile. Each group header also carries
// the group's 24h signal strip (2026-08-02): the probe-weighted aggregate
// of the group's ENABLED entries (aggregateDots24h — never a per-endpoint
// average, overviewDots discipline).
const listSections = computed<ListSection[]>(() => {
  if (grouping.value === 'none') {
    return [{ key: null, label: '', meta: '', entries: filteredEntries.value }]
  }
  return groupListSections(filteredEntries.value, grouping.value, listSort.value).map(group => ({
    key: group.key,
    label: group.key,
    meta: listSectionMeta(group.entries),
    entries: group.entries,
    tileFamily: grouping.value === 'family' ? group.key : null,
    dots: aggregateDots24h(group.entries.filter(e => e.enabled)),
  }))
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
/* Label + select wrap as one unit (2026-08-01 narrow batch): without the
   pairing wrapper a wrapped toolbar can orphan the label at the end of one
   row while its select lands on the next. Desktop rendering is unchanged —
   the wrapper is a plain inline-flex with the same gap rhythm. */
.filter-field {
  display: inline-flex;
  align-items: center;
  gap: var(--hs-space-2);
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

/* Narrow form (2026-08-01 shell drawer batch): below the 1024px breakpoint
   the page padding tightens to space-4 (the registered narrow container
   padding) and the toolbar reflows — the keyword field takes a full row,
   the selects share the next rows evenly, and the share button keeps its
   trailing seat. The desktop layout above is pixel-untouched. */
@media (max-width: 1023px) {
  .dashboard {
    padding: var(--hs-space-4) var(--hs-space-4) var(--hs-space-6);
  }
  .filter-row {
    width: 100%;
    margin-left: 0;
  }
  .filter-keyword {
    width: 100%;
  }
  /* Paired fields share a row evenly; the share button goes full-width on
     its own row — thumb-sized and unambiguous. */
  .filter-field {
    flex: 1 1 150px;
  }
  .filter-field .filter-select {
    flex: 1 1 0;
    width: auto;
  }
  /* The grouping select has no label pair — it takes the same even share. */
  .filter-row > .filter-select {
    flex: 1 1 150px;
    width: auto;
  }
  .share-btn {
    width: 100%;
    margin-left: 0;
  }
}
</style>
