<template>
  <div class="board-page">
    <h1 class="page-title">评估榜单</h1>

    <!-- Loading / error / empty states (three-state rule, ui-guidelines §6). -->
    <el-card v-if="loading" shadow="never" class="state-block">
      <el-skeleton :rows="8" animated />
    </el-card>
    <el-alert v-else-if="error" type="error" :closable="false" class="state-block" :title="error">
      <el-button size="small" @click="load">重试</el-button>
    </el-alert>
    <template v-else-if="!report">
      <p v-if="running" class="running-note">新一批评估进行中</p>
      <el-card shadow="never" class="state-block">
        <el-empty description="暂无已完成的评估批次" />
      </el-card>
    </template>

    <!-- The settled board: the shared Leaderboard ranks and filters locally
         (the public endpoint takes no params), rows are not clickable (no
         drill-down on the public page), and the share-image entry reads
         "保存图片" for the recipient reader (shared caliber). -->
    <template v-else>
      <p v-if="running" class="running-note">新一批评估进行中,当前展示已完成批次 #{{ report.id }}</p>
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
// BoardView: the public eval board (/board, ticket 81, spec 0010) — the
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
.board-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px;
}
.page-title {
  margin: 0 0 16px;
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.state-block {
  margin-bottom: 16px;
}
/* In-flight hint (spec 0010): one neutral line, no background, no border,
   no polling. */
.running-note {
  margin: 0 0 16px;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
</style>
