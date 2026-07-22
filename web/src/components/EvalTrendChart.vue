<template>
  <el-card v-if="selection" shadow="never" class="trend-card">
    <div class="card-title">
      历史趋势:{{ selection.modelId }} · {{ selection.suiteName }}
      <el-tag v-if="selectionDeleted" size="small" type="info">已删除</el-tag>
      <el-button class="close-button" size="small" text @click="$emit('close')">关闭</el-button>
    </div>
    <div v-loading="loading">
      <TimeSeriesChart
        v-if="points.length > 0"
        title=""
        :categories="categories"
        :series="series"
        y-name="得分"
      />
      <el-empty v-else-if="!loading" description="该组合暂无历史评估数据" :image-size="60" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getEvalRun } from '@/api/evals'
import TimeSeriesChart from '@/components/TimeSeriesChart.vue'
import { formatTime } from '@/utils/format'
import type { EvalRun, Model } from '@/api/types'

// Per-model score trend over historical runs of one suite. Run details are
// fetched on demand (the list only carries a cross-model aggregate), limited
// to the most recent runs to bound the fan-out. A model absent from the live
// model list is badged as deleted; its historical points stay visible.
const props = defineProps<{
  selection: { modelId: string; modelDbId: number; suiteId: number; suiteName: string } | null
  runs: EvalRun[]
  models: Model[]
}>()

defineEmits<{ close: [] }>()

const maxTrendRuns = 20

const loading = ref(false)
// Chronological (model, run) score points, oldest first.
const points = ref<{ finishedAt: string; score: number | null }[]>([])

// The selected model counts as deleted once it left the live model list.
const selectionDeleted = computed(
  () => props.selection !== null && !props.models.some(m => m.id === props.selection!.modelDbId)
)

const categories = computed(() => points.value.map(p => formatTime(p.finishedAt)))
const series = computed(() => [{ name: '聚合分', data: points.value.map(p => p.score) }])

// Reload the trend whenever the selected (model, suite) pair changes.
watch(
  () => props.selection,
  async sel => {
    points.value = []
    if (!sel) return
    loading.value = true
    try {
      const candidates = props.runs
        .filter(r => r.suite_id === sel.suiteId && r.status === 'done')
        .slice(0, maxTrendRuns)
      const details = await Promise.all(candidates.map(r => getEvalRun(r.id)))
      const loaded = details.map(d => ({
        finishedAt: d.finished_at ?? d.started_at,
        score: modelAggregate(d.results, sel.modelId),
      }))
      loaded.sort((a, b) => a.finishedAt.localeCompare(b.finishedAt))
      points.value = loaded
    } finally {
      loading.value = false
    }
  },
  { immediate: true }
)

// Average the non-null scores of one model inside a run; null when unscored.
function modelAggregate(results: { model_id: string; score: number | null }[], modelId: string): number | null {
  let sum = 0
  let n = 0
  for (const r of results) {
    if (r.model_id === modelId && r.score !== null) {
      sum += r.score
      n++
    }
  }
  return n === 0 ? null : sum / n
}
</script>

<style scoped>
.trend-card {
  margin-bottom: 16px;
}
/* Admin console compact density: 12px card padding (ui-guidelines §2). */
.trend-card {
  --el-card-padding: 12px;
}
.card-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 8px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.close-button {
  margin-left: auto;
}
</style>
