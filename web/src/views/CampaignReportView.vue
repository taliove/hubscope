<template>
  <div class="report-page">
    <div class="page-header">
      <router-link to="/eval" class="back-link">← 返回评估榜单</router-link>
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
            <router-link to="/admin" class="failed-link">到管理台评估运营查看失败运行详情</router-link>
          </template>
        </el-alert>

        <Leaderboard :report="report" :family-options="familyOptions" @query="onQuery" />
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getCampaignReport } from '@/api/campaigns'
import Leaderboard from '@/components/Leaderboard.vue'
import { formatTime } from '@/utils/format'
import type { CampaignReport, CampaignStatus } from '@/api/types'

// Campaign report page (ticket 31): the leaderboard over one campaign's done
// runs, reusing the shared Leaderboard component (ticket 45). Deleted models
// never rank (server-side); family filter and ranking column are server
// query params re-emitted by the Leaderboard toolbar.
const route = useRoute()
const campaignId = Number(route.params.id)

const report = ref<CampaignReport | null>(null)
const loading = ref(false)
const error = ref('')

// Last query chosen inside the Leaderboard toolbar; re-applied on refresh.
const query = ref<{ family?: string; sort: string }>({ sort: 'total' })
// Family options come from the unfiltered board, so filtering never
// collapses the option list itself.
const familyOptions = ref<string[]>([])

const progressPercent = computed(() => {
  const p = report.value?.progress
  if (!p || p.total === 0) return 0
  return Math.round(((p.done + p.failed) / p.total) * 100)
})

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

function onQuery(q: { family?: string; sort: string }) {
  query.value = q
  void reload()
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const data = await getCampaignReport(campaignId, query.value)
    report.value = data
    armPolling()
    if (!query.value.family) {
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
  --el-card-padding: 16px;
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
</style>
