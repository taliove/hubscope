<template>
  <div class="benchmark-page">
    <!-- Page head (GH #118, spec 0018 §13): the Apple comparison-page
         framing — v2 page-title tier (3xl) plus the one-line positioning.
         The page answers "which model fits which scenario"; the matrix
         below is the evidence. -->
    <header class="page-head">
      <h1 class="page-title">Benchmark</h1>
      <p class="page-lede">不同模型适合什么场景：同一套考题，各能力维度逐项对比。</p>
    </header>

    <!-- Loading / error / empty states (three-state rule, ui-guidelines §6).
         State blocks share the leaderboard's light-container syntax (white
         surface, 1px border, radius-lg, no shadow). -->
    <div v-if="loading" class="state-block">
      <el-skeleton :rows="8" animated />
    </div>
    <el-alert v-else-if="error" type="error" :closable="false" class="state-alert" :title="error">
      <el-button size="small" @click="load">重试</el-button>
    </el-alert>
    <template v-else-if="!report">
      <p v-if="running" class="running-note">新一批评估进行中</p>
      <div class="state-block">
        <el-empty description="暂无已完成的评估批次" />
      </div>
    </template>

    <!-- The settled board (GH #118: the share entry keeps the OLD EvalCard
         material until T12 rebuilds the share materials — transitional):
         the shared Leaderboard ranks and filters locally (the public
         endpoint takes no params), rows are not clickable (no drill-down on
         the public page), and the share-image entry reads "保存图片" for
         the recipient reader (shared caliber). -->
    <template v-else>
      <!-- Batch identity + data freshness (GH #57): one neutral meta line
           under the page head, only on the loaded-report branch; the failed
           batch must read 失败于 (anti-fake caliber). -->
      <p class="batch-meta">{{ batchMeta }}</p>
      <p v-if="running" class="running-note">新一批评估进行中,当前展示已完成批次</p>
      <Leaderboard
        v-if="boardReport"
        :report="boardReport"
        :family-options="familyOptions"
        shared
        :selectable="false"
        @query="onQuery"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
// BoardView: the public Benchmark page (/benchmark, GH #118, spec 0018 §13 —
// renamed from /board in the v2 IA, GH #112; /board redirects here). The
// newest settled batch's matrix leaderboard, anonymous like the status
// board. The page fetches the full report once; column-header sorting and
// family filtering run client-side (boardSort mirrors the server caliber),
// the share image is generated purely client-side, and there is no polling
// — the in-flight hint is a static line.
import { computed, onMounted, ref } from 'vue'
import { getPublicEvalBoard } from '@/api/campaigns'
import type { CampaignReport } from '@/api/types'
import Leaderboard from '@/components/Leaderboard.vue'
import { familyOptionsOf, filterRowsByFamily, sortRows } from '@/utils/boardSort'
import { campaignSettleVerb, campaignTriggerLabel } from '@/utils/evalWording'
import { formatTimeMinute } from '@/utils/format'

const report = ref<CampaignReport | null>(null)
const running = ref(false)
const loading = ref(false)
const error = ref('')

// Local ranking/filter state, re-emitted by the Leaderboard's toolbar and
// column headers. familyOptions come from the unfiltered board so the
// option list never collapses with the selection.
const family = ref('')
const sortKey = ref('total')

const familyOptions = computed(() => (report.value ? familyOptionsOf(report.value.rows) : []))

// Batch meta line (GH #57):「批次 #N · 定时/手动 · 完成于/失败于 HH:mm」.
// The settle segment is omitted entirely when finished_at is null (a
// theoretical edge — /benchmark only ever shows settled batches).
const batchMeta = computed(() => {
  const r = report.value
  if (!r) return ''
  const base = `批次 #${r.id} · ${campaignTriggerLabel(r.trigger)}`
  if (!r.finished_at) return base
  return `${base} · ${campaignSettleVerb(r.status)} ${formatTimeMinute(r.finished_at)}`
})

// The board the Leaderboard renders: the fetched report with its rows
// re-ranked and filtered locally — same shape, never a second caliber.
const boardReport = computed<CampaignReport | null>(() => {
  if (!report.value) return null
  return {
    ...report.value,
    rows: sortRows(filterRowsByFamily(report.value.rows, family.value), sortKey.value),
  }
})

function onQuery(query: { family?: string; sort: string }) {
  family.value = query.family ?? ''
  sortKey.value = query.sort
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const board = await getPublicEvalBoard()
    report.value = board.report
    running.value = board.running
  } catch (err) {
    error.value = `加载评估榜单失败:${(err as Error).message}`
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.benchmark-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--hs-space-5) var(--hs-space-4) var(--hs-space-7);
}
/* Comparison-page head: the title carries the hierarchy, the lede answers
   "what is this page for" in one neutral line. */
.page-head {
  margin-bottom: var(--hs-space-5);
}
.page-title {
  margin: 0;
  font-size: var(--hs-text-3xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.page-lede {
  margin: var(--hs-space-2) 0 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-secondary);
}
/* Light container (v2 Apple syntax, AlertsView timeline-panel precedent):
   white surface, 1px border, radius-lg, no shadow — static containers never
   take a shadow. */
.state-block {
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
  padding: var(--hs-space-5) var(--hs-space-6);
  margin-bottom: var(--hs-space-4);
}
.state-alert {
  margin-bottom: var(--hs-space-4);
}
/* Batch meta line (GH #57): sits under the page head as the factual
   sub-line, neutral and uncolored like the running note. */
.batch-meta {
  margin: 0 0 var(--hs-space-4);
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
/* In-flight hint (spec 0010): one neutral line, no background, no border,
   no polling. */
.running-note {
  margin: 0 0 var(--hs-space-4);
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
</style>
