<template>
  <!-- The v-if IS the information boundary (GH #132, main ruling on the
       ticket): the alerts API is session-protected and the public status
       board must not leak operations events to anonymous readers — when
       `authed` is false the section renders nothing and reload() below
       refuses to run, so an unauthenticated dashboard issues zero alerts
       requests. -->
  <section v-if="authed" class="recent-events" aria-labelledby="recent-events-title">
    <div class="section-head">
      <h2 id="recent-events-title" class="section-title">近期事件</h2>
      <router-link to="/alerts" class="view-all">查看全部事件</router-link>
    </div>

    <!-- First-load skeleton: four static gray cards anchored to the real
         card height (no pulse — the v2 motion budget is reserved for status
         changes, spec 0018 decision 4). The bar layout mirrors the real
         card: time-left + chip-right top row, title, two meta lines. Later
         refreshes keep the cards on screen (local refresh, never a
         re-mount). -->
    <div v-if="showSkeleton" class="card-grid" aria-label="加载中">
      <div v-for="i in RECENT_EVENTS_CARD_LIMIT" :key="i" class="event-card skeleton-card">
        <div class="sk-toprow">
          <span class="skeleton-bar sk-time" />
          <span class="skeleton-bar sk-chip" />
        </div>
        <span class="skeleton-bar sk-title" />
        <span class="skeleton-bar sk-meta" />
        <span class="skeleton-bar sk-meta" />
      </div>
    </div>

    <!-- Error state: reason + retry; a failure stays inside this section
         and never takes the board down with it. -->
    <div v-else-if="error && cards.length === 0" class="state-panel">
      <p class="state-text state-error">加载失败:{{ error }}</p>
      <el-button size="small" @click="reload">重试</el-button>
    </div>

    <!-- Empty state: a quiet recent window — the wording must never read
         as "no incidents ever" (anti-fake discipline). -->
    <div v-else-if="cards.length === 0" class="state-panel">
      <p class="state-text">近期无告警事件</p>
    </div>

    <div v-else class="card-grid">
      <article v-for="card in cardModels" :key="card.ev.id" class="event-card">
        <div class="card-top">
          <span class="event-time">{{ formatClockMinute(card.ev.created_at) }}</span>
          <!-- Chip double-track (GH #138): color = the kind's tag type
               (soft ground + text step), word = incident state for openers
               or the kind word for point-in-time events. -->
          <span class="status-chip" :class="`chip--${card.chip.tone}`">
            {{ card.chip.text }}
          </span>
        </div>
        <p class="event-title" :title="card.ev.message">{{ eventTitle(card.ev.message) }}</p>
        <div class="event-meta">
          <span class="meta-line" :title="card.impact">{{ card.impact }}</span>
          <span v-if="card.duration" class="meta-line">{{ card.duration }}</span>
        </div>
        <!-- Watermark kind icon (reference design): the glyph carries the
             event category at low opacity in the corner; its COLOR is the
             kind's tag type on the graphic tier — the same glyph/color
             split as the /alerts timeline nodes. Decorative: aria-hidden. -->
        <el-icon class="watermark-icon" :class="`icon--${card.chip.tone}`" aria-hidden="true">
          <component :is="iconOf(card.ev.kind)" />
        </el-icon>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
// RecentEvents (GH #132, UI v2 O5; card rebuilt to the reference design in
// GH #138): the dashboard's 近期事件 section — the newest alert events as a
// horizontal card row plus a 查看全部事件 link to the /alerts timeline.
// Self-built signature surface (light-container cards). Card layout per the
// reference: time top-left, chip top-right, md/600 title, two muted meta
// lines, and a large low-opacity kind icon watermarking the bottom-right
// corner. The chip is double-track: color = the kind's tag type, word =
// incident state (openers) or kind word (point-in-time). The kind
// vocabulary and tag colors come from utils/alertKind.ts untouched;
// incident duration pairing reuses utils/alertTimeline.ts FIFO; every card
// derivation lives in utils/recentEvents.ts.
import { computed, ref, watch } from 'vue'
import {
  Bell,
  CircleCheck,
  CircleClose,
  Clock,
  Finished,
  MuteNotification,
  Notification,
  Promotion,
  TrendCharts,
  Warning,
} from '@element-plus/icons-vue'
import { listAlerts, type AlertEvent, type AlertKind } from '@/api/settings'
import type { OverviewEntry } from '@/api/types'
import { formatClockMinute } from '@/utils/format'
import { pairIncidentDurations } from '@/utils/alertTimeline'
import {
  RECENT_EVENTS_CARD_LIMIT,
  RECENT_EVENTS_FETCH_LIMIT,
  buildEndpointModelMap,
  eventChip,
  eventTitle,
  impactText,
  incidentDurationText,
  selectRecentEvents,
} from '@/utils/recentEvents'

const props = defineProps<{
  // Session state owned by the parent (DashboardView's fetchAuthStatus);
  // false renders nothing and blocks every fetch.
  authed: boolean
  // Overview entries the dashboard already holds — the endpoint→model
  // resolution map is built from them, no second overview request.
  entries: OverviewEntry[]
  // Overview poll tick (generated_at): the parent bumps it on every
  // successful 10s overview refresh and the section refetches along — no
  // second poll timer exists (ticket: no new polling).
  refreshTick: string | null
}>()

// Kind icon glyphs (the color comes from the kind's tag type, not from the
// glyph). Line icons per the v2 iconography discipline.
const KIND_ICONS: Record<AlertKind, unknown> = {
  down: CircleClose,
  recovered: CircleCheck,
  group_down: Warning,
  group_recovered: CircleCheck,
  score_drop: TrendCharts,
  score_drop_skipped: TrendCharts,
  test: Promotion,
  batch: Notification,
  quiet_summary: MuteNotification,
  retire_pending: Clock,
  retired: Finished,
}

// Unknown future kinds fall back to the generic bell (alertKindLabel's
// 未知类型 placeholder precedent).
function iconOf(kind: string): unknown {
  return (KIND_ICONS as Record<string, unknown>)[kind] ?? Bell
}

const events = ref<AlertEvent[]>([])
const loading = ref(false)
const error = ref('')
let inFlight = false

async function reload() {
  // The boundary guard: no session, no request — even if a watcher fires.
  if (!props.authed || inFlight) return
  inFlight = true
  loading.value = true
  try {
    events.value = await listAlerts(RECENT_EVENTS_FETCH_LIMIT)
    error.value = ''
  } catch (err) {
    // A failed refresh keeps the last good cards on screen. A 401 (session
    // lost mid-viewing) already redirects to /login inside the http client
    // — the same behavior as the sidebar batch poll.
    error.value = (err as Error).message
  } finally {
    inFlight = false
    loading.value = false
  }
}

// Fetch on mount (immediate) when already authenticated, on the auth check
// resolving, and on every overview tick — the guard inside reload() makes
// the anonymous case a no-op.
watch(
  () => [props.authed, props.refreshTick] as const,
  () => {
    void reload()
  },
  { immediate: true },
)

// First load only: skeletons show until the first response lands; a failed
// refresh with cards on screen keeps them (dashboard list precedent).
const showSkeleton = computed(
  () => loading.value && events.value.length === 0 && error.value === '',
)

const modelMap = computed(() => buildEndpointModelMap(props.entries))

// Pairing runs on the FULL fetched window so the four-card display subset
// can never change a duration (alertTimeline discipline).
const durations = computed(() => pairIncidentDurations(events.value))

const cards = computed(() => selectRecentEvents(events.value))

// Per-card view model: all derivations are the utils/recentEvents pure
// functions; `now` is read once per recompute so the 已持续 wording stays
// consistent within a render.
const cardModels = computed(() => {
  const now = new Date()
  return cards.value.map((ev) => ({
    ev,
    chip: eventChip(ev, durations.value),
    duration: incidentDurationText(ev, durations.value, now),
    impact: impactText(ev, modelMap.value),
  }))
})
</script>

<style scoped>
.recent-events {
  /* Region seam rhythm (GH #139): space-4 below the list tile on the
     gray skeleton ground. */
  margin-top: var(--hs-space-4);
}
.section-head {
  display: flex;
  align-items: baseline;
  margin-bottom: var(--hs-space-3);
}
.section-title {
  margin: 0;
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.view-all {
  margin-left: auto;
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  text-decoration: none;
}
.view-all:hover {
  color: var(--hs-brand-hover);
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--hs-space-3);
}
/* Narrow form (2026-08-01 shell drawer batch): the four-up card row stacks
   to a single column below the 1024px breakpoint; the 150px card height
   anchor is width-independent, so cards and skeletons keep their shape. */
@media (max-width: 1023px) {
  .card-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

/* Light container (DESIGN.md: white surface + 1px border + radius-lg, no
   shadow — a static container never takes a shadow). relative + hidden
   clip the corner watermark icon. The min-height is the skeleton anchor,
   shared by both: padding 16×2 + top row 20 + gap 8 + title two md lines
   42 + gap 8 + meta two xs lines 36 + meta gap 4 = 150. */
.event-card {
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 150px;
  padding: var(--hs-space-4);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
}

.card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--hs-space-2);
  min-height: 20px;
}
.event-time {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  font-variant-numeric: tabular-nums;
}
/* Chip double-track (GH #138): tone = the kind's tag type — soft ground +
   text step of that functional color (the el-tag light-effect equivalent).
   info/primary share the neutral info pair (alertKindTagType never returns
   primary today; the class pair is defensive). */
.status-chip {
  flex: none;
  padding: 1px var(--hs-space-2);
  border-radius: var(--hs-radius-sm);
  font-size: var(--hs-text-xs);
  font-weight: 600;
}
.chip--danger {
  background: var(--hs-danger-soft);
  color: var(--hs-danger-text);
}
.chip--success {
  background: var(--hs-success-soft);
  color: var(--hs-success-text);
}
.chip--warning {
  background: var(--hs-warning-soft);
  color: var(--hs-warning-text);
}
.chip--info,
.chip--primary {
  background: var(--hs-info-soft);
  color: var(--hs-info);
}

.event-title {
  margin: var(--hs-space-2) 0 0;
  /* Two md lines at 1.5 line-height — the fixed minimum keeps every card
     in the row the same height. */
  min-height: 42px;
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.event-meta {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
  margin-top: var(--hs-space-2);
  /* Reserve the watermark corner: 44px icon + space-3 offset + space-2
     clearance, so a long impact line ellipsizes before sliding under the
     glyph. */
  padding-right: calc(44px + var(--hs-space-3) + var(--hs-space-2));
}
.meta-line {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Watermark kind icon (reference design): ~44px in the bottom-right corner
   at watermark opacity, colored on the graphic tier of the kind's tag
   type. Purely decorative — the chip already carries the kind word.
   Sized via font-size, NOT width/height: EP's `.el-icon svg` renders at
   1em, so container dimensions would only inflate the box and leave the
   glyph at 16px (GH #138 check HIGH-1). */
.watermark-icon {
  position: absolute;
  right: var(--hs-space-3);
  bottom: var(--hs-space-3);
  font-size: 44px;
  opacity: 0.18;
  pointer-events: none;
}
.icon--danger {
  color: var(--hs-danger);
}
.icon--success {
  color: var(--hs-success);
}
.icon--warning {
  color: var(--hs-warning);
}
.icon--info,
.icon--primary {
  color: var(--hs-info);
}

/* Empty / error panel: the same light-container syntax as the cards. */
.state-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--hs-space-2);
  padding: var(--hs-space-6);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
}
.state-text {
  margin: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.state-error {
  color: var(--hs-danger-text);
}

/* Skeleton bars (static gray — no pulse). The top row mirrors the real
   card: time bar left, chip bar right. */
.skeleton-card {
  gap: var(--hs-space-2);
}
.sk-toprow {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 20px;
}
.skeleton-bar {
  display: block;
  height: 12px;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
}
.sk-time {
  width: 32%;
  height: 12px;
}
.sk-chip {
  width: 52px;
  height: 18px;
}
.sk-title {
  width: 85%;
  margin-top: var(--hs-space-1);
}
.sk-meta {
  width: 55%;
}
</style>
