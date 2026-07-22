<template>
  <div class="report-page">
    <div class="page-header">
      <router-link v-if="!shared" to="/eval" class="back-link no-print">← 返回评估榜单</router-link>
      <div class="title-row">
        <h1 class="page-title">考核批次 #{{ displayCampaignId }} 报告</h1>
        <template v-if="report">
          <el-tag size="small" :type="report.trigger === 'scheduled' ? 'info' : 'primary'">
            {{ report.trigger === 'scheduled' ? '定时' : '手动' }}
          </el-tag>
          <el-tag size="small" :type="statusTagType(report.status)">{{ statusLabel(report.status) }}</el-tag>
          <span class="meta-time">{{ formatTime(report.started_at) }}</span>
        </template>
        <span class="header-actions no-print">
          <el-button v-if="!shared" size="small" :loading="sharing" @click="onShare">分享</el-button>
          <el-button size="small" @click="onPrint">导出 PDF</el-button>
        </span>
      </div>
      <p v-if="shared" class="shared-note no-print">此页面为只读分享视图</p>
    </div>

    <!-- Error state: reason plus a retry entry (ui-guidelines §6). In shared
         mode a 404 means the link is gone; anything else is a server or
         network failure and must not be misread as a dead link. -->
    <el-alert v-if="error" type="error" :closable="false" class="state-block">
      <template #title>{{ sharedErrorTitle }}</template>
      <el-button v-if="!shared || !errorIs404" size="small" @click="reload">重试</el-button>
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
        <el-button size="small" :loading="loading" class="no-print" @click="reload">刷新进度</el-button>
      </el-card>

      <template v-else>
        <el-alert
          v-if="report.status === 'failed'"
          type="warning"
          :closable="false"
          class="state-block"
          :title="`批次有 ${report.progress.failed} 个评估运行失败,榜单仅统计已完成的评估集`"
        >
          <template v-if="!shared" #default>
            <router-link to="/admin" class="failed-link no-print">到管理台评估运营查看失败运行详情</router-link>
          </template>
        </el-alert>

        <Leaderboard :report="report" :family-options="familyOptions" @query="onQuery" @select="openTrend" />

        <ModelTrendDialog :campaign-id="campaignId" :model="trendModel" @close="trendModel = null" />
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ApiError } from '@/api/client'
import { getCampaignReport } from '@/api/campaigns'
import { createShareLink, getSharedReport, shareLinkUrl } from '@/api/shareLinks'
import Leaderboard from '@/components/Leaderboard.vue'
import ModelTrendDialog from '@/components/ModelTrendDialog.vue'
import { formatTime } from '@/utils/format'
import { copyText } from '@/utils/clipboard'
import type { CampaignReport, CampaignStatus, ReportRow } from '@/api/types'

// Campaign report page (ticket 31): the leaderboard over one campaign's done
// runs, reusing the shared Leaderboard component (ticket 45). Deleted models
// never rank (server-side); family filter and ranking column are server
// query params re-emitted by the Leaderboard toolbar.
//
// Shared mode (ticket 33, ADR 0006): the /report/:token route renders this
// same view without a session, fed by the public shared-report endpoint and
// stripped of every operation entry (back links, the share button).
const route = useRoute()
const shareToken = route.params.token as string | undefined
const shared = shareToken !== undefined
const authedCampaignId = shared ? 0 : Number(route.params.id)
// The trend dialog always keys off the concrete campaign: the route id when
// authenticated, the payload id once a shared report has loaded.
const campaignId = computed(() => (shared ? (report.value?.id ?? 0) : authedCampaignId))

const report = ref<CampaignReport | null>(null)
const loading = ref(false)
const error = ref('')
const errorStatus = ref(0)
const sharing = ref(false)

// In shared mode only a 404 means the link is dead; server/network failures
// get a retry instead of a misleading "link gone" message.
const errorIs404 = computed(() => errorStatus.value === 404)
const sharedErrorTitle = computed(() => {
  if (!shared) return `加载失败:${error.value}`
  return errorIs404.value ? '链接不存在或已撤销' : `加载失败:${error.value}`
})

// Last query chosen inside the Leaderboard toolbar; re-applied on refresh.
const query = ref<{ family?: string; sort: string }>({ sort: 'total' })
// Family options come from the unfiltered board, so filtering never
// collapses the option list itself.
const familyOptions = ref<string[]>([])

// In shared mode the campaign id only arrives with the report payload.
const displayCampaignId = computed(() => (shared ? (report.value?.id ?? '—') : authedCampaignId))

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

// Trend drill-down (ticket 32): the row under inspection; null = dialog closed.
const trendModel = ref<ReportRow | null>(null)
function openTrend(row: ReportRow) {
  trendModel.value = row
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

function onQuery(q: { family?: string; sort: string }) {
  query.value = q
  void reload()
}

// Mint a share link for this campaign and hand the URL to the clipboard;
// when the clipboard is unavailable the URL is shown for manual copying.
async function onShare() {
  sharing.value = true
  try {
    const link = await createShareLink(authedCampaignId)
    const url = shareLinkUrl(link.token)
    if (await copyText(url)) {
      ElMessage.success('分享链接已复制,无需登录即可打开')
    } else {
      await ElMessageBox.alert(url, '分享链接(请手动复制)', { confirmButtonText: '关闭' })
    }
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    sharing.value = false
  }
}

// Print export (ticket 33): the browser's print-to-PDF over the print
// stylesheet; no server-side rendering or new dependency.
function onPrint() {
  window.print()
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const data = shared
      ? await getSharedReport(shareToken as string, query.value)
      : await getCampaignReport(campaignId.value, query.value)
    report.value = data
    armPolling()
    if (!query.value.family) {
      const seen = new Set<string>()
      for (const row of data.rows) seen.add(row.family)
      familyOptions.value = [...seen].sort()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    errorStatus.value = err instanceof ApiError ? err.status : 0
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
  flex-wrap: wrap;
}
.page-title {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin: 0;
}
.header-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}
.shared-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  margin: 8px 0 0;
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
