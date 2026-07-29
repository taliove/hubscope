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
        <!-- Unfinished batches (ticket 52, spec 0004): the progress grid is
             the default view; the live half-scored leaderboard sits behind
             the "实时分数" switch. Both stay mounted (v-show) so the view
             switch never interrupts polling or resets the family filter. -->
        <template v-if="isUnfinished(report.status)">
          <EvalProgressGrid v-show="viewMode === 'grid'" v-model:view="viewMode" :report="report" />
          <Leaderboard
            v-show="viewMode === 'scores'"
            :key="report.id"
            :report="report"
            :family-options="familyOptions"
            live
            :view="viewMode"
            @update:view="viewMode = $event"
            @query="onQuery"
            @select="openTrend"
          />
        </template>

        <template v-else>
          <el-alert
            v-if="report.status === 'failed'"
            type="warning"
            :closable="false"
            class="state-block"
            :title="failedBatchWarning(report.progress.failed)"
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
            @select="openTrend"
          />
        </template>

        <!-- Row drill-down (ticket 32 pattern, same as the report page): the
             trend dialog fetches per model on demand. -->
        <ModelTrendDialog :campaign-id="selectedId ?? 0" :model="trendModel" @close="trendModel = null" />
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { listCampaigns, getCampaignReport } from '@/api/campaigns'
import EvalProgressGrid from '@/components/EvalProgressGrid.vue'
import Leaderboard from '@/components/Leaderboard.vue'
import ModelTrendDialog from '@/components/ModelTrendDialog.vue'
import { formatTime } from '@/utils/format'
import { failedBatchWarning } from '@/utils/evalWording'
import { createVisibilityPoll, type VisibilityPollHandle } from '@/utils/visibilityPoll'
import type { Campaign, CampaignReport, CampaignStatus, EvalBoardView, ReportRow } from '@/api/types'

// Eval leaderboard page (ticket 45): a pure consumption page. The batch
// switcher defaults to the newest done campaign; unfinished batches render
// the progress grid by default with a live half-scored board behind the
// view switch (ticket 52), and only they are polled (ui-guidelines §6). Ops
// and the case library live in /admin since ticket 44; row drill-down opens
// the shared ModelTrendDialog (ticket 32 pattern).
const router = useRouter()

const campaigns = ref<Campaign[]>([])
const selectedId = ref<number | null>(null)
const report = ref<CampaignReport | null>(null)

const loadingCampaigns = ref(false)
const loadingReport = ref(false)
const error = ref('')
const reportError = ref('')

// Board view of an unfinished batch: the progress grid is the default
// (spec 0004); "scores" reveals the live half-scored leaderboard.
const viewMode = ref<EvalBoardView>('grid')

// Last query chosen inside the Leaderboard toolbar; re-applied on refresh.
const query = ref<{ family?: string; sort: string }>({ sort: 'total' })
const familyOptions = ref<string[]>([])

const selected = computed(() => campaigns.value.find((c) => c.id === selectedId.value) ?? null)

// Trend drill-down (ticket 32): the row under inspection; null = dialog closed.
const trendModel = ref<ReportRow | null>(null)
function openTrend(row: ReportRow) {
  trendModel.value = row
}

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
// (ui-guidelines §6: every setInterval pairs with cleanup). The poll pauses
// in a hidden tab and refreshes immediately on return (ui-guidelines §6);
// a batch that settles while hidden is observed by that return refresh, so
// the settle transition semantics are unchanged.
let poll: VisibilityPollHandle | null = null
function armPolling() {
  poll?.clear()
  poll = null
  if (selected.value && isUnfinished(selected.value.status)) {
    poll = createVisibilityPoll(
      () => {
        void Promise.all([loadCampaigns(true), loadReport()]).then(armPolling)
      },
      { intervalMs: 3000 },
    )
  }
}
onBeforeUnmount(() => poll?.clear())

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
    const previous = report.value
    report.value = data
    // Settle transition (ui-guidelines §6): the poll tick that observes
    // done/failed stops polling (armPolling re-checks the campaign list) and
    // renders from exactly this response — no extra refetch, no emphasis
    // animation; the rank dashes simply swap for numbers. The message
    // carries the outcome.
    if (previous && previous.id === data.id && isUnfinished(previous.status) && !isUnfinished(data.status)) {
      if (data.status === 'done') {
        ElMessage.success(`批次 #${data.id} 已完成,榜单已生成`)
      } else {
        ElMessage.warning(
          `批次 #${data.id} 已结束:${data.progress.failed} 个评估运行失败,榜单仅统计已完成的评估集`,
        )
      }
    }
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
  viewMode.value = 'grid'
  trendModel.value = null
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
.failed-link {
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  text-decoration: none;
}
.failed-link:hover {
  color: var(--hs-brand-hover);
}
</style>
