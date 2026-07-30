<template>
  <!-- Endpoint quick-view dialog (ui-guidelines §5, dashboard surface brief):
     a light in-place glance opened by clicking a dashboard card. First frame
     renders entirely from the entry snapshot frozen at open time; the latency
     curve and recent-failures sections load async (one shot at open, frozen).
     The custom hs-quickview quiet-entrance transition and the frosted
     hs-quickview-overlay live in ep-theme.css (global block — teleported EP
     shells carry no scope id); align-center centers the dialog (EP
     per-instance prop, this dialog only). -->
  <el-dialog
    :model-value="visible"
    width="640px"
    class="hs-quickview-dialog"
    modal-class="hs-quickview-overlay"
    transition="hs-quickview"
    align-center
    @update:model-value="emit('update:visible', $event)"
    @open="onOpen"
    @close="emit('close')"
    @closed="emit('closed')"
  >
    <template #header>
      <!-- Accessible name = model id (ui-guidelines §5); truncated with
           hover-to-reveal per the long-text rule. -->
      <span class="qv-title" :title="entry?.model_id">{{ entry?.model_id }}</span>
    </template>

    <div v-if="entry" class="qv-body">
      <div class="qv-status-row">
        <StatusBadge :status="entry.status" :causes="entry.degrade_causes" size="md" />
        <el-tag :type="protocolTagType(entry.protocol)" size="small">{{ entry.protocol }}</el-tag>
        <el-tag v-if="!entry.enabled" type="info" size="small">已停用</el-tag>
      </div>

      <div class="qv-metrics">
        <div class="metric">
          <span class="metric-label">24h 成功率</span>
          <span class="metric-value">{{ formatPercent(entry.success_rate_24h) }}</span>
        </div>
        <div class="metric">
          <span class="metric-label">P50</span>
          <span class="metric-value metric-p50">{{ formatMs(entry.p50_ms) }}</span>
        </div>
        <div class="metric">
          <span class="metric-label">P95</span>
          <span class="metric-value metric-p95">{{ formatMs(entry.p95_ms) }}</span>
        </div>
      </div>

      <EndpointUptimePanel :dots="entry.dots_24h" :baseline-ms="entry.baseline_p50_ms" />

      <!-- 24h latency detail curve (2026-07-30): per-probe resolution below
           the hourly UptimePanel — the curve gives the time-series shape, the
           failures table gives record detail (mutual corroboration, same
           philosophy as dot-cell/tooltip). Async like the failures region:
           one shot at open, frozen with the dialog, skeleton / error+retry /
           data tri-state, self-contained. -->
      <div class="qv-curve">
        <div class="qv-section-title">24h 延迟明细</div>
        <div v-if="curveLoading" class="qv-curve-body">
          <el-skeleton animated>
            <template #template>
              <el-skeleton-item variant="rect" class="qv-curve-skeleton" />
            </template>
          </el-skeleton>
        </div>
        <div v-else-if="curveError" class="qv-curve-body qv-error-body">
          <el-alert :title="`延迟明细加载失败:${curveError}`" type="error" :closable="false" />
          <el-button size="small" @click="loadCurve">重试</el-button>
        </div>
        <ProbeLatencyChart v-else :records="curveRecords" />
      </div>

      <!-- Recent failures: an async region. Its three states are
           self-contained — a failure here never pollutes the frozen body. -->
      <div class="qv-failures">
        <div class="qv-section-title">最近失败</div>
        <div v-if="failuresLoading" class="qv-failures-body">
          <el-skeleton animated>
            <template #template>
              <el-skeleton-item variant="rect" class="qv-failures-skeleton" />
            </template>
          </el-skeleton>
        </div>
        <div v-else-if="failuresError" class="qv-failures-body qv-error-body">
          <el-alert :title="`最近失败加载失败:${failuresError}`" type="error" :closable="false" />
          <el-button size="small" @click="loadFailures">重试</el-button>
        </div>
        <!-- slim (2026-07-29 main 裁决): trim the five detail-page material
             columns so the table fits the 640px dialog without horizontal
             scroll (§4); empty wording names failures, not probes — this
             region only queries ok=false. The fixed table height keeps the
             async region exactly as tall as its skeleton (surface brief
             async 区定高 — no align-center gravity jump); 5 rows fit inside
             without scrolling. -->
        <ProbeRecordTable
          v-else
          :records="failures"
          :compact="false"
          slim
          empty-text="暂无失败记录"
          height="280"
        />
      </div>
    </div>

    <template #footer>
      <!-- Deep-link drill-down replaces the old whole-card router.push; the
           detail page itself is unchanged. -->
      <el-button type="primary" @click="goDetail">打开完整详情</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import type { OverviewEntry, ProbeRecord } from '@/api/types'
import StatusBadge from './StatusBadge.vue'
import EndpointUptimePanel from './EndpointUptimePanel.vue'
import ProbeLatencyChart from './ProbeLatencyChart.vue'
import ProbeRecordTable from './ProbeRecordTable.vue'
import { listProbeHistory } from '@/api/probes'
import { formatPercent, formatMs } from '@/utils/format'
import { protocolTagType } from '@/utils/protocol'

const props = defineProps<{ visible: boolean; entry: OverviewEntry | null }>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  // close fires when the leave transition starts; closed fires after it (the
  // dashboard's single reset point — it returns focus to the trigger card).
  (e: 'close'): void
  (e: 'closed'): void
}>()

const router = useRouter()

// Latency-detail curve (2026-07-30): one-shot 24h window fetch at open
// (hours=24, server row cap 2000 — snapshot discipline extends to the async
// regions: fetched once, frozen, never polled). Same tri-state and stale-seq
// guard as the failures region below.
const curveRecords = ref<ProbeRecord[]>([])
const curveLoading = ref(false)
const curveError = ref<string | null>(null)
let curveSeq = 0

// Recent-failures section (surface brief): async top-5 failed probes,
// skeleton / error+retry / data tri-state. A monotonic sequence number
// discards stale responses when the dialog is reopened for another endpoint.
const failures = ref<ProbeRecord[]>([])
const failuresLoading = ref(false)
const failuresError = ref<string | null>(null)
let failuresSeq = 0

function onOpen() {
  curveRecords.value = []
  curveError.value = null
  failures.value = []
  failuresError.value = null
  loadCurve()
  loadFailures()
}

async function loadCurve() {
  const entry = props.entry
  if (!entry) return
  const seq = ++curveSeq
  curveLoading.value = true
  curveError.value = null
  try {
    const records = await listProbeHistory(entry.endpoint_id, 50, undefined, 24)
    if (seq === curveSeq) curveRecords.value = records
  } catch (e) {
    if (seq === curveSeq) curveError.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (seq === curveSeq) curveLoading.value = false
  }
}

async function loadFailures() {
  const entry = props.entry
  if (!entry) return
  const seq = ++failuresSeq
  failuresLoading.value = true
  failuresError.value = null
  try {
    const records = await listProbeHistory(entry.endpoint_id, 5, false)
    if (seq === failuresSeq) failures.value = records
  } catch (e) {
    if (seq === failuresSeq) failuresError.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (seq === failuresSeq) failuresLoading.value = false
  }
}

function goDetail() {
  if (!props.entry) return
  emit('update:visible', false)
  router.push(`/endpoints/${props.entry.endpoint_id}`)
}
</script>

<style scoped>
.qv-title {
  /* Long model ids truncate with hover-to-reveal (the title attr above);
     scoped styles reach slot content even though the dialog is teleported. */
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.qv-status-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--hs-space-2);
  margin-bottom: var(--hs-space-3);
}
.qv-metrics {
  display: flex;
  gap: var(--hs-space-4);
  margin-bottom: var(--hs-space-3);
}
.metric {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.metric-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.metric-value {
  font-size: var(--hs-text-md);
  line-height: 1.2;
  color: var(--hs-text-primary);
  /* 2px in the card original; the scale has no 2px stop, so the nearest
     4px tier (visual difference negligible). */
  margin-top: var(--hs-space-1);
}
/* Same metric hierarchy as the card (GH #54): P50 primary, P95 secondary. */
.metric-p50 {
  font-size: var(--hs-text-lg);
  font-weight: 600;
}
.metric-p95 {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.qv-curve {
  margin-top: var(--hs-space-3);
}
.qv-failures {
  margin-top: var(--hs-space-3);
}
.qv-section-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-regular);
  margin-bottom: var(--hs-space-2);
}
/* Async regions keep skeleton === terminal height (surface brief async 区定高):
   under align-center a late height change would jump the dialog mid-entrance.
   The 180px curve twin is ProbeLatencyChart's default height; the 280px
   failures twin is the el-table height prop — change all copies together. */
.qv-curve-skeleton {
  height: 180px;
}
.qv-failures-skeleton {
  height: 280px;
}
.qv-error-body {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: var(--hs-space-2);
}
.qv-curve-body.qv-error-body {
  height: 180px;
}
.qv-failures-body.qv-error-body {
  height: 280px;
}
</style>
