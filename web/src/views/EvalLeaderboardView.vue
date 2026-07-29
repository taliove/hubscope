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
            <!-- Retry-failed (GH #28): settled batches with failed
                 (null-score) results; the batch returns to the running view
                 with its usual polling after the confirm. -->
            <el-button
              v-if="retryFailedVisible"
              size="small"
              :loading="retrying"
              @click="onRetryFailed"
            >
              重跑失败项
            </el-button>
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
          <!-- Case-level live feed (issue #17): console-only, refreshed by
               the page's own poll timer; unmounts at settle (historical
               batches never show it — the simple form of the brief's
               "settle 后停止增长" choice). -->
          <EvalLiveFeed
            :campaign-id="report.id"
            :entries="liveFeed"
            :loading="liveFeedLoading"
            :error="liveFeedError"
            @retry="loadLiveFeed"
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listCampaigns, getCampaignLiveFeed, getCampaignReport, retryCampaignFailed } from '@/api/campaigns'
import EvalLiveFeed from '@/components/EvalLiveFeed.vue'
import EvalProgressGrid from '@/components/EvalProgressGrid.vue'
import Leaderboard from '@/components/Leaderboard.vue'
import ModelTrendDialog from '@/components/ModelTrendDialog.vue'
import { formatTime } from '@/utils/format'
import { failedBatchWarning } from '@/utils/evalWording'
import { liveFeedCursor, mergeLiveFeed } from '@/utils/liveFeed'
import { parseBatchQuery, resolveInitialBatchId } from '@/utils/batchSelect'
import type { Campaign, CampaignReport, CampaignStatus, EvalBoardView, LiveFeedEntry, ReportRow } from '@/api/types'

// Eval leaderboard page (ticket 45): a pure consumption page. The batch
// switcher defaults to the newest done campaign; unfinished batches render
// the progress grid by default with a live half-scored board behind the
// view switch (ticket 52), and only they are polled (ui-guidelines §6). Ops
// and the case library live in /admin since ticket 44; row drill-down opens
// the shared ModelTrendDialog (ticket 32 pattern). A ?batch=<id> query
// (issue #16, from the AppHeader progress entry) overrides the default so
// the entry lands on the batch it was showing.
const router = useRouter()
const route = useRoute()

// Batch requested via ?batch=<id>; null when absent or unparseable. Kept
// reactive so a late navigation (clicking the header entry while already
// on /eval) still re-targets the selection.
const requestedBatchId = computed(() => parseBatchQuery(route.query.batch))

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

// Retry-failed entry (GH #28): settled selected batch with failed
// (null-score) results. Lives in the switcher row so it is visible in every
// view mode; the confirm wording matches the report page's entry.
const retrying = ref(false)
const retryFailedVisible = computed(
  () => report.value !== null && !isUnfinished(report.value.status) && report.value.failed_results > 0,
)

async function onRetryFailed() {
  if (!report.value) return
  const campaignID = report.value.id
  const failures = report.value.failed_results
  try {
    await ElMessageBox.confirm(
      `将重新评估批次 #${campaignID} 的 ${failures} 个失败项(只补未判分的题目,已判分结果不变),期间批次回到运行中。`,
      '重跑失败项',
      { confirmButtonText: '开始重跑', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return // cancelled — no feedback needed
  }
  retrying.value = true
  try {
    await retryCampaignFailed(campaignID)
    ElMessage.success(`已发起批次 #${campaignID} 的失败项重跑`)
    await reload()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    retrying.value = false
  }
}

// Trend drill-down (ticket 32): the row under inspection; null = dialog closed.
const trendModel = ref<ReportRow | null>(null)
function openTrend(row: ReportRow) {
  trendModel.value = row
}

// Live feed (issue #17): accumulated judged-case events of the selected
// unfinished batch. The parent owns the cursor and the fetch so the feed
// rides the page's single poll timer — no setInterval of its own.
const liveFeed = ref<LiveFeedEntry[]>([])
const liveFeedLoading = ref(false)
const liveFeedError = ref('')
// Monotonic token invalidating stale feed responses on batch switch (same
// race guard as reportSeq).
let feedSeq = 0

async function loadLiveFeed() {
  if (selectedId.value === null || !selected.value || !isUnfinished(selected.value.status)) return
  const seq = ++feedSeq
  liveFeedLoading.value = true
  try {
    const batch = await getCampaignLiveFeed(selectedId.value, liveFeedCursor(liveFeed.value))
    if (seq !== feedSeq) return
    liveFeed.value = mergeLiveFeed(liveFeed.value, batch)
    liveFeedError.value = ''
  } catch (err) {
    if (seq !== feedSeq) return
    liveFeedError.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (seq === feedSeq) liveFeedLoading.value = false
  }
}

// Batch switch resets the feed: the cursor is per-batch, and a stale
// in-flight response must not append into the new batch's stream.
function resetLiveFeed() {
  feedSeq++
  liveFeed.value = []
  liveFeedError.value = ''
  liveFeedLoading.value = false
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
// (ui-guidelines §6: every setInterval pairs with cleanup). The live feed
// (issue #17) rides this same timer — no interval of its own.
let pollTimer: ReturnType<typeof setInterval> | undefined
function armPolling() {
  clearInterval(pollTimer)
  pollTimer = undefined
  if (selected.value && isUnfinished(selected.value.status)) {
    pollTimer = setInterval(() => {
      void Promise.all([loadCampaigns(true), loadReport(), loadLiveFeed()]).then(armPolling)
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
    // Initial selection (issue #16): a valid ?batch=<id> query wins;
    // otherwise the established default — the newest done campaign, falling
    // back to the newest batch of any state so a running first batch still
    // shows its progress. Re-entry only happens when the selection is empty
    // or vanished, so polling never overrides a manual choice.
    if (selectedId.value === null || !list.some((c) => c.id === selectedId.value)) {
      selectedId.value = resolveInitialBatchId(list, requestedBatchId.value)
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
  await Promise.all([loadReport(), loadLiveFeed()])
  armPolling()
}

function switchBatch(id: number) {
  selectedId.value = id
  report.value = null
  query.value = { sort: 'total' }
  familyOptions.value = []
  viewMode.value = 'grid'
  trendModel.value = null
  resetLiveFeed()
  void Promise.all([loadReport(), loadLiveFeed()]).then(armPolling)
}

function onBatchChange() {
  if (selectedId.value === null) return
  // A manual switch syncs the URL (issue #16): the stale entry query can
  // never drag the user back, and a refresh keeps landing on the chosen
  // batch.
  void router.replace({ query: { ...route.query, batch: String(selectedId.value) } })
  switchBatch(selectedId.value)
}

// Late navigation to /eval?batch=<id> while the page is already mounted
// (e.g. clicking the AppHeader progress entry from /eval itself): re-target
// the selection when the query names a different, existing batch.
watch(requestedBatchId, (id) => {
  if (id === null || id === selectedId.value) return
  if (!campaigns.value.some((c) => c.id === id)) return
  switchBatch(id)
})

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
