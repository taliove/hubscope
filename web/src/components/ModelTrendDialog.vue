<template>
  <el-dialog
    :model-value="model !== null"
    width="80%"
    @update:model-value="(v: boolean) => { if (!v) $emit('close') }"
  >
    <template #header>
      <div class="dialog-title">
        <span class="title-text" :title="model?.model_id">趋势:{{ model?.model_id }}</span>
        <el-tag v-if="trends?.model.deleted" size="small" type="info">已删除</el-tag>
      </div>
    </template>

    <div v-loading="loading" class="dialog-body">
      <!-- Error state: reason plus a retry entry (ui-guidelines §6). -->
      <div v-if="error" class="load-error">
        <p class="load-error-text">加载失败:{{ error }}</p>
        <el-button size="small" @click="reload">重试</el-button>
      </div>

      <template v-else-if="trends">
        <el-empty
          v-if="trends.suites.length === 0 && trends.probe.length === 0"
          description="该模型在此批次暂无趋势数据"
        />

        <template v-else>
          <!-- Score trend: one chart per suite, version breaks marked. -->
          <section class="section">
            <div class="section-title">跨批次分数趋势</div>
            <el-empty v-if="trends.suites.length === 0" description="暂无评估分数" :image-size="60" />
            <TrendChart
              v-for="suite in trends.suites"
              :key="suite.suite_id"
              :title="suite.name"
              :categories="categoriesOf(suite)"
              :series="seriesOf(suite)"
              :mark-lines="markLinesOf(suite)"
              y-name="得分"
              :fix-y-range="{ min: 0, max: 100 }"
              class="chart-block"
            />
            <p v-if="hasBreak" class="break-note">
              虚线为断点:题目已变更或判分口径已变更,断点两侧分数不可直接比较
            </p>
          </section>

          <!-- Probe side: success rate and latency over the same timeline. -->
          <section class="section">
            <div class="section-title">探测侧走势(启用端点聚合)</div>
            <el-empty
              v-if="trends.probe.length === 0"
              :description="trends.model.deleted ? '探测数据已随模型删除' : '暂无探测数据'"
              :image-size="60"
            />
            <template v-else>
              <TrendChart
                title="探测成功率"
                :categories="probeCategories"
                :series="successRateSeries"
                y-name="%"
                :fix-y-range="{ min: 0, max: 100 }"
                class="chart-block"
              />
              <TrendChart
                title="探测延迟"
                :categories="probeCategories"
                :series="latencySeries"
                y-name="ms"
                class="chart-block"
              />
            </template>
          </section>
        </template>
      </template>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getCampaignTrends } from '@/api/campaigns'
import TrendChart from '@/components/TrendChart.vue'
import { formatBucketTime } from '@/utils/format'
import type { CampaignTrends, TrendSuite } from '@/api/types'

// Model trend drill-down dialog (ticket 32): clicking a leaderboard row opens
// this dialog, which fetches the model's cross-campaign trend on demand. The
// score trend marks both kinds of caliber break — suite-version breaks
// ("vN 起题目变更", ADR 0007) and verdict-profile breaks ("判分口径已变更",
// ADR 0008) — with the same grey dashed placeholder line; the probe side
// (success rate + p50/p95 latency of the model's enabled endpoints) sits next
// to it so "score steady but latency exploding" is visible at a glance.
const props = defineProps<{
  campaignId: number
  model: { model_db_id: number; model_id: string } | null
}>()

defineEmits<{ close: [] }>()

const trends = ref<CampaignTrends | null>(null)
const loading = ref(false)
const error = ref('')

const hasBreak = computed(
  () => trends.value?.suites.some(s => s.points.some(p => p.version_changed || p.profile_changed)) ?? false
)

// Score charts: x = campaign batches, y = 0-100 score. Null points (unjudged
// batches) break the line instead of faking a score.
function categoriesOf(suite: TrendSuite): string[] {
  return suite.points.map(p => `#${p.campaign_id}`)
}

function seriesOf(suite: TrendSuite): { name: string; data: (number | null)[] }[] {
  return [{ name: '得分', data: suite.points.map(p => p.score) }]
}

// Break markers (ui-guidelines §5, TrendChart): one grey dashed vertical line
// per broken point; a point where the question bank and the scoring caliber
// changed at once carries both labels.
function markLinesOf(suite: TrendSuite): { xAxis: string; label: string }[] {
  const lines: { xAxis: string; label: string }[] = []
  for (const p of suite.points) {
    const labels: string[] = []
    if (p.version_changed) labels.push(`v${p.suite_version} 起题目变更`)
    if (p.profile_changed) labels.push('判分口径已变更')
    if (labels.length > 0) lines.push({ xAxis: `#${p.campaign_id}`, label: labels.join(' · ') })
  }
  return lines
}

// Probe charts: x = hour buckets, shared across both charts.
const probeCategories = computed(() => (trends.value?.probe ?? []).map(b => formatBucketTime(b.bucket_start)))

const successRateSeries = computed(() => [
  {
    name: '成功率',
    data: (trends.value?.probe ?? []).map(b =>
      b.total > 0 ? Math.round((1 - b.failures / b.total) * 1000) / 10 : null
    ),
  },
])

const latencySeries = computed(() => [
  { name: 'P50', data: (trends.value?.probe ?? []).map(b => b.p50_ms) },
  { name: 'P95', data: (trends.value?.probe ?? []).map(b => b.p95_ms) },
])

// Monotonic token invalidating stale responses (same race as ticket 42):
// opening row B while row A's fetch is in flight must not let A's late
// response overwrite B's trends.
let loadSeq = 0

async function reload() {
  if (!props.model) return
  const seq = ++loadSeq
  loading.value = true
  error.value = ''
  try {
    const data = await getCampaignTrends(props.campaignId, props.model.model_db_id)
    if (seq !== loadSeq) return
    trends.value = data
  } catch (err) {
    if (seq !== loadSeq) return
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

// Fetch on demand whenever a row is opened; reset on close so a reopened
// dialog never flashes the previous model's data.
watch(
  () => props.model,
  model => {
    loadSeq++
    trends.value = null
    error.value = ''
    if (model) reload()
  }
)
</script>

<style scoped>
.dialog-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.title-text {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dialog-body {
  min-height: 160px;
}
.load-error {
  text-align: center;
  padding: 24px 0;
}
.load-error-text {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin: 0 0 12px;
}
.section {
  margin-bottom: 24px;
}
.section-title {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 12px;
}
.chart-block {
  margin-bottom: 16px;
}
.break-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  margin: 0;
}
</style>
