<template>
  <div class="detail-page">
    <header class="detail-header">
      <template v-if="detail">
        <h1 class="model-title" :title="detail.model_id_str">{{ detail.model_id_str }}</h1>
        <el-tag :type="detail.protocol === 'anthropic' ? 'success' : 'warning'" size="small">
          {{ detail.protocol }}
        </el-tag>
        <span class="hub-name">Hub:{{ detail.hub_name }}</span>
      </template>
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

    <!-- Metrics cards: stability score (from probes) and eval score (from latest campaign) -->
    <div v-if="detail" class="metrics-row">
      <el-card shadow="never" class="metric-card">
        <div class="metric-label">24h 稳定性</div>
        <div class="metric-value" :class="`score-${stabilityTier}`">
          {{ formatScore(stabilityScore) }}
        </div>
      </el-card>

      <el-card shadow="never" class="metric-card">
        <template v-if="evalSummary">
          <div class="metric-label">评估总分</div>
          <div class="metric-value">{{ formatScore(evalSummary.total_score) }}</div>
          <div class="suite-tags">
            <el-tag v-for="suite in evalSummary.suite_scores" :key="suite.suite_id" size="small">
              {{ suite.suite_name }} {{ formatScore(suite.score) }}
            </el-tag>
          </div>
          <div class="eval-time">评估于 {{ formatTime(evalSummary.campaign_created_at) }}</div>
        </template>
        <template v-else>
          <div class="metric-label empty">暂无评估数据</div>
        </template>
      </el-card>
    </div>

    <!-- Window and streaming selectors drive the three charts below. -->
    <div class="controls">
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

    <el-alert
      v-if="error"
      :title="`加载失败:${error}`"
      type="error"
      :closable="false"
      class="error-alert"
    />

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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getEndpointDetail, getEndpointSeries } from '@/api/endpoints'
import { listProbeHistory } from '@/api/probes'
import { getModelEvalSummary } from '@/api/evals'
import StatusBadge from '@/components/StatusBadge.vue'
import TimeSeriesChart from '@/components/TimeSeriesChart.vue'
import ProbeRecordTable from '@/components/ProbeRecordTable.vue'
import { formatBucketTime, formatScore, formatTime } from '@/utils/format'
import { availabilityTier, type AvailabilityTier } from '@/utils/statusCardSummary'
import type { EndpointDetail, ProbeRecord, SeriesBucket, SeriesStreaming, ModelEvalSummary } from '@/api/types'

// Endpoint detail page: status header plus latency/TTFT/success-rate charts
// over hourly buckets, and the recent-failures evidence table.
const route = useRoute()
const endpointId = Number(route.params.id)

const detail = ref<EndpointDetail | null>(null)
const buckets = ref<SeriesBucket[]>([])
const failures = ref<ProbeRecord[]>([])
const evalSummary = ref<ModelEvalSummary | null>(null)
const hours = ref(24)
const mode = ref<SeriesStreaming>('all')
const loading = ref(false)
const error = ref('')

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

// Compute 24h stability score from recent buckets (simplified from overview entry).
// In a full implementation this would come from the backend, but for now we
// approximate it from the success rate of loaded buckets.
const stabilityScore = computed((): number | null => {
  if (buckets.value.length === 0) return null
  let total = 0
  let failures = 0
  for (const b of buckets.value) {
    total += b.total
    failures += b.failures
  }
  if (total === 0) return null
  const rate = (total - failures) / total
  return Math.round(rate * 100)
})

const stabilityTier = computed((): AvailabilityTier => {
  if (stabilityScore.value === null) return 'none'
  return availabilityTier(stabilityScore.value / 100)
})

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

onMounted(async () => {
  try {
    const [d, f] = await Promise.all([
      getEndpointDetail(endpointId),
      listProbeHistory(endpointId, 20, false),
    ])
    detail.value = d
    failures.value = f

    // Load evaluation summary for this model (ticket 60.3)
    try {
      evalSummary.value = await getModelEvalSummary(d.model_id)
    } catch (e) {
      // Eval summary is optional; don't block the page if it fails
      console.warn('Failed to load eval summary:', e)
      evalSummary.value = null
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
  await reloadSeries()
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
  color: var(--el-color-success);
}
.metric-value.score-partial {
  color: var(--el-color-warning);
}
.metric-value.score-fail {
  color: var(--el-color-danger);
}
.metric-value.score-none {
  color: var(--hs-text-placeholder);
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
