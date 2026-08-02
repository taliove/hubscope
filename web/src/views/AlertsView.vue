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
      <div v-else-if="visibleGroups.length === 0" class="state-block">
        <p class="state-text">所选范围内暂无告警事件</p>
        <p class="state-hint">可放宽时间范围或清除筛选条件</p>
      </div>

      <template v-else>
        <section v-for="group in visibleGroups" :key="group.key" class="day-section">
          <h2 class="day-label">{{ group.label }}</h2>
          <ol class="day-events">
            <li v-for="ev in group.events" :key="ev.id" class="event-row">
              <span class="event-time">{{ formatClockMinute(ev.created_at) }}</span>
              <span class="rail"><span class="node" :class="`node--${alertKindTagType(ev.kind)}`" /></span>
              <div class="event-body">
                <div class="event-head">
                  <el-tag :type="alertKindTagType(ev.kind)" size="small" class="event-kind">
                    {{ alertKindLabel(ev.kind) }}
                  </el-tag>
                  <span v-if="targetOf(ev)" class="event-target" :title="targetOf(ev) ?? undefined">
                    {{ targetOf(ev) }}
                  </span>
                  <span v-if="durationText(ev)" class="event-duration" :class="{ ongoing: isOngoing(ev) }">
                    {{ durationText(ev) }}
                  </span>
                  <span class="event-sent" :class="sentClass(ev)">{{ sentText(ev) }}</span>
                </div>
                <p class="event-message" :title="ev.message">{{ ev.message }}</p>
              </div>
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
import { computed, onMounted, ref } from 'vue'
import { listAlerts, type AlertEvent, type AlertKind } from '@/api/settings'
import { fetchAuthStatus } from '@/api/auth'
import { fetchOverview } from '@/api/overview'
import { formatClockMinute, formatDuration } from '@/utils/format'
import { alertKindLabel, alertKindTagType, visibleKindOptions } from '@/utils/alertKind'
import {
  filterEventsByTimeRange,
  pairIncidentDurations,
  groupEventsByDate,
  type AlertTimeRange,
  type IncidentDuration,
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

const modelFilter = ref<string | null>(null)
const kindFilter = ref<AlertKind | null>(null)
const rangeFilter = ref<AlertTimeRange>('7d')

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

// Delivery state cell — the words (成功/失败/未发送) are the existing
// vocabulary carried over from the history table unchanged.
function sentText(ev: AlertEvent): string {
  if (ev.kind === 'score_drop_skipped') return '未发送'
  return ev.sent_ok ? '成功' : '失败'
}

function sentClass(ev: AlertEvent): string {
  if (ev.kind === 'score_drop_skipped') return 'sent--skip'
  return ev.sent_ok ? 'sent--ok' : 'sent--fail'
}

const visibleGroups = computed(() => {
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
  return groupEventsByDate(filtered, now)
})

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

/* Day sections */
.day-section + .day-section {
  margin-top: var(--hs-space-6);
}
.day-label {
  margin: 0 0 var(--hs-space-3);
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}

/* Event rows: [time | rail | body]. The rail column carries the continuous
   vertical line; each event is a node on it. */
.day-events {
  list-style: none;
  margin: 0;
  padding: 0;
}
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
/* The continuous line: every row paints its own segment; the first and
   last rows of a day clip theirs so the line starts and ends at a node. */
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
.event-row:first-child .rail::before {
  top: 10px;
}
.event-row:last-child .rail::before {
  bottom: auto;
  height: 10px;
}
.event-row:only-child .rail::before {
  display: none;
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
     lines, the title attribute carries the full text. */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
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
