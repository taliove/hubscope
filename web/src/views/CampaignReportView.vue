<template>
  <div class="report-page">
    <div class="page-header">
      <router-link to="/eval" class="back-link">← 返回评估中心</router-link>
      <div class="title-row">
        <h1 class="page-title">考核批次 #{{ campaignId }} 报告</h1>
        <template v-if="report">
          <el-tag size="small" :type="report.trigger === 'scheduled' ? 'info' : 'primary'">
            {{ report.trigger === 'scheduled' ? '定时' : '手动' }}
          </el-tag>
          <el-tag size="small" :type="statusTagType(report.status)">{{ statusLabel(report.status) }}</el-tag>
          <span class="meta-time">{{ formatTime(report.started_at) }}</span>
        </template>
      </div>
    </div>

    <!-- Error state: reason plus a retry entry (ui-guidelines §6). -->
    <el-alert v-if="error" type="error" :closable="false" class="state-block">
      <template #title>加载失败:{{ error }}</template>
      <el-button size="small" @click="reload">重试</el-button>
    </el-alert>

    <!-- Loading state. -->
    <el-card v-else-if="loading && !report" shadow="never" class="state-block">
      <el-skeleton :rows="8" animated />
    </el-card>

    <template v-else-if="report">
      <!-- Running batches show progress only: half-baked rankings are never
           displayed (spec 0002 review condition). -->
      <el-card v-if="report.status === 'running' || report.status === 'pending'" shadow="never" class="state-block">
        <el-progress
          :percentage="progressPercent"
          :status="report.progress.failed > 0 ? 'exception' : undefined"
        />
        <p class="progress-note">
          批次运行中:已完成 {{ report.progress.done + report.progress.failed }}/{{ report.progress.total }} 个评估运行,榜单将在批次结束后生成
        </p>
        <el-button size="small" :loading="loading" @click="reload">刷新进度</el-button>
      </el-card>

      <template v-else>
        <el-alert
          v-if="report.status === 'failed'"
          type="warning"
          :closable="false"
          class="state-block"
          :title="`批次有 ${report.progress.failed} 个评估运行失败,榜单仅统计已完成的评估集`"
        >
          <template #default>
            <router-link to="/eval" class="failed-link">到评估中心查看失败运行详情</router-link>
          </template>
        </el-alert>

        <el-card shadow="never" class="board-card">
          <!-- Toolbar: suite view switch, family filter, ranking column. -->
          <div class="toolbar">
            <el-radio-group v-model="viewSuite" size="small" @change="onViewSuiteChange">
              <el-radio-button value="total">总分</el-radio-button>
              <el-radio-button v-for="s in report.suites" :key="s.key" :value="s.key">
                {{ s.name }}
              </el-radio-button>
            </el-radio-group>
            <el-select
              v-model="family"
              size="small"
              clearable
              placeholder="全部系列"
              class="family-select"
              @change="reload"
            >
              <el-option v-for="f in familyOptions" :key="f" :label="f" :value="f" />
            </el-select>
            <el-select v-model="sortKey" size="small" class="sort-select" @change="reload">
              <el-option label="按总分排序" value="total" />
              <el-option v-for="s in report.suites" :key="s.key" :label="`按${s.name}排序`" :value="s.key" />
            </el-select>
          </div>

          <!-- Empty state: no model ranked (nothing scored, or filtered out). -->
          <el-empty v-if="report.rows.length === 0" :description="emptyDescription" />

          <!-- DesignArena-style horizontal bar leaderboard. -->
          <div v-else class="rows">
            <div v-for="(row, index) in report.rows" :key="row.model_db_id" class="row">
              <span class="rank">{{ index + 1 }}</span>
              <span class="model" :title="row.model_id">{{ row.model_id }}</span>
              <el-tag size="small" effect="plain" class="family-tag">{{ row.family }}</el-tag>
              <div class="bar-track">
                <div class="bar-fill" :style="{ width: barWidth(scoreOf(row)) }" />
              </div>
              <span class="score">{{ formatScore(scoreOf(row)) }}</span>
            </div>
          </div>
        </el-card>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getCampaignReport } from '@/api/campaigns'
import { formatScore, formatTime } from '@/utils/format'
import type { CampaignReport, CampaignStatus, ReportRow } from '@/api/types'

// Campaign report page (ticket 31): a DesignArena-style leaderboard over the
// campaign's done runs. Deleted models never rank (server-side). Family
// filter and ranking column are server query params; the suite view switch
// is client-side since every row carries all suite scores.
const route = useRoute()
const campaignId = Number(route.params.id)

const report = ref<CampaignReport | null>(null)
const loading = ref(false)
const error = ref('')

const viewSuite = ref('total')
const family = ref<string>('')
const sortKey = ref('total')
// Family options come from the unfiltered board, so filtering never collapses
// the option list itself.
const familyOptions = ref<string[]>([])

const progressPercent = computed(() => {
  const p = report.value?.progress
  if (!p || p.total === 0) return 0
  return Math.round(((p.done + p.failed) / p.total) * 100)
})

// Empty-state copy distinguishes "nothing scored" from "filtered out" so a
// fully-failed batch never reads as "deleted models don't rank".
const emptyDescription = computed(() => {
  if (family.value) return `系列 ${family.value} 下暂无上榜模型`
  if (report.value?.status === 'failed') return '暂无上榜模型:评估运行全部失败'
  return '暂无上榜模型:已删除模型不上榜'
})

// Switching the suite view also re-ranks by that suite: the board is a
// ranking, so bar lengths must stay monotonic top to bottom.
function onViewSuiteChange(value: string | number | boolean) {
  sortKey.value = String(value)
  reload()
}

// Poll while the batch is unfinished so progress feels live (ui-guidelines:
// every setInterval pairs with cleanup).
let pollTimer: ReturnType<typeof setInterval> | undefined
function armPolling() {
  clearInterval(pollTimer)
  pollTimer = undefined
  if (report.value?.status === 'running' || report.value?.status === 'pending') {
    pollTimer = setInterval(reload, 3000)
  }
}
onBeforeUnmount(() => clearInterval(pollTimer))

function scoreOf(row: ReportRow): number | null {
  if (viewSuite.value === 'total') return row.total_score
  return row.suite_scores[viewSuite.value] ?? null
}

function barWidth(score: number | null): string {
  if (score === null) return '0%'
  return `${Math.min(100, Math.max(0, score))}%`
}

function statusLabel(status: CampaignStatus): string {
  switch (status) {
    case 'done':
      return '已完成'
    case 'failed':
      return '失败'
    case 'pending':
      return '等待中'
    default:
      return '运行中'
  }
}

function statusTagType(status: CampaignStatus): 'info' | 'success' | 'danger' {
  return status === 'done' ? 'success' : status === 'failed' ? 'danger' : 'info'
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const data = await getCampaignReport(campaignId, {
      family: family.value || undefined,
      sort: sortKey.value,
    })
    report.value = data
    armPolling()
    if (!family.value) {
      const seen = new Set<string>()
      for (const row of data.rows) seen.add(row.family)
      familyOptions.value = [...seen].sort()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(reload)
</script>

<style scoped>
.report-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.page-header {
  margin-bottom: 16px;
}
.back-link {
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  text-decoration: none;
}
.back-link:hover {
  color: var(--hs-brand-hover);
}
.title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}
.page-title {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin: 0;
}
.meta-time {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.state-block {
  margin-bottom: 16px;
}
.failed-link {
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  text-decoration: none;
}
.failed-link:hover {
  color: var(--hs-brand-hover);
}
.progress-note {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin: 12px 0;
}
.board-card {
  margin-bottom: 16px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.family-select {
  width: 140px;
}
.sort-select {
  width: 160px;
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.rank {
  width: 24px;
  text-align: right;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  flex-shrink: 0;
}
.model {
  width: 220px;
  flex-shrink: 0;
  font-size: var(--hs-text-md);
  color: var(--hs-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.family-tag {
  flex-shrink: 0;
}
.bar-track {
  flex: 1;
  height: 20px;
  background: var(--hs-brand-soft);
  border-radius: var(--hs-radius-sm);
  overflow: hidden;
}
.bar-fill {
  height: 100%;
  background: var(--hs-brand);
  border-radius: var(--hs-radius-sm);
  transition: width 0.3s ease;
}
.score {
  width: 56px;
  text-align: right;
  font-size: var(--hs-text-md);
  font-weight: 600;
  line-height: 1.2;
  color: var(--hs-text-primary);
  flex-shrink: 0;
}
</style>
