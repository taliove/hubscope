<template>
  <!-- Model detail side panel (GH #116, spec 0018 §10): the signature
       triage surface that replaces the retired quick-view dialog AND the
       T5 interim deep-link click. A self-built right-edge panel (decision 8:
       signature surfaces are self-built, el-dialog is NOT used here) —
       scrim + full-height sheet, five regions top to bottom: model name +
       current status / Availability · Latency · Error Rate / 24h latency
       trend / failure event records / 「打开完整详情」.

       Freeze discipline (spec 0018 §10): the entry prop is a SNAPSHOT taken
       at row-click time — the overview 10s poll never refreshes an open
       panel (live data lives one click away on the full detail page). The
       probe fetch (latency chart + event records share ONE 24h window
       fetch) runs once per open; reopening re-fetches. Real-time is not
       the panel's job — locating the cause is.

       First self-built modal surface of the repo, so the modal trio
       precedent lives here: focus trap (utils/focusTrap.ts — Tab cycles
       inside the sheet, el-dialog parity) + unified ESC/scrim/button close
       + focus return to the trigger row (parent). The rebuilt share
       dialogs will reuse the same trio. -->
  <Teleport to="body">
    <Transition name="panel-fade">
      <div v-if="entry" class="panel-scrim" @click="emit('close')" />
    </Transition>
    <Transition name="panel-slide" @after-enter="focusClose">
      <aside
        v-if="entry"
        ref="panelEl"
        class="detail-panel"
        role="dialog"
        aria-modal="true"
        :aria-label="entry.model_id"
      >
        <header class="panel-header">
          <div class="panel-heading">
            <h2 class="panel-title" :title="entry.model_id">{{ entry.model_id }}</h2>
            <div class="panel-status-row">
              <StatusBadge
                :status="entry.status"
                :causes="entry.degrade_causes"
                :reason="entry.status_reason"
                size="md"
              />
              <span v-if="!entry.enabled" class="disabled-note">已停用</span>
            </div>
            <p v-if="entry.status_reason" class="panel-reason" :title="entry.status_reason">
              {{ entry.status_reason }}
            </p>
          </div>
          <button ref="closeBtn" type="button" class="close-btn" aria-label="关闭详情面板" @click="emit('close')">
            ×
          </button>
        </header>

        <!-- Metric cells: every scalar derives from the frozen snapshot
             (panelMetrics) — null renders a dash, never an invented 0%. -->
        <section class="panel-metrics">
          <div class="metric-cell">
            <span class="metric-label">24h 可用率</span>
            <span class="metric-value" :class="`tier-${availabilityRateTier(metrics.availability)}`">
              {{ formatPercent(metrics.availability) }}
            </span>
          </div>
          <div class="metric-cell">
            <span class="metric-label">平均延迟</span>
            <span class="metric-value ink">{{ formatMs(metrics.latencyP50Ms) }}</span>
            <span class="metric-sub">P95 {{ formatMs(metrics.latencyP95Ms) }}</span>
          </div>
          <div class="metric-cell">
            <span class="metric-label">错误率</span>
            <span class="metric-value" :class="`tier-${errorRateTier}`">
              {{ formatPercent(metrics.errorRate) }}
            </span>
          </div>
        </section>

        <!-- The async region: ONE 24h probe fetch feeds both the latency
             chart and the event records, so loading/error surface once and
             never poison the snapshot-driven header above. -->
        <section class="panel-section">
          <h3 class="section-title">24h 延迟趋势</h3>
          <div v-if="probesLoading" class="region-skeleton" />
          <div v-else-if="probesError" class="region-error">
            <span class="region-error-text" :title="probesError">加载失败：{{ probesError }}</span>
            <button type="button" class="retry-btn" @click="loadProbes">重试</button>
          </div>
          <ProbeLatencyChart v-else :records="probes" height="180px" />
        </section>

        <section class="panel-section">
          <h3 class="section-title">事件记录</h3>
          <div v-if="probesLoading" class="region-skeleton slim" />
          <div v-else-if="probesError" class="region-error">
            <span class="region-error-text">探测数据不可用，事件记录暂缺</span>
            <button type="button" class="retry-btn" @click="loadProbes">重试</button>
          </div>
          <ul v-else-if="events.length > 0" class="event-list">
            <li v-for="event in events" :key="event.id" class="event-row">
              <span class="event-time">{{ formatTime(event.createdAt) }}</span>
              <span class="event-stream">{{ event.streaming ? '流式' : '非流式' }}</span>
              <span class="event-reason" :title="event.reason">{{ event.reason }}</span>
            </li>
          </ul>
          <p v-else class="empty-note">24h 内无失败记录</p>
        </section>

        <footer class="panel-footer">
          <el-button type="primary" class="detail-btn" @click="openFullDetail">打开完整详情</el-button>
        </footer>
      </aside>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import StatusBadge from '@/components/StatusBadge.vue'
import ProbeLatencyChart from '@/components/ProbeLatencyChart.vue'
import { listProbeHistory } from '@/api/probes'
import { formatMs, formatPercent, formatTime } from '@/utils/format'
import { availabilityRateTier } from '@/utils/overviewMetrics'
import { panelMetrics, recentFailureEvents } from '@/utils/modelDetailPanel'
import { createFocusTrap, type FocusTrapHandle } from '@/utils/focusTrap'
import type { OverviewEntry, ProbeRecord } from '@/api/types'

const props = defineProps<{ entry: OverviewEntry | null }>()
const emit = defineEmits<{ close: [] }>()

const router = useRouter()

const metrics = computed(() =>
  props.entry
    ? panelMetrics(props.entry)
    : { availability: null, errorRate: null, latencyP50Ms: null, latencyP95Ms: null },
)

// Error-rate tier: reuse the availability three-tier thresholds on the
// COMPLEMENT so the two cells always agree (a 97% availability and a 3%
// error rate read the same color). A null rate tiers to none.
const errorRateTier = computed(() =>
  metrics.value.errorRate === null ? 'none' : availabilityRateTier(1 - metrics.value.errorRate),
)

// --- One-time probe fetch (latency chart + event records) -----------------

const probes = ref<ProbeRecord[]>([])
const probesLoading = ref(false)
const probesError = ref('')

const events = computed(() => recentFailureEvents(probes.value))

async function loadProbes() {
  if (!props.entry) return
  probesLoading.value = true
  probesError.value = ''
  try {
    // Window mode (hours=24, server row cap 2000): the per-probe latency
    // chart needs the full window, and the event records derive from the
    // same payload — one fetch, one failure surface.
    probes.value = await listProbeHistory(props.entry.endpoint_id, 0, undefined, 24)
  } catch (e) {
    probes.value = []
    probesError.value = e instanceof Error ? e.message : String(e)
  } finally {
    probesLoading.value = false
  }
}

// --- Open/close plumbing ----------------------------------------------------

const closeBtn = ref<HTMLButtonElement>()
const panelEl = ref<HTMLElement>()
let focusTrap: FocusTrapHandle | null = null

function focusClose() {
  closeBtn.value?.focus()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

// Fetch once per open and wire the modal trio while open (focus trap +
// ESC close here, focus return in the parent); the sheet is teleported to
// body, so its listeners are document-level. Background scroll locks while
// the panel is up (the scrim is modal). The trap installs on nextTick —
// the sheet must be in the DOM before its focusables can be queried.
watch(
  () => props.entry?.endpoint_id,
  id => {
    if (id !== undefined) {
      probes.value = []
      probesError.value = ''
      loadProbes()
      document.addEventListener('keydown', onKeydown)
      document.body.style.overflow = 'hidden'
      nextTick(() => {
        if (panelEl.value) focusTrap = createFocusTrap(panelEl.value)
      })
    } else {
      focusTrap?.deactivate()
      focusTrap = null
      document.removeEventListener('keydown', onKeydown)
      document.body.style.overflow = ''
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  focusTrap?.deactivate()
  document.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
})

// The only way out of the panel toward more data: close first, then
// deep-link (the route change unmounts the dashboard and this panel with
// it; closing first keeps the transition honest and the focus return a
// no-op once the trigger row is gone).
function openFullDetail() {
  if (!props.entry) return
  const id = props.entry.endpoint_id
  emit('close')
  nextTick(() => router.push(`/endpoints/${id}`))
}
</script>

<style scoped>
/* Scrim: the shared light overlay veil (Apple syntax — the content behind
   stays legible; the panel, not darkness, carries the focus). */
.panel-scrim {
  position: fixed;
  inset: 0;
  background: var(--hs-overlay-bg);
  z-index: var(--hs-z-overlay);
}

/* Sheet: right-edge, full height, one tier above the scrim (drawer slot of
   the z scale). Shadow expresses the floating layer (shadow semantics:
   shadows only express overlay/clickable). box-sizing: border-box is
   LOAD-BEARING here (2026-08-01 mobile fix): the repo has no global reset
   (content-box), so without it the 94vw width + 48px padding + 1px border
   made the sheet 415px wide on a 390px viewport — fixed right:0 then pushed
   the content lane 25px past the LEFT edge. */
.detail-panel {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  box-sizing: border-box;
  width: min(440px, 94vw);
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-5);
  padding: var(--hs-space-5);
  overflow-y: auto;
  background: var(--hs-bg-card);
  border-left: 1px solid var(--hs-border);
  box-shadow: var(--hs-shadow-lg);
  z-index: var(--hs-z-drawer);
}
/* Narrow form (2026-08-01 shell drawer batch): the registered 16px narrow
   container padding and a tighter region rhythm. */
@media (max-width: 1023px) {
  .detail-panel {
    gap: var(--hs-space-4);
    padding: var(--hs-space-4);
  }
}

.panel-fade-enter-active,
.panel-fade-leave-active {
  transition: opacity var(--hs-transition);
}
.panel-fade-enter-from,
.panel-fade-leave-to {
  opacity: 0;
}
.panel-slide-enter-active,
.panel-slide-leave-active {
  transition: transform var(--hs-transition);
}
.panel-slide-enter-from,
.panel-slide-leave-to {
  transform: translateX(100%);
}

.panel-header {
  display: flex;
  align-items: flex-start;
  gap: var(--hs-space-3);
}
.panel-heading {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
  /* flex-grow is what pins the close button to the sheet's right edge —
     without it the button followed the title inline (2026-08-01 mobile
     report: 「关闭按钮位置不正确」). min-width: 0 keeps the title's
     ellipsis working inside the flex row. */
  flex: 1 1 auto;
  min-width: 0;
}
.panel-title {
  margin: 0;
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.panel-status-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
}
.disabled-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.panel-reason {
  margin: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.close-btn {
  flex: none;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--hs-radius-sm);
  background: none;
  font-size: var(--hs-text-lg);
  line-height: 1;
  color: var(--hs-text-secondary);
  cursor: pointer;
}
.close-btn:hover {
  background: var(--hs-bg-hover);
  color: var(--hs-text-primary);
}
/* The sheet focuses this button on open (focusClose) — without a rule the
   UA default black ring showed (2026-08-01 mobile report). The single
   focus language applies: 2px brand outline. */
.close-btn:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}

/* Metric cells: three light wells, availability/error tiered by the shared
   three-tier thresholds (text grade), latency neutral ink — it is a cost
   reading, not a health signal. */
.panel-metrics {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--hs-space-3);
}
.metric-cell {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
  padding: var(--hs-space-3);
  background: var(--hs-bg-subtle);
  border-radius: var(--hs-radius-lg);
}
.metric-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.metric-value {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.metric-value.ink {
  color: var(--hs-text-primary);
}
.metric-value.tier-success {
  color: var(--hs-success-text);
}
.metric-value.tier-warning {
  color: var(--hs-warning-text);
}
.metric-value.tier-danger {
  color: var(--hs-danger-text);
}
.metric-value.tier-none {
  color: var(--hs-text-placeholder);
}
.metric-sub {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  font-variant-numeric: tabular-nums;
}

.panel-section {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.section-title {
  margin: 0;
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}

/* Async-region three states: the skeleton holds the chart's terminal
   height so the reveal never shifts the layout; an error stays scoped to
   the region (the snapshot header above is never poisoned). */
.region-skeleton {
  height: 180px;
  border-radius: var(--hs-radius-lg);
  background: var(--hs-bg-hover);
}
.region-skeleton.slim {
  height: 96px;
}
.region-error {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  padding: var(--hs-space-3);
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  border: 1px dashed var(--hs-border);
  border-radius: var(--hs-radius-sm);
}
.region-error-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.retry-btn {
  flex: none;
  background: none;
  border: none;
  padding: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  cursor: pointer;
}
.retry-btn:hover {
  color: var(--hs-brand-hover);
}

/* Event records: a light list (lighter than the ProbeRecordTable slim
   variant — the panel is a triage surface). Time + streaming channel +
   reason; the impact scope is constant within a single-model panel and is
   carried by the header, not repeated per row. */
.event-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
}
.event-row {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-2);
  padding: var(--hs-space-2) 0;
  border-bottom: 1px solid var(--hs-border-light);
}
.event-row:last-child {
  border-bottom: none;
}
.event-time {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  font-variant-numeric: tabular-nums;
}
.event-stream {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.event-reason {
  font-size: var(--hs-text-sm);
  color: var(--hs-danger-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.empty-note {
  margin: 0;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-placeholder);
}

.panel-footer {
  margin-top: auto;
  padding-top: var(--hs-space-3);
  border-top: 1px solid var(--hs-border-light);
}
.detail-btn {
  width: 100%;
}
</style>
