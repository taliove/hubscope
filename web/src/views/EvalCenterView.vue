<template>
  <div class="eval-center">
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
      :models="models"
      @close="trendSelection = null"
    />

    <EvalRunList
      :suites="suites"
      :models="models"
      :runs="runs"
      :campaigns="campaigns"
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
import { listCampaigns } from '@/api/campaigns'
import { listModels } from '@/api/models'
import ScoreMatrix from '@/components/ScoreMatrix.vue'
import EvalTrendChart from '@/components/EvalTrendChart.vue'
import EvalRunList from '@/components/EvalRunList.vue'
import EvalRunDetailDialog from '@/components/EvalRunDetailDialog.vue'
import CaseLibrary from '@/components/CaseLibrary.vue'
import type { Campaign, EvalRun, LatestScore, Model, Suite } from '@/api/types'

// Evaluation center page (public): score matrix + trend, run history with
// manual triggering, and the case library. Write calls redirect to /login
// via the API client when the session is missing.
const suites = ref<Suite[]>([])
const models = ref<Model[]>([])
const runs = ref<EvalRun[]>([])
const campaigns = ref<Campaign[]>([])
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
    const [s, m, r, c, l] = await Promise.all([
      listSuites(),
      listModels(),
      listEvalRuns(),
      listCampaigns(),
      listLatestScores(),
    ])
    suites.value = s
    models.value = m
    runs.value = r
    campaigns.value = c
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
.error-alert {
  margin-bottom: 16px;
}
</style>
