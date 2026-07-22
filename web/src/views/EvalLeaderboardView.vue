<template>
  <div class="eval-leaderboard">
    <div class="page-header">
      <h1 class="page-title">评估榜单</h1>
    </div>

    <!-- Error state: reason plus a retry entry (ui-guidelines §6). -->
    <el-alert v-if="error" type="error" :closable="false" class="state-block">
      <template #title>加载失败:{{ error }}</template>
      <el-button size="small" @click="reload">重试</el-button>
    </el-alert>

    <!-- Loading state. -->
    <el-card v-else-if="loadingCampaigns && campaigns.length === 0" shadow="never" class="state-block">
      <el-skeleton :rows="6" animated />
    </el-card>

    <!-- Batch switcher empty state: no campaign has ever run. -->
    <el-card v-else-if="campaigns.length === 0" shadow="never" class="state-block">
      <el-empty description="暂无评估批次">
        <el-button type="primary" @click="router.push('/admin')">去管理台触发评估</el-button>
      </el-empty>
    </el-card>

    <template v-else>
      <!-- Batch switcher + batch meta. -->
      <el-card shadow="never" class="switcher-card">
        <div class="switcher-row">
          <el-select
            v-model="selectedId"
            class="batch-select"
            :loading="loadingCampaigns"
            @change="onBatchChange"
          >
            <el-option
              v-for="c in campaigns"
              :key="c.id"
              :value="c.id"
              :label="batchLabel(c)"
            />
          </el-select>
          <template v-if="selected">
            <el-tag size="small" :type="selected.trigger === 'scheduled' ? 'info' : 'primary'">
              {{ selected.trigger === 'scheduled' ? '定时' : '手动' }}
            </el-tag>
            <el-tag size="small" :type="statusTagType(selected.status)">
              {{ campaignStatusLabel(selected.status) }}
            </el-tag>
            <span class="meta-time">开始于 {{ formatTime(selected.started_at) }}</span>
            <span v-if="selected.finished_at" class="meta-time">
              结束于 {{ formatTime(selected.finished_at) }}
            </span>
          </template>
        </div>
      </el-card>

      <!-- Report-level error (kept separate so the switcher stays usable). -->
      <el-alert v-if="reportError" type="error" :closable="false" class="state-block">
        <template #title>榜单加载失败:{{ reportError }}</template>
        <el-button size="small" @click="loadReport">重试</el-button>
      </el-alert>

      <el-card v-else-if="loadingReport && !report" shadow="never" class="state-block">
        <el-skeleton :rows="8" animated />
      </el-card>

      <template v-else-if="report">
        <!-- Unfinished batches show progress only: half-baked rankings are
             never displayed (spec 0002 review condition). -->
        <el-card v-if="report.status === 'running' || report.status === 'pending'" shadow="never" class="state-block">
          <el-progress
            :percentage="progressPercent"
            :status="report.progress.failed > 0 ? 'exception' : undefined"
          />
          <p class="progress-note">
            批次{{ campaignStatusLabel(report.status) }}:已完成 {{ report.progress.done + report.progress.failed }}/{{ report.progress.total }} 个评估运行,榜单将在批次结束后生成
          </p>
          <el-button size="small" :loading="loadingReport" @click="loadReport">刷新进度</el-button>
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

          <!-- Hero: the leaderboard. Keyed by campaign so toolbar state
               (suite view, family, sort) resets per batch. -->
          <Leaderboard
            :key="report.id"
            :report="report"
            :family-options="familyOptions"
            @query="onQuery"
          />
        </template>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listCampaigns, getCampaignReport } from '@/api/campaigns'
import Leaderboard from '@/components/Leaderboard.vue'
import { formatTime } from '@/utils/format'
import type { Campaign, CampaignReport, CampaignStatus } from '@/api/types'

// Eval leaderboard page (ticket 45): a pure consumption page. The batch
// switcher defaults to the newest done campaign; unfinished batches are
// selectable but render a progress state instead of a board, and only they
// are polled (ui-guidelines §6). Ops and the case library live in /admin
// since ticket 44; row drill-down lands with the trends ticket.
const router = useRouter()

const campaigns = ref<Campaign[]>([])
const selectedId = ref<number | null>(null)
const report = ref<CampaignReport | null>(null)

const loadingCampaigns = ref(false)
const loadingReport = ref(false)
const error = ref('')
const reportError = ref('')

// Last query chosen inside the Leaderboard toolbar; re-applied on refresh.
const query = ref<{ family?: string; sort: string }>({ sort: 'total' })
const familyOptions = ref<string[]>([])

const selected = computed(() => campaigns.value.find((c) => c.id === selectedId.value) ?? null)

const progressPercent = computed(() => {
  const p = report.value?.progress
  if (!p || p.total === 0) return 0
  return Math.round(((p.done + p.failed) / p.total) * 100)
})

function campaignStatusLabel(status: CampaignStatus): string {
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

function batchLabel(c: Campaign): string {
  const trigger = c.trigger === 'scheduled' ? '定时' : '手动'
  return `批次 #${c.id} · ${trigger} · ${campaignStatusLabel(c.status)} · ${formatTime(c.started_at)}`
}

function isUnfinished(status: CampaignStatus): boolean {
  return status === 'running' || status === 'pending'
}

// Poll only while the selected batch is unfinished; every tick re-arms, so
// completion stops the timer and the board replaces the progress state
// (ui-guidelines §6: every setInterval pairs with cleanup).
let pollTimer: ReturnType<typeof setInterval> | undefined
function armPolling() {
  clearInterval(pollTimer)
  pollTimer = undefined
  if (selected.value && isUnfinished(selected.value.status)) {
    pollTimer = setInterval(() => {
      void Promise.all([loadCampaigns(true), loadReport()]).then(armPolling)
    }, 3000)
  }
}
onBeforeUnmount(() => clearInterval(pollTimer))

// Monotonic token invalidating stale report responses (same race as ticket
// 42): switching batches fast must not let an older report overwrite the
// newly selected one.
let reportSeq = 0

async function loadCampaigns(silent = false) {
  if (!silent) loadingCampaigns.value = true
  try {
    const list = await listCampaigns()
    campaigns.value = list
    // Default: the newest done campaign; fall back to the newest batch of
    // any state so a running first batch still shows its progress.
    if (selectedId.value === null || !list.some((c) => c.id === selectedId.value)) {
      selectedId.value = (list.find((c) => c.status === 'done') ?? list[0])?.id ?? null
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (!silent) loadingCampaigns.value = false
  }
}

async function loadReport() {
  if (selectedId.value === null) return
  const seq = ++reportSeq
  loadingReport.value = true
  reportError.value = ''
  try {
    const data = await getCampaignReport(selectedId.value, query.value)
    if (seq !== reportSeq) return
    report.value = data
    if (!query.value.family) {
      const seen = new Set<string>()
      for (const row of data.rows) seen.add(row.family)
      familyOptions.value = [...seen].sort()
    }
  } catch (err) {
    if (seq !== reportSeq) return
    reportError.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (seq === reportSeq) loadingReport.value = false
  }
}

async function reload() {
  error.value = ''
  await loadCampaigns()
  await loadReport()
  armPolling()
}

function onBatchChange() {
  report.value = null
  query.value = { sort: 'total' }
  familyOptions.value = []
  void loadReport().then(armPolling)
}

function onQuery(q: { family?: string; sort: string }) {
  query.value = q
  void loadReport()
}

onMounted(reload)
</script>

<style scoped>
.eval-leaderboard {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.page-header {
  margin-bottom: 16px;
}
.page-title {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin: 0;
}
.state-block {
  --el-card-padding: 16px;
  margin-bottom: 16px;
}
.switcher-card {
  --el-card-padding: 16px;
  margin-bottom: 16px;
}
.switcher-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.batch-select {
  width: 360px;
}
.meta-time {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.progress-note {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
  margin: 12px 0;
}
.failed-link {
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  text-decoration: none;
}
.failed-link:hover {
  color: var(--hs-brand-hover);
}
</style>
