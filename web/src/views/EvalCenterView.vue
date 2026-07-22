<template>
  <div class="eval-center">
    <el-alert v-if="error" :title="`加载失败:${error}`" type="error" :closable="false" class="error-alert">
      <el-button size="small" @click="reload">重试</el-button>
    </el-alert>

    <ScoreMatrix
      :suites="suites"
      :models="models"
      :latest="latest"
      :loading="loading"
      @select="trendSelection = $event"
    />

    <EvalTrendChart
      :selection="trendSelection"
      :runs="runs"
      :models="models"
      @close="trendSelection = null"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listEvalRuns, listLatestScores, listSuites } from '@/api/evals'
import { listModels } from '@/api/models'
import ScoreMatrix from '@/components/ScoreMatrix.vue'
import EvalTrendChart from '@/components/EvalTrendChart.vue'
import type { EvalRun, LatestScore, Model, Suite } from '@/api/types'

// Evaluation center (session-gated by the router guard): the score matrix
// plus the per-model trend chart. Ops (manual triggering, campaign history,
// run details) and the case library moved to /admin in ticket 44; the full
// leaderboard redesign of this page is ticket 31.
const suites = ref<Suite[]>([])
const models = ref<Model[]>([])
const runs = ref<EvalRun[]>([])
const latest = ref<LatestScore[]>([])
const loading = ref(false)
const error = ref('')

const trendSelection = ref<{ modelId: string; modelDbId: number; suiteId: number; suiteName: string } | null>(null)

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [s, m, r, l] = await Promise.all([
      listSuites(),
      listModels(),
      listEvalRuns(),
      listLatestScores(),
    ])
    suites.value = s
    models.value = m
    runs.value = r
    latest.value = l
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(reload)
</script>

<style scoped>
.eval-center {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.error-alert {
  margin-bottom: 16px;
}
</style>
