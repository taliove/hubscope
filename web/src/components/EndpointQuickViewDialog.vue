<template>
  <!-- Endpoint quick-view dialog (ui-guidelines §5, dashboard surface brief):
     a light in-place glance opened by clicking a dashboard card. First frame
     renders entirely from the entry snapshot frozen at open time; only the
     recent-failures section loads async. The custom hs-flip transition and
     the frosted hs-quickview-overlay live in ep-theme.css (global block —
     teleported EP shells carry no scope id). -->
  <el-dialog
    :model-value="visible"
    width="640px"
    class="hs-quickview-dialog"
    modal-class="hs-quickview-overlay"
    transition="hs-flip"
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

      <!-- Recent failures: the only async region. Its three states are
           self-contained — a failure here never pollutes the frozen body. -->
      <div class="qv-failures">
        <div class="qv-failures-title">最近失败</div>
        <el-skeleton v-if="failuresLoading" :rows="3" animated />
        <div v-else-if="failuresError" class="qv-failures-error">
          <el-alert :title="`最近失败加载失败:${failuresError}`" type="error" :closable="false" />
          <el-button size="small" @click="loadFailures">重试</el-button>
        </div>
        <!-- slim (2026-07-29 main 裁决): trim the five detail-page material
             columns so the table fits the 640px dialog without horizontal
             scroll (§4); empty wording names failures, not probes — this
             region only queries ok=false. -->
        <ProbeRecordTable v-else :records="failures" :compact="false" slim empty-text="暂无失败记录" />
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
import ProbeRecordTable from './ProbeRecordTable.vue'
import { listProbeHistory } from '@/api/probes'
import { formatPercent, formatMs } from '@/utils/format'
import { protocolTagType } from '@/utils/protocol'

const props = defineProps<{ visible: boolean; entry: OverviewEntry | null }>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  // close fires when the leave transition starts (the dashboard schedules the
  // card flip-back overlap here); closed fires after it (the single reset
  // point for the dashboard's flip state).
  (e: 'close'): void
  (e: 'closed'): void
}>()

const router = useRouter()

// Recent-failures section (surface brief): async top-5 failed probes,
// skeleton / error+retry / data tri-state. A monotonic sequence number
// discards stale responses when the dialog is reopened for another endpoint.
const failures = ref<ProbeRecord[]>([])
const failuresLoading = ref(false)
const failuresError = ref<string | null>(null)
let failuresSeq = 0

function onOpen() {
  failures.value = []
  failuresError.value = null
  loadFailures()
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
.qv-failures {
  margin-top: var(--hs-space-1);
}
.qv-failures-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-regular);
  margin-bottom: var(--hs-space-2);
}
.qv-failures-error {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--hs-space-2);
}
</style>
