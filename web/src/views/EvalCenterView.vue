<template>
  <div class="eval-center">
    <header class="page-header">
      <h1>AI Hub Checker</h1>
      <span class="subtitle">评估中心</span>
      <router-link to="/" class="nav-link">状态总览</router-link>
      <router-link to="/admin" class="nav-link admin-link">管理视图</router-link>
    </header>

    <el-alert v-if="error" :title="`加载失败:${error}`" type="error" :closable="false" class="error-alert" />

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
      @close="trendSelection = null"
    />

    <EvalRunList
      :suites="suites"
      :models="models"
      :runs="runs"
      :loading="loading"
      @show-detail="detailRunId = $event"
      @run-settled="reload"
    />

    <EvalRunDetailDialog
      :run-id="detailRunId"
      :suites="suites"
      @close="detailRunId = null"
    />

    <CaseLibrary :suites="suites" :authed="authed" @refresh="reloadSuites" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { fetchAuthStatus } from '@/api/auth'
import { listEvalRuns, listLatestScores, listSuites } from '@/api/evals'
import { listModels } from '@/api/models'
import ScoreMatrix from '@/components/ScoreMatrix.vue'
import EvalTrendChart from '@/components/EvalTrendChart.vue'
import EvalRunList from '@/components/EvalRunList.vue'
import EvalRunDetailDialog from '@/components/EvalRunDetailDialog.vue'
import CaseLibrary from '@/components/CaseLibrary.vue'
import type { EvalRun, LatestScore, Model, Suite } from '@/api/types'

// Evaluation center page (public): score matrix + trend, run history with
// manual triggering, and the case library. Write calls redirect to /login
// via the API client when the session is missing.
const suites = ref<Suite[]>([])
const models = ref<Model[]>([])
const runs = ref<EvalRun[]>([])
const latest = ref<LatestScore[]>([])
const authed = ref(false)
const loading = ref(false)
const error = ref('')

const trendSelection = ref<{ modelId: string; modelDbId: number; suiteId: number; suiteName: string } | null>(null)
const detailRunId = ref<number | null>(null)

async function reloadSuites() {
  suites.value = await listSuites()
}

// Reload everything after a run settles (scores and history both change).
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

onMounted(async () => {
  await reload()
  // Auth status only gates the case edit forms; the page itself is public.
  try {
    authed.value = (await fetchAuthStatus()).authenticated
  } catch {
    authed.value = false
  }
})
</script>

<style scoped>
.eval-center {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.page-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 20px;
}
.page-header h1 {
  margin: 0;
  font-size: 22px;
  color: #303133;
}
.subtitle {
  color: #909399;
  font-size: 14px;
}
.nav-link {
  margin-left: auto;
  font-size: 14px;
  color: #409eff;
  text-decoration: none;
}
.admin-link {
  margin-left: 12px;
}
.error-alert {
  margin-bottom: 16px;
}
</style>
