<template>
  <div class="detail-page">
    <header class="detail-header">
      <template v-if="detail">
        <h1 class="model-title" :title="detail.model_id_str">{{ detail.model_id_str }}</h1>
        <el-tag :type="protocolTagType(detail.protocol)" size="small">
          {{ detail.protocol }}
        </el-tag>
        <span class="hub-name">Hub:{{ detail.hub_name }}</span>
        <el-button v-if="authed" class="share-btn" @click="shareModel">
          <el-icon><Share /></el-icon>
          分享
        </el-button>
      </template>
      <el-skeleton v-else-if="!error" class="header-skeleton" :rows="1" animated />
    </header>
    <!-- Status row mirrors the Dashboard card: StatusBadge first, then tags,
         then the reason text (same colors, same words, ui-guidelines §3). -->
    <div v-if="detail" class="status-row">
      <StatusBadge :status="detail.status" :reason="detail.status_reason" />
      <el-tag v-if="!detail.enabled" type="info" size="small">已停用</el-tag>
      <span v-if="detail.status_reason" class="status-reason" :title="detail.status_reason">
        {{ detail.status_reason }}
      </span>
    </div>

    <!-- Metrics cards: fixed 24h availability KPI (GH #56 — never drifts with
         the window controls below) and eval score (from latest campaign). -->
    <div v-if="detail" class="metrics-row">
      <el-card shadow="never" class="metric-card">
        <div class="metric-label">24h 可用率</div>
        <div class="metric-value" :class="`score-${availabilityTierName}`">
          {{ formatPercent(availability24h) }}
        </div>
        <div v-if="availability24h === null" class="metric-note">24h 内无探测数据</div>
      </el-card>

      <el-card shadow="never" class="metric-card">
        <el-skeleton v-if="evalLoading" :rows="2" animated />
        <template v-else-if="evalError">
          <!-- Failure is not the empty state: neutral secondary text (the eval
               card is auxiliary — danger red would read as an endpoint
               incident) plus a retry link. -->
          <div class="metric-label eval-error">
            <span class="eval-error-text" :title="evalError">评估数据加载失败:{{ evalError }}</span>
            <el-button link type="primary" size="small" @click="loadEvalSummary">重试</el-button>
          </div>
        </template>
        <template v-else-if="evalSummary">
          <div class="metric-label">评估总分</div>
          <div class="metric-value">{{ formatScore(evalSummary.total_score) }}</div>
          <div class="suite-tags">
            <el-tag v-for="suite in evalSummary.suite_scores" :key="suite.suite_id" size="small">
              {{ suite.suite_name }} {{ formatScore(suite.score) }}
            </el-tag>
          </div>
          <div class="eval-time">评估于 {{ formatTime(evalSummary.campaign_created_at) }}</div>
          <!-- GH #156: straight to the batch this score came from. /eval is
               session-gated, so anonymous visitors bounce to /login — same
               caliber as the header's batch entry (issue #16). -->
          <router-link :to="`/eval?batch=${evalSummary.campaign_id}`" class="eval-link">
            查看评估榜单 →
          </router-link>
        </template>
        <template v-else>
          <div class="metric-label empty">暂无评估数据</div>
        </template>
      </el-card>
    </div>
    <div v-else-if="!error" class="metrics-row">
      <el-card v-for="i in 2" :key="i" shadow="never" class="metric-card">
        <el-skeleton :rows="2" animated />
      </el-card>
    </div>

    <!-- Window and streaming selectors drive the three charts below. -->
    <div class="controls">
      <div class="controls-left">
        <el-radio-group v-model="hours" @change="reloadSeries">
          <el-radio-button :value="24">24 小时</el-radio-button>
          <el-radio-button :value="168">7 天</el-radio-button>
          <el-radio-button :value="720">30 天</el-radio-button>
        </el-radio-group>
        <el-radio-group v-model="mode" @change="reloadSeries">
          <el-radio-button value="all">合并</el-radio-button>
          <el-radio-button value="streaming">流式</el-radio-button>
          <el-radio-button value="non_streaming">非流式</el-radio-button>
        </el-radio-group>
      </div>
      <el-button v-if="authed && detail" type="primary" @click="triggerEval">评估此模型</el-button>
    </div>

    <el-alert v-if="error" type="error" :closable="false" class="error-alert">
      <template #title>加载失败:{{ error }}</template>
      <!-- Retry reruns the whole initial load chain (detail + failures +
           overview + eval + series): the error ref is shared by the first
           load and reloadSeries, and the full-chain rerun covers both. -->
      <el-button size="small" @click="loadInitial">重试</el-button>
    </el-alert>

    <div v-loading="loading">
      <TimeSeriesChart title="延迟(P50 / P95)" :categories="categories" :series="latencySeries" y-name="ms" />
      <!-- TTFT only exists for streaming probes; hide it in non-streaming mode. -->
      <TimeSeriesChart
        v-if="mode !== 'non_streaming'"
        title="TTFT(首 token 延迟,仅流式有意义)"
        :categories="categories"
        :series="ttftSeries"
        y-name="ms"
      />
      <TimeSeriesChart title="成功率" :categories="categories" :series="successSeries" y-name="%" />
      <el-empty v-if="!loading && buckets.length === 0" description="该时间范围内暂无数据" />
    </div>

    <el-card shadow="never" class="failures-card">
      <div class="failures-title">近期失败(最近 20 条)</div>
      <ProbeRecordTable :records="failures" :compact="false" />
    </el-card>

    <EvalTriggerDialog
      v-model="evalDialogVisible"
      :suites="suites"
      :models="models"
      :preselected-model-id="detail?.model_id"
      @triggered="onEvalTriggered"
    />

    <StatusShareDialog v-model:visible="shareVisible" :snapshot="shareSnapshot" />

    <!-- Quiet admin entry (ticket 90): the shared PublicFooter. -->
    <PublicFooter />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Share } from '@element-plus/icons-vue'
import { getEndpointDetail, getEndpointSeries } from '@/api/endpoints'
import { listProbeHistory } from '@/api/probes'
import { getModelEvalSummary, listSuites } from '@/api/evals'
import { listModels } from '@/api/models'
import { fetchOverview } from '@/api/overview'
import { fetchAuthStatus } from '@/api/auth'
import StatusBadge from '@/components/StatusBadge.vue'
import TimeSeriesChart from '@/components/TimeSeriesChart.vue'
import ProbeRecordTable from '@/components/ProbeRecordTable.vue'
import EvalTriggerDialog from '@/components/EvalTriggerDialog.vue'
import PublicFooter from '@/components/PublicFooter.vue'
import StatusShareDialog from '@/components/StatusShareDialog.vue'
import { formatBucketTime, formatPercent, formatScore, formatTime } from '@/utils/format'
import { protocolTagType } from '@/utils/protocol'
import { availabilityTier, endpointAvailability24h, type AvailabilityTier } from '@/utils/statusCardSummary'
import { createSingleModelSnapshot, type StatusCardSnapshot } from '@/utils/statusCardSnapshot'
import type { EndpointDetail, ProbeRecord, SeriesBucket, SeriesStreaming, ModelEvalSummary, Suite, Model, Campaign, OverviewEntry } from '@/api/types'

// Endpoint detail page: status header plus latency/TTFT/success-rate charts
// over hourly buckets, and the recent-failures evidence table.
const route = useRoute()
const endpointId = Number(route.params.id)

const detail = ref<EndpointDetail | null>(null)
const buckets = ref<SeriesBucket[]>([])
const failures = ref<ProbeRecord[]>([])
const evalSummary = ref<ModelEvalSummary | null>(null)
const evalLoading = ref(false)
const evalError = ref('')
// Overview entry for this endpoint, fetched once at mount (anonymous-safe);
// drives the fixed 24h availability KPI. Undefined when the entry is not
// visible (hub-scoped session viewing a foreign hub's endpoint).
const overviewEntry = ref<OverviewEntry | undefined>(undefined)
const suites = ref<Suite[]>([])
const models = ref<Model[]>([])
const evalDialogVisible = ref(false)
const shareVisible = ref(false)
const shareSnapshot = ref<StatusCardSnapshot | null>(null)
const hours = ref(24)
const mode = ref<SeriesStreaming>('all')
const loading = ref(false)
const error = ref('')

// Session state gates the eval/share buttons: they are management actions
// that require a login, while probe and eval data stay public.
const authed = ref(false)
async function refreshAuth() {
  try {
    authed.value = (await fetchAuthStatus()).authenticated
  } catch {
    authed.value = false
  }
}

const categories = computed(() => buckets.value.map(b => formatBucketTime(b.bucket_start)))

const latencySeries = computed(() => [
  { name: 'P50', data: buckets.value.map(b => b.p50_ms) },
  { name: 'P95', data: buckets.value.map(b => b.p95_ms) },
])

const ttftSeries = computed(() => [
  { name: '平均 TTFT', data: buckets.value.map(b => b.avg_ttft_ms) },
])

// Per-bucket success rate as a 0~100 percentage.
const successSeries = computed(() => [
  {
    name: '成功率',
    data: buckets.value.map(b =>
      b.total > 0 ? Math.round(((b.total - b.failures) / b.total) * 1000) / 10 : null
    ),
  },
])

// Fixed 24h availability KPI (GH #56): computed from the mount-time overview
// entry's dots_24h via the batch-59 probe-weighted pure function — identical
// to the backend figure by construction and NEVER drifting with the window/
// mode controls below (those only drive the charts). Null renders "-" plus a
// neutral no-data note; no fallback to the chart buckets (second data path).
const availability24h = computed(() => endpointAvailability24h(overviewEntry.value))

const availabilityTierName = computed((): AvailabilityTier => availabilityTier(availability24h.value))

async function reloadSeries() {
  loading.value = true
  error.value = ''
  try {
    buckets.value = await getEndpointSeries(endpointId, hours.value, mode.value)
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

// Eval summary failure is NOT the empty state (GH #56): a failed load keeps
// evalSummary null but records the reason in evalError, so the card renders
// "评估数据加载失败 · 重试" instead of masquerading as "暂无评估数据".
async function loadEvalSummary() {
  if (!detail.value) return
  evalLoading.value = true
  evalError.value = ''
  try {
    evalSummary.value = await getModelEvalSummary(detail.value.model_id)
  } catch (e) {
    evalSummary.value = null
    evalError.value = e instanceof Error ? e.message : String(e)
  } finally {
    evalLoading.value = false
  }
}

function triggerEval() {
  evalDialogVisible.value = true
}

function onEvalTriggered(campaign: Campaign) {
  ElMessage.success(`评估已触发,批次 #${campaign.id}`)
  // Refresh eval summary to show the latest data
  loadEvalSummary()
}

async function shareModel() {
  if (!detail.value) return
  try {
    // Fetch the latest overview to get the OverviewEntry for this endpoint
    const overview = await fetchOverview()
    const entry = overview.endpoints.find(e => e.endpoint_id === endpointId)
    if (!entry) {
      ElMessage.error('无法找到该端点的状态数据')
      return
    }
    shareSnapshot.value = createSingleModelSnapshot(
      entry,
      detail.value.hub_name,
      evalSummary.value,
      new Date().toISOString()
    )
    shareVisible.value = true
  } catch (e) {
    ElMessage.error(`加载分享数据失败: ${e instanceof Error ? e.message : String(e)}`)
  }
}

// Whole initial load chain (GH #56): detail + failures + overview + eval +
// series. The error alert's retry button reruns this, so a retry covers both
// error sources (first load and series reload) with one path.
async function loadInitial() {
  error.value = ''
  try {
    const [d, f, overview] = await Promise.all([
      getEndpointDetail(endpointId),
      listProbeHistory(endpointId, 20, false),
      // Anonymous-safe; drives the fixed 24h availability KPI.
      fetchOverview(),
    ])
    detail.value = d
    failures.value = f
    overviewEntry.value = overview.endpoints.find(e => e.endpoint_id === endpointId)

    // Load evaluation summary for this model (ticket 60.3)
    await loadEvalSummary()

    // Load suites and models for the eval trigger dialog (ticket 60.4);
    // skipped for anonymous visitors since the eval button stays hidden.
    if (authed.value) {
      try {
        const [suitesData, modelsData] = await Promise.all([
          listSuites(),
          listModels(),
        ])
        suites.value = suitesData
        models.value = modelsData
      } catch (e) {
        console.warn('Failed to load eval trigger data:', e)
      }
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
  await reloadSeries()
}

onMounted(async () => {
  await refreshAuth()
  await loadInitial()
})
</script>

<style scoped>
.detail-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 32px;
}
.detail-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.model-title {
  margin: 0;
  font-size: var(--hs-text-xl);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hub-name {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.status-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.status-reason {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.metrics-row {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  margin-top: 16px;
}
.metric-card {
  --el-card-padding: 16px;
}
.metric-label {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin-bottom: 8px;
}
.metric-label.empty {
  text-align: center;
  font-size: var(--hs-text-md);
  color: var(--hs-text-placeholder);
  margin-bottom: 0;
}
.metric-value {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 8px;
}
/* Availability tier colors (ui-guidelines §3) */
.metric-value.score-ok {
  color: var(--hs-success);
}
.metric-value.score-partial {
  color: var(--hs-warning);
}
.metric-value.score-fail {
  color: var(--hs-danger);
}
.metric-value.score-none {
  color: var(--hs-text-placeholder);
}
.metric-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
/* Eval-load failure: neutral secondary (auxiliary card — danger red would
   read as an endpoint incident, ui-guidelines §3 semantic domain). */
.eval-error {
  margin-bottom: 0;
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
}
/* Long backend messages truncate with title hover (ui-guidelines §6), same
   pattern as .status-reason. */
.eval-error-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.header-skeleton {
  flex: 1;
  min-width: 0;
}
.suite-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}
.eval-time {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.eval-link {
  font-size: var(--hs-text-xs);
  color: var(--hs-brand);
  text-decoration: none;
}
.controls {
  display: flex;
  gap: 16px;
  margin: 16px 0;
  flex-wrap: wrap;
}
.error-alert {
  margin-bottom: 16px;
}
.failures-card {
  margin-top: 8px;
}
/* Public status board density: 16px card padding (ui-guidelines §2). */
.failures-card {
  --el-card-padding: 16px;
}
.failures-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 8px;
}
</style>
