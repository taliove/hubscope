<template>
  <div class="task-center">
    <el-alert v-if="error" :title="`加载失败:${error}`" type="error" :closable="false" class="error-alert" />

    <!-- Eval campaigns are first-class batches with their own state machine;
         the task center surfaces their aggregate progress here instead of
         duplicating them as tasks (ADR 0004). -->
    <el-card shadow="never" class="campaign-card">
      <template #header>
        <div class="card-header">
          <span>考核批次</span>
          <el-button size="small" @click="reloadCampaigns">刷新</el-button>
        </div>
      </template>
      <el-table :data="campaigns" v-loading="campaignsLoading" size="small" empty-text="暂无考核批次">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="来源" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.trigger === 'manual' ? 'primary' : 'info'" plain>
              {{ row.trigger === 'manual' ? '手动' : '定时' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="campaignTagType(row.status)">{{ campaignStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="进度" min-width="160">
          <template #default="{ row }">
            <el-progress
              :percentage="campaignPercent(row)"
              :status="row.status === 'failed' ? 'exception' : row.status === 'done' ? 'success' : undefined"
            />
            <span class="progress-text">
              {{ row.progress.done + row.progress.failed }}/{{ row.progress.total }} 个运行
            </span>
          </template>
        </el-table-column>
        <el-table-column label="开始时间" width="165">
          <template #default="{ row }">{{ formatTime(row.started_at) }}</template>
        </el-table-column>
        <el-table-column label="结束时间" width="165">
          <template #default="{ row }">{{ formatTime(row.finished_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <router-link :to="`/campaigns/${row.id}/report`" class="entity-link">查看报告</router-link>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" class="task-card">
      <template #header>
        <div class="card-header">
          <span>任务列表</span>
          <div class="filters">
            <el-select
              v-model="typeFilter"
              placeholder="全部类型"
              clearable
              size="small"
              style="width: 140px"
              @change="onFilterChange"
            >
              <el-option v-for="t in TYPE_OPTIONS" :key="t.value" :label="t.label" :value="t.value" />
            </el-select>
            <el-select
              v-model="statusFilter"
              placeholder="全部状态"
              clearable
              size="small"
              style="width: 140px"
              @change="onFilterChange"
            >
              <el-option v-for="s in STATUS_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-button size="small" @click="reload">刷新</el-button>
          </div>
        </div>
      </template>

      <el-table :data="tasks" v-loading="loading" size="small" empty-text="暂无任务">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }">{{ typeLabel(row.type) }}</template>
        </el-table-column>
        <el-table-column label="来源" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.source === 'manual' ? 'primary' : 'info'" plain>
              {{ sourceLabel(row.source) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
            <!-- Live unit completion (GH #156): running eval_run tasks only;
                 the fraction arrives from the server, never computed here. -->
            <span v-if="row.status === 'running' && row.progress !== null" class="progress-text">
              {{ Math.round(row.progress * 100) }}%
            </span>
          </template>
        </el-table-column>
        <el-table-column label="关联实体" width="140">
          <template #default="{ row }">
            <!-- GH #156: deep-link straight into the batch the run belongs
                 to instead of the bare leaderboard. -->
            <router-link
              v-if="row.entity_type === 'eval_run'"
              :to="row.campaign_id !== null ? `/eval?batch=${row.campaign_id}` : '/eval'"
              class="entity-link"
            >
              eval_run #{{ row.entity_id }}
            </router-link>
            <router-link v-else-if="row.entity_type === 'hub'" to="/admin" class="entity-link">
              hub #{{ row.entity_id }}
            </router-link>
            <span v-else-if="row.entity_type">{{ row.entity_type }} #{{ row.entity_id }}</span>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column label="开始时间" width="165">
          <template #default="{ row }">{{ formatTime(row.started_at) }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="100">
          <template #default="{ row }">{{ formatMs(row.duration_ms) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="165">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" link type="primary" @click="openLogs(row.id)">查看日志</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          background
          layout="total, prev, pager, next, sizes"
          :total="total"
          :page-size="pageSize"
          :page-sizes="[20, 50, 100]"
          :current-page="page"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>
    </el-card>

    <el-drawer v-model="drawerOpen" :title="`任务 #${detail?.id ?? ''} 执行日志`" size="55%">
      <div v-if="detail" class="log-header">
        <el-tag size="small" :type="statusTagType(detail.status)">{{ statusLabel(detail.status) }}</el-tag>
        <span class="log-meta">{{ typeLabel(detail.type) }} · {{ sourceLabel(detail.source) }} · 耗时 {{ formatMs(detail.duration_ms) }}</span>
        <el-button size="small" :loading="logsLoading" @click="reloadLogs">刷新</el-button>
      </div>
      <div v-loading="logsLoading" class="log-list">
        <div v-for="line in detail?.logs ?? []" :key="line.id" class="log-line">
          <span class="log-time">{{ formatTime(line.at) }}</span>
          <el-tag size="small" :type="levelTagType(line.level)" class="log-level">{{ line.level }}</el-tag>
          <span class="log-message">{{ line.message }}</span>
        </div>
        <el-empty v-if="!logsLoading && (detail?.logs.length ?? 0) === 0" description="暂无日志" />
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { getTask, listTasks } from '@/api/tasks'
import { listCampaigns } from '@/api/campaigns'
import type { Campaign, CampaignStatus, TaskDetail, TaskItem, TaskLogLevel, TaskStatus, TaskType } from '@/api/types'
import { formatMs, formatTime } from '@/utils/format'

// Task center page (tickets 18, 28): filterable, paginated task list with a
// per-task log drawer. Covers eval runs, discovery syncs, rollup and
// retention cleanup; probe rounds are not tasks and never appear here.
// Reads require a session like the other monitoring APIs; the router guard
// bounces anonymous visitors to /login.
const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const typeFilter = ref<TaskType | ''>('')
const statusFilter = ref<TaskStatus | ''>('')

// Eval campaigns (ticket 29): aggregate progress of every assessment batch.
const campaigns = ref<Campaign[]>([])
const campaignsLoading = ref(false)

const drawerOpen = ref(false)
const detail = ref<TaskDetail | null>(null)
const logsLoading = ref(false)

const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: 'pending', label: '等待中' },
  { value: 'running', label: '运行中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
]

const TYPE_OPTIONS: { value: TaskType; label: string }[] = [
  { value: 'eval_run', label: '评估运行' },
  { value: 'discovery_sync', label: '发现同步' },
  { value: 'rollup', label: '聚合汇总' },
  { value: 'retention_cleanup', label: '数据清理' },
]

function typeLabel(type: string): string {
  return TYPE_OPTIONS.find((t) => t.value === type)?.label ?? type
}

function sourceLabel(source: string): string {
  return source === 'manual' ? '手动' : source === 'scheduled' ? '定时' : source
}

function statusLabel(status: TaskStatus): string {
  return STATUS_OPTIONS.find((s) => s.value === status)?.label ?? status
}

function statusTagType(status: TaskStatus): 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'success':
      return 'success'
    case 'running':
      return 'warning'
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
}

function levelTagType(level: TaskLogLevel): 'success' | 'warning' | 'danger' | 'info' {
  switch (level) {
    case 'warn':
      return 'warning'
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const data = await listTasks({
      type: typeFilter.value || undefined,
      status: statusFilter.value || undefined,
      page: page.value,
      page_size: pageSize.value,
    })
    tasks.value = data.items
    total.value = data.total
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

// Campaign helpers: overall progress is the settled-run share of the batch.
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

function campaignTagType(status: CampaignStatus): 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    case 'running':
      return 'warning'
    default:
      return 'info'
  }
}

function campaignPercent(campaign: Campaign): number {
  if (campaign.progress.total === 0) return 0
  const settled = campaign.progress.done + campaign.progress.failed
  return Math.round((settled / campaign.progress.total) * 100)
}

async function reloadCampaigns() {
  campaignsLoading.value = true
  try {
    campaigns.value = await listCampaigns()
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    campaignsLoading.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  reload()
}

function onSizeChange(size: number) {
  pageSize.value = size
  page.value = 1
  reload()
}

function onFilterChange() {
  page.value = 1
  reload()
}

async function openLogs(id: number) {
  drawerOpen.value = true
  detail.value = null
  logsLoading.value = true
  try {
    detail.value = await getTask(id)
  } catch (err) {
    ElMessage.error((err as Error).message)
    drawerOpen.value = false
  } finally {
    logsLoading.value = false
  }
}

async function reloadLogs() {
  if (!detail.value) return
  logsLoading.value = true
  try {
    detail.value = await getTask(detail.value.id)
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    logsLoading.value = false
  }
}

onMounted(() => {
  reload()
  reloadCampaigns()
})
</script>

<style scoped>
.task-center {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.error-alert {
  margin-bottom: 16px;
}
/* Admin console compact density: 12px card padding (ui-guidelines §2). */
.task-card,
.campaign-card {
  --el-card-padding: 12px;
}
.campaign-card {
  margin-bottom: 16px;
}
.progress-text {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.filters {
  display: flex;
  gap: 8px;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
.entity-link {
  color: var(--hs-brand);
  text-decoration: none;
}
.log-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.log-meta {
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-sm);
  flex: 1;
}
.log-list {
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: var(--hs-text-xs);
}
.log-line {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 4px 0;
  border-bottom: 1px solid var(--hs-border);
}
.log-time {
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
.log-level {
  flex-shrink: 0;
}
.log-message {
  word-break: break-all;
}
</style>
