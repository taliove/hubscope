<template>
  <!-- 故障记录 (GH #117, spec 0018 §12): the alert history is rebuilt from a
       plain log table into an event timeline — time / event kind / affected
       model / duration / delivery state — so an operator can read an
       incident's course at a glance. The nine-kind event vocabulary
       (utils/alertKind.ts) is consumed unchanged: event-category words are
       not health states and stay outside the v2 three-state mapping. -->
  <div class="alerts-page">
    <h1 class="page-title">故障记录</h1>

    <!-- Filter bar: model / kind / time range. All three filter the
         client-side window fetched through the existing `limit` API
         parameter — no second server-side filter caliber is invented. -->
    <div class="filter-bar">
      <el-select
        v-model="modelFilter"
        class="filter-model"
        placeholder="全部模型"
        clearable
        :disabled="loading && events.length === 0"
      >
        <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
      </el-select>
      <el-select v-model="kindFilter" class="filter-kind" placeholder="全部类型" clearable>
        <el-option v-for="k in kindOptions" :key="k" :label="alertKindLabel(k)" :value="k" />
      </el-select>
      <el-select v-model="rangeFilter" class="filter-range">
        <el-option v-for="r in RANGE_OPTIONS" :key="r.value" :label="r.label" :value="r.value" />
      </el-select>
    </div>

    <div class="timeline-panel">
      <!-- First-load skeleton: static gray bars (no pulse — the v2 motion
           budget is reserved for status changes, spec 0018 decision 4). -->
      <div v-if="loading && events.length === 0" class="timeline-skeleton" aria-label="加载中">
        <div v-for="i in 5" :key="i" class="skeleton-row">
          <span class="skeleton-bar skeleton-time" />
          <span class="skeleton-node" />
          <div class="skeleton-body">
            <span class="skeleton-bar skeleton-head" />
            <span class="skeleton-bar skeleton-message" />
          </div>
        </div>
      </div>

      <!-- Error state: reason + retry (three-state discipline). -->
      <div v-else-if="error" class="state-block">
        <p class="state-text state-error">加载失败:{{ error }}</p>
        <el-button size="small" @click="reload">重试</el-button>
      </div>

      <!-- Empty state: never reads as "no incidents ever" — the copy names
           the active range so a narrow filter is not mistaken for a clean
           record. -->
      <div v-else-if="visibleMonths.length === 0" class="state-block">
        <p class="state-text">所选范围内暂无告警事件</p>
        <p class="state-hint">可放宽时间范围或清除筛选条件</p>
      </div>

      <template v-else>
        <!-- Month > week > day nesting (GH #145, spec 0019 ruling 3): month
             and week headers are collapse toggles (chevron + count, the
             event-row expansion language — role/tabindex/Enter/Space,
             aria-expanded). Collapsing is pure view organization: hidden
             subtrees are v-if'd, no event is lost and nothing refetches. -->
        <section v-for="month in visibleMonths" :key="month.key" class="month-section">
          <h2
            class="month-label group-toggle"
            role="button"
            tabindex="0"
            :aria-expanded="!isGroupCollapsed(`m:${month.key}`)"
            @click="toggleGroup(`m:${month.key}`)"
            @keydown.enter.prevent="toggleGroup(`m:${month.key}`)"
            @keydown.space.prevent="toggleGroup(`m:${month.key}`)"
          >
            <span class="group-chevron" aria-hidden="true">{{
              isGroupCollapsed(`m:${month.key}`) ? '▸' : '▾'
            }}</span>
            {{ month.label }}
            <span class="group-count">{{ monthEventCount(month) }} 条</span>
          </h2>
          <ol v-if="!isGroupCollapsed(`m:${month.key}`)" class="month-events">
            <!-- The month content is one FLAT row list (week header / day
                 header / event rows share the 44/24/1fr grid) so the rail
                 line paints through every header without a gap — the v1
                 per-day :first-child/:last-child clipping is gone; the line
                 clips only at the month's first/last VISIBLE node (a
                 collapsed week moves the visible ends). -->
            <li v-for="row in monthRows(month)" :key="row.key" :class="rowClass(row)">
              <template v-if="row.type === 'week'">
                <span class="rail" :class="row.rail" aria-hidden="true" />
                <h3
                  class="group-label week-label group-toggle"
                  role="button"
                  tabindex="0"
                  :aria-expanded="!row.collapsed"
                  @click="toggleGroup(row.toggleKey)"
                  @keydown.enter.prevent="toggleGroup(row.toggleKey)"
                  @keydown.space.prevent="toggleGroup(row.toggleKey)"
                >
                  <span class="group-chevron" aria-hidden="true">{{ row.collapsed ? '▸' : '▾' }}</span>
                  {{ row.label }}
                  <span class="group-count">{{ row.count }} 条</span>
                </h3>
              </template>

              <template v-else-if="row.type === 'day'">
                <span class="rail" :class="row.rail" aria-hidden="true" />
                <h4 class="group-label day-label">{{ row.label }}</h4>
              </template>

              <!-- Inline expansion (GH #144, spec 0019 裁决 4, EvalLiveFeed
                   precedent — no dialog, no panel): the summary is the toggle
                   (role/tabindex/Enter/Space, aria-expanded); the expansion
                   set is keyed by event id so filter changes and load-earlier
                   prepends never collapse an open row; multi-open unlimited.
                   The row is self-contained (time / rail / body incl.
                   expansion in one li) — the month>week>day nesting lifts it
                   whole. -->
              <template v-else>
                <span class="event-time">{{ formatClockMinute(row.event.created_at) }}</span>
                <span class="rail" :class="row.rail"
                  ><span class="node" :class="`node--${alertKindTagType(row.event.kind)}`"
                /></span>
                <div class="event-body">
                  <div
                    class="event-summary"
                    role="button"
                    tabindex="0"
                    :aria-expanded="isExpanded(row.event.id)"
                    :aria-controls="`event-detail-${row.event.id}`"
                    @click="toggleExpand(row.event.id)"
                    @keydown.enter.prevent="toggleExpand(row.event.id)"
                    @keydown.space.prevent="toggleExpand(row.event.id)"
                  >
                    <div class="event-head">
                      <el-tag :type="alertKindTagType(row.event.kind)" size="small" class="event-kind">
                        {{ alertKindLabel(row.event.kind) }}
                      </el-tag>
                      <span
                        v-if="targetOf(row.event)"
                        class="event-target"
                        :title="targetOf(row.event) ?? undefined"
                      >
                        {{ targetOf(row.event) }}
                      </span>
                      <span
                        v-if="durationText(row.event)"
                        class="event-duration"
                        :class="{ ongoing: isOngoing(row.event) }"
                      >
                        {{ durationText(row.event) }}
                      </span>
                      <span class="event-sent" :class="sentClass(row.event)">{{ alertSentLabel(row.event) }}</span>
                      <span class="event-expand" aria-hidden="true">{{ isExpanded(row.event.id) ? '▾' : '▸' }}</span>
                    </div>
                    <p class="event-message" :title="row.event.message">{{ row.event.message }}</p>
                  </div>
                  <!-- Expansion: five stacked items — full message / full
                       timestamp / ids / paired recovery + duration / delivery
                       detail — plus the endpoint deep link (the page's only
                       outbound interaction). Deleted endpoints stay linkable
                       by raw id. -->
                  <div v-if="isExpanded(row.event.id)" :id="`event-detail-${row.event.id}`" class="event-detail">
                    <p class="detail-message">{{ detailOf(row.event).message }}</p>
                    <div class="detail-row">
                      <span class="detail-label">时间</span>
                      <span class="detail-value">{{ detailOf(row.event).timestamp }}</span>
                    </div>
                    <div class="detail-row">
                      <span class="detail-label">标识</span>
                      <span class="detail-value">{{ detailOf(row.event).idText }}</span>
                    </div>
                    <div class="detail-row">
                      <span class="detail-label">配对</span>
                      <span
                        class="detail-value"
                        :class="{ ongoing: detailOf(row.event).pairing.state === 'ongoing' }"
                      >{{ detailOf(row.event).pairing.text }}</span>
                    </div>
                    <div class="detail-row">
                      <span class="detail-label">投递</span>
                      <span class="detail-value">{{ detailOf(row.event).sentText }}</span>
                    </div>
                    <router-link
                      v-if="detailOf(row.event).endpointId !== null"
                      class="detail-link"
                      :to="`/endpoints/${detailOf(row.event).endpointId}`"
                    >查看端点详情</router-link>
                  </div>
                </div>
              </template>
            </li>
          </ol>
        </section>

        <!-- Pagination walks the existing `limit` API parameter (the only
             one the endpoint honors, capped at 200 server-side). -->
        <div class="timeline-foot">
          <el-button v-if="canLoadMore" text :loading="loading" @click="loadMore">
            加载更早的事件
          </el-button>
          <span v-else-if="atServerCap" class="foot-hint">
            已达单次上限 200 条,更早事件请缩小时间范围
          </span>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listAlerts, type AlertEvent, type AlertKind } from '@/api/settings'
import { fetchAuthStatus } from '@/api/auth'
import { fetchOverview } from '@/api/overview'
import { formatClockMinute, formatDuration } from '@/utils/format'
import { alertKindLabel, alertKindTagType, visibleKindOptions } from '@/utils/alertKind'
import {
  filterEventsByTimeRange,
  pairIncidentDurations,
  groupEventsByMonthWeekDay,
  alertsFilterToQuery,
  parseAlertsFilterQuery,
  alertSentLabel,
  buildEventDetail,
  type AlertTimeRange,
  type IncidentDuration,
  type AlertEventDetail,
  type AlertMonthGroup,
  type AlertWeekGroup,
} from '@/utils/alertTimeline'

// Time-range presets (今天 = local calendar day; the rest are rolling).
const RANGE_OPTIONS: Array<{ value: AlertTimeRange; label: string }> = [
  { value: 'today', label: '今天' },
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' },
]

// The backend caps `limit` at 200 (internal/server/probes.go parseLimit);
// pagination walks 50 → 100 → … → 200 through that single parameter.
const INITIAL_LIMIT = 50
const LIMIT_STEP = 50
const SERVER_LIMIT_CAP = 200

const events = ref<AlertEvent[]>([])
const loading = ref(false)
const error = ref('')
const limit = ref(INITIAL_LIMIT)

// --- URL deep-link state (GH #143, spec 0019 T3) -----------------------------
// The three filter-bar conditions mirror into the query string so any
// timeline view is shareable/bookmarkable by copying the address bar. On
// mount the URL WINS (a pasted link must reproduce the exact view) —
// DashboardView five-param precedent (2026-08-02).
const route = useRoute()
const router = useRouter()
const initialFilter = parseAlertsFilterQuery(route.query)

const modelFilter = ref<string | null>(initialFilter.model)
const kindFilter = ref<AlertKind | null>(initialFilter.kind)
const rangeFilter = ref<AlertTimeRange>(initialFilter.range)

// Mirror the filters into the query string (router.replace — no history
// spam; query changes never remount the page, App.vue keys on path).
// Defaults stay OUT of the query so a clean URL is the default view. A
// 200ms debounce keeps rapid select changes to one replace per pause.
let querySyncTimer: ReturnType<typeof setTimeout> | null = null
watch([modelFilter, kindFilter, rangeFilter], () => {
  if (querySyncTimer) clearTimeout(querySyncTimer)
  querySyncTimer = setTimeout(() => {
    void router.replace({
      query: alertsFilterToQuery({
        model: modelFilter.value,
        kind: kindFilter.value,
        range: rangeFilter.value,
      }),
    })
  }, 200)
})
onBeforeUnmount(() => {
  if (querySyncTimer) clearTimeout(querySyncTimer)
})

// Session state forks the type filter's option set (spec 0019, GH #142):
// anonymous visitors only see the four incident-narrative kinds their
// payload can ever contain — the other seven would be unmatchable options,
// i.e. dishonest UI. A failed status check is treated as anonymous
// (AppSidebar / router guard precedent): failure never impersonates login.
const authed = ref(false)
const kindOptions = computed(() => visibleKindOptions(authed.value))

async function refreshAuth() {
  try {
    authed.value = (await fetchAuthStatus()).authenticated
  } catch {
    authed.value = false
  }
}

// Re-check on every route switch (AppTopbar / AppSidebar shell precedent) —
// a login/logout on another page must fork the kind options without a
// reload (check GH #142 LOW-1, fixed alongside GH #143). The watch source
// is route.path, NOT fullPath: this view mirrors its own filters into the
// query (debounced router.replace above), and watching fullPath would
// self-trigger a redundant /api/auth/status on every filter change
// (check GH #143 LOW-2).
watch(() => route.path, refreshAuth)

// endpoint_id → model_id resolution map from the overview payload. A deleted
// endpoint drops out of the overview and falls back to its raw id label —
// the history must still render (audit surface never loses rows).
const endpointModels = ref<Map<number, string>>(new Map())

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [alertsResult, overviewResult] = await Promise.allSettled([
      listAlerts(limit.value),
      fetchOverview(),
    ])
    if (alertsResult.status === 'rejected') {
      throw alertsResult.reason
    }
    events.value = alertsResult.value
    if (overviewResult.status === 'fulfilled') {
      const map = new Map<number, string>()
      for (const entry of overviewResult.value.endpoints) {
        map.set(entry.endpoint_id, entry.model_id)
      }
      endpointModels.value = map
    }
    // An overview failure only degrades model labels to raw ids — the
    // timeline itself must still render.
  } catch (err) {
    error.value = (err as Error).message
  } finally {
    loading.value = false
  }
}

function loadMore() {
  limit.value = Math.min(limit.value + LIMIT_STEP, SERVER_LIMIT_CAP)
  void reload()
}

onMounted(() => {
  void reload()
  void refreshAuth()
})

// The model an event affects: the resolved model id for endpoint events,
// null for vendor-group and hub-less events (those carry no single model).
function modelOf(ev: AlertEvent): string | null {
  if (ev.endpoint_id === null) return null
  return endpointModels.value.get(ev.endpoint_id) ?? `#${ev.endpoint_id}`
}

// The 影响对象 cell: vendor family name for group alerts, model id for
// endpoint events, nothing for hub-less events (test / batch / quiet
// summary / score comparisons — the message line carries their detail).
function targetOf(ev: AlertEvent): string | null {
  if (ev.group_key !== null) return ev.group_key
  return modelOf(ev)
}

const modelOptions = computed(() => {
  const set = new Set<string>()
  for (const ev of events.value) {
    const m = modelOf(ev)
    if (m !== null) set.add(m)
  }
  return [...set].sort()
})

// Duration pairing is computed on the UNFILTERED window so a display filter
// can never change an incident's span. Known caliber: a down whose recovery
// falls outside the fetched window reads 进行中.
const durations = computed<Map<number, IncidentDuration>>(() => pairIncidentDurations(events.value))

function durationOf(ev: AlertEvent): IncidentDuration | null {
  return durations.value.get(ev.id) ?? null
}

function isOngoing(ev: AlertEvent): boolean {
  return durationOf(ev)?.state === 'ongoing'
}

function durationText(ev: AlertEvent): string {
  const d = durationOf(ev)
  if (!d) return ''
  return d.state === 'paired' ? `持续 ${formatDuration(d.ms)}` : '进行中'
}

// Delivery state cell — the words come from the shared alertSentLabel
// (alertTimeline.ts) so the row and the inline detail never fork; only the
// color class stays a component concern.
function sentClass(ev: AlertEvent): string {
  if (ev.kind === 'score_drop_skipped') return 'sent--skip'
  return ev.sent_ok ? 'sent--ok' : 'sent--fail'
}

// Inline expansion state (GH #144), keyed by event id — EvalLiveFeed
// precedent: filter changes and load-earlier prepends never collapse an
// open row, and multi-open is unlimited. Every update replaces the Set to
// stay reactive.
const expandedIds = ref<Set<number>>(new Set())

function isExpanded(id: number): boolean {
  return expandedIds.value.has(id)
}

function toggleExpand(id: number) {
  const next = new Set(expandedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedIds.value = next
}

// The expansion content is assembled by the pure builder on the UNFILTERED
// window with the shared pairing map — no second pairing caliber.
function detailOf(ev: AlertEvent): AlertEventDetail {
  return buildEventDetail(ev, events.value, durations.value)
}

const visibleMonths = computed(() => {
  const now = new Date()
  let filtered = filterEventsByTimeRange(events.value, rangeFilter.value, now)
  if (kindFilter.value !== null) {
    const kind = kindFilter.value
    filtered = filtered.filter((ev) => ev.kind === kind)
  }
  if (modelFilter.value !== null) {
    const model = modelFilter.value
    filtered = filtered.filter((ev) => modelOf(ev) === model)
  }
  return groupEventsByMonthWeekDay(filtered, now)
})

// --- Group collapse state (GH #145, spec 0019 ruling 3) --------------------
// Session-only (no localStorage — the ruling explicitly rejects persistence).
// The Map stores user OVERRIDES keyed by group key (`m:YYYY-MM` /
// `w:YYYY-MM:YYYY-MM-DD`), so filter changes keep a toggled group's state
// for as long as its key survives. Default (2026-08-02 main ruling): the
// current month and every week are expanded; earlier months are collapsed.
// Every update replaces the Map to stay reactive.
const groupOverrides = ref<Map<string, boolean>>(new Map())

// Local calendar month key of right now — the same caliber the grouping
// function buckets by (local YYYY-MM).
function currentMonthKey(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

function isGroupCollapsed(key: string): boolean {
  const override = groupOverrides.value.get(key)
  if (override !== undefined) return override
  return key.startsWith('m:') && key.slice(2) !== currentMonthKey()
}

function toggleGroup(key: string) {
  const next = new Map(groupOverrides.value)
  next.set(key, !isGroupCollapsed(key))
  groupOverrides.value = next
}

function weekEventCount(week: AlertWeekGroup): number {
  let n = 0
  for (const day of week.days) n += day.events.length
  return n
}

function monthEventCount(month: AlertMonthGroup): number {
  let n = 0
  for (const week of month.weeks) n += weekEventCount(week)
  return n
}

// One flattened render row of a month section: week-header / day-header /
// event rows share the 44/24/1fr grid so the rail column lines up.
type RailClass = 'rail--none' | 'rail--start' | 'rail--end' | 'rail--full'
type TimelineRow =
  | {
      type: 'week'
      key: string
      rail: RailClass
      label: string
      count: number
      collapsed: boolean
      toggleKey: string
    }
  | { type: 'day'; key: string; rail: RailClass; label: string }
  | { type: 'event'; key: string; rail: RailClass; event: AlertEvent }

function rowClass(row: TimelineRow): string {
  return row.type === 'event' ? 'event-row' : `group-row group-row--${row.type}`
}

// monthRows flattens a month section into render rows, honoring the collapse
// state (a collapsed week renders only its header row — its events are
// hidden, never dropped). Rail continuity (ruling 3): the line is ONE
// continuous stroke through days and weeks — every header paints its own
// segment, so the stroke never breaks at a group boundary; it starts at the
// first VISIBLE node and ends at the last (rows outside that span paint
// nothing, and a lone visible node paints no line either — the v1
// :only-child caliber). A collapsed week therefore moves the visible ends
// and the clipping follows.
function monthRows(month: AlertMonthGroup): TimelineRow[] {
  const rows: TimelineRow[] = []
  for (const week of month.weeks) {
    const toggleKey = `w:${month.key}:${week.key}`
    const collapsed = isGroupCollapsed(toggleKey)
    rows.push({
      type: 'week',
      key: toggleKey,
      rail: 'rail--full',
      label: week.label,
      count: weekEventCount(week),
      collapsed,
      toggleKey,
    })
    if (collapsed) continue
    for (const day of week.days) {
      rows.push({ type: 'day', key: `d:${day.key}`, rail: 'rail--full', label: day.label })
      for (const ev of day.events) {
        rows.push({ type: 'event', key: `e:${ev.id}`, rail: 'rail--full', event: ev })
      }
    }
  }

  let firstIdx = -1
  let lastIdx = -1
  for (let i = 0; i < rows.length; i++) {
    if (rows[i].type !== 'event') continue
    if (firstIdx === -1) firstIdx = i
    lastIdx = i
  }
  for (let i = 0; i < rows.length; i++) {
    const row = rows[i]
    if (firstIdx === -1 || i < firstIdx || i > lastIdx) {
      row.rail = 'rail--none'
    } else if (row.type === 'event') {
      if (firstIdx === lastIdx) row.rail = 'rail--none'
      else if (i === firstIdx) row.rail = 'rail--start'
      else if (i === lastIdx) row.rail = 'rail--end'
    }
  }
  return rows
}

// "Load earlier" stays available while the last fetch returned a full page
// (shorter page = the backend has no more) and the server cap is unreached.
const canLoadMore = computed(
  () => events.value.length >= limit.value && limit.value < SERVER_LIMIT_CAP,
)
const atServerCap = computed(
  () => events.value.length >= SERVER_LIMIT_CAP && limit.value >= SERVER_LIMIT_CAP,
)
</script>

<style scoped>
.alerts-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.page-title {
  margin: 0 0 var(--hs-space-5);
  font-size: var(--hs-text-3xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}

/* Filter bar */
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--hs-space-3);
  margin-bottom: var(--hs-space-4);
}
.filter-model {
  width: 200px;
}
.filter-kind {
  width: 160px;
}
.filter-range {
  width: 140px;
}

/* Timeline panel: the light-container Apple syntax — white surface, 1px
   border, radius-lg, no shadow (static container). */
.timeline-panel {
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
  padding: var(--hs-space-5) var(--hs-space-6);
}

/* Month sections (GH #145): the inter-month gap is the ONE place the rail
   line breaks — clipping happens only at month boundaries and the panel
   top/bottom (ruling 3). */
.month-section + .month-section {
  margin-top: var(--hs-space-6);
}
.month-label {
  margin: 0 0 var(--hs-space-2);
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}

/* Month content: one flat row list. Week/day header rows share the event
   rows' grid (44px time gutter / 24px rail / body) so every label aligns
   with the event bodies and the rail column paints its segment straight
   through each header — the line reads as a single stroke. */
.month-events {
  list-style: none;
  margin: 0;
  padding: 0;
}
.group-row {
  display: grid;
  grid-template-columns: 44px 24px minmax(0, 1fr);
  column-gap: var(--hs-space-2);
  padding: var(--hs-space-1) var(--hs-space-2);
}
.group-row .rail {
  grid-column: 2;
}
.group-label {
  grid-column: 3;
  margin: 0;
  align-self: center;
}
.week-label {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-regular);
}
.day-label {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-secondary);
}

/* Collapse toggles (month/week headers): the event-row expansion language —
   pointer + chevron carry affordance, focus-visible speaks the shell's
   single focus language (2px brand outline). */
.group-toggle {
  cursor: pointer;
  user-select: none;
  border-radius: var(--hs-radius-sm);
}
.group-toggle:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 2px;
}
.group-chevron {
  display: inline-block;
  width: 14px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.group-count {
  margin-left: var(--hs-space-2);
  font-size: var(--hs-text-xs);
  font-weight: 400;
  color: var(--hs-text-placeholder);
}

/* Event rows: [time | rail | body]. The rail column carries the continuous
   vertical line; each event is a node on it. */
.event-row {
  display: grid;
  grid-template-columns: 44px 24px minmax(0, 1fr);
  column-gap: var(--hs-space-2);
  padding: var(--hs-space-2) var(--hs-space-2);
  border-radius: var(--hs-radius-sm);
}
.event-row:hover {
  background: var(--hs-bg-hover);
}
.event-time {
  padding-top: 2px;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.rail {
  position: relative;
}
/* The continuous line (GH #145): every row paints its own full-height
   segment; modifier classes assigned from the month's VISIBLE ends clip the
   stroke so it starts at the first node and ends at the last. The v1
   per-day :first-child/:last-child clipping is gone. */
.rail::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 50%;
  width: 2px;
  margin-left: -1px;
  background: var(--hs-border-light);
}
.rail--none::before {
  display: none;
}
.rail--start::before {
  top: 10px;
}
.rail--end::before {
  bottom: auto;
  height: 10px;
}
.node {
  position: absolute;
  top: 6px;
  left: 50%;
  width: 9px;
  height: 9px;
  margin-left: -4.5px;
  border-radius: var(--hs-radius-full);
  background: var(--hs-info);
}
/* Node color = the event kind's tag type, mapped onto the graphic-tier
   functional tokens (text scenarios elsewhere use the *-text steps). */
.node--danger {
  background: var(--hs-danger);
}
.node--success {
  background: var(--hs-success);
}
.node--warning {
  background: var(--hs-warning);
}
.node--info,
.node--primary {
  background: var(--hs-info);
}

/* Event body */
.event-body {
  min-width: 0;
  padding-bottom: var(--hs-space-2);
}
.event-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--hs-space-2);
}
.event-kind {
  flex: none;
}
.event-target {
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.event-duration {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  font-variant-numeric: tabular-nums;
}
/* An unclosed incident reads in danger text — it is the one cell on the
   timeline that means "still happening". */
.event-duration.ongoing {
  color: var(--hs-danger-text);
  font-weight: 600;
}
.event-sent {
  margin-left: auto;
  font-size: var(--hs-text-xs);
}
.sent--ok {
  color: var(--hs-success-text);
}
.sent--fail {
  color: var(--hs-danger-text);
}
.sent--skip {
  color: var(--hs-text-secondary);
}
.event-message {
  margin: var(--hs-space-1) 0 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  line-height: 1.5;
  /* Aggregate messages (batch / quiet summary) can run long — clamp to two
     lines in the list state; the inline expansion carries the full text. */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* Row expansion (GH #144): the summary is the toggle — pointer + the row's
   existing hover fill carry affordance, focus-visible speaks the shell's
   single focus language (2px brand outline). */
.event-summary {
  cursor: pointer;
  user-select: none;
  border-radius: var(--hs-radius-sm);
}
.event-summary:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 2px;
}
.event-expand {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}

/* Inline detail: a hairline separates it from the summary; stacked
   label/value rows in the timeline's secondary voice (EvalLiveFeed
   four-block precedent). */
.event-detail {
  margin-top: var(--hs-space-2);
  padding-top: var(--hs-space-2);
  border-top: 1px solid var(--hs-border-light);
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.detail-message {
  margin: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.detail-row {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr);
  gap: var(--hs-space-3);
  align-items: baseline;
}
.detail-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.detail-value {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  font-variant-numeric: tabular-nums;
  word-break: break-word;
}
/* Same caliber as the row's ongoing duration cell: the one reading on the
   timeline that means "still happening". */
.detail-value.ongoing {
  color: var(--hs-danger-text);
  font-weight: 600;
}
/* 查看端点详情 — the page's only outbound interaction (feed-fix-link
   precedent: brand-colored inline action, secondary weight). */
.detail-link {
  align-self: flex-start;
  font-size: var(--hs-text-xs);
  color: var(--hs-brand);
  text-decoration: none;
}
.detail-link:hover {
  color: var(--hs-brand-hover);
  text-decoration: underline;
}

/* Footer (load-more / cap hint) */
.timeline-foot {
  display: flex;
  justify-content: center;
  margin-top: var(--hs-space-4);
}
.foot-hint {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}

/* Three-state blocks */
.state-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--hs-space-2);
  padding: var(--hs-space-7) 0;
}
.state-text {
  margin: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
}
.state-error {
  color: var(--hs-danger-text);
}
.state-hint {
  margin: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
}

/* Skeleton */
.timeline-skeleton {
  display: flex;
  flex-direction: column;
}
.skeleton-row {
  display: grid;
  grid-template-columns: 44px 24px minmax(0, 1fr);
  column-gap: var(--hs-space-2);
  padding: var(--hs-space-3) var(--hs-space-2);
  align-items: start;
}
.skeleton-bar {
  display: block;
  height: 12px;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
}
.skeleton-time {
  width: 32px;
  justify-self: end;
  margin-top: 2px;
}
.skeleton-node {
  width: 9px;
  height: 9px;
  margin-top: 4px;
  border-radius: var(--hs-radius-full);
  background: var(--hs-bg-hover);
  justify-self: center;
}
.skeleton-body {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.skeleton-head {
  width: 40%;
}
.skeleton-message {
  width: 70%;
}
</style>
