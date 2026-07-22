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
import StatusBadge from '@/components/StatusBadge.vue'
import TimeSeriesChart from '@/components/TimeSeriesChart.vue'
import ProbeRecordTable from '@/components/ProbeRecordTable.vue'
import { formatBucketTime } from '@/utils/format'
import type { EndpointDetail, ProbeRecord, SeriesBucket, SeriesStreaming } from '@/api/types'

// Endpoint detail page: status header plus latency/TTFT/success-rate charts
// over hourly buckets, and the recent-failures evidence table.
const route = useRoute()
const endpointId = Number(route.params.id)

const detail = ref<EndpointDetail | null>(null)
const buckets = ref<SeriesBucket[]>([])
const failures = ref<ProbeRecord[]>([])
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
