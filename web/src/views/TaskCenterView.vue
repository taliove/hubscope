<template>
  <div class="task-center">
    <header class="page-header">
      <h1>AI Hub Checker</h1>
      <span class="subtitle">任务中心</span>
      <router-link to="/" class="nav-link">状态总览</router-link>
      <router-link to="/eval" class="nav-link sub-link">评估中心</router-link>
      <router-link to="/admin" class="nav-link sub-link">管理视图</router-link>
    </header>

    <el-alert v-if="error" :title="`加载失败:${error}`" type="error" :closable="false" class="error-alert" />

    <el-card shadow="never">
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
              <el-option label="评估运行" value="eval_run" />
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
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="关联实体" width="140">
          <template #default="{ row }">
            <router-link v-if="row.entity_type === 'eval_run'" to="/eval" class="entity-link">
              eval_run #{{ row.entity_id }}
            </router-link>
            <span v-else>{{ row.entity_type }} #{{ row.entity_id }}</span>
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
import { ElMessage } from 'element-plus'
import { getTask, listTasks } from '@/api/tasks'
import type { TaskDetail, TaskItem, TaskLogLevel, TaskStatus, TaskType } from '@/api/types'
import { formatMs, formatTime } from '@/utils/format'

// Task center page (ticket 18): filterable, paginated task list with a
// per-task log drawer. Reads require a session like the other monitoring
// APIs; the router guard bounces anonymous visitors to /login.
const tasks = ref<TaskItem[]>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const typeFilter = ref<TaskType | ''>('')
const statusFilter = ref<TaskStatus | ''>('')

const drawerOpen = ref(false)
const detail = ref<TaskDetail | null>(null)
const logsLoading = ref(false)

const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: 'pending', label: '等待中' },
  { value: 'running', label: '运行中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
]

function typeLabel(type: string): string {
  return type === 'eval_run' ? '评估运行' : type
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

onMounted(reload)
</script>

<style scoped>
.task-center {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.page-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 20px;
}
.page-header h1 {
  margin: 0;
  font-size: 22px;
  color: #303133;
}
.subtitle {
  color: #909399;
  font-size: 14px;
}
.nav-link {
  margin-left: auto;
  font-size: 14px;
  color: #409eff;
  text-decoration: none;
}
.sub-link {
  margin-left: 12px;
}
.error-alert {
  margin-bottom: 16px;
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
  color: #409eff;
  text-decoration: none;
}
.log-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.log-meta {
  color: #909399;
  font-size: 13px;
  flex: 1;
}
.log-list {
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
}
.log-line {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 3px 0;
  border-bottom: 1px solid #f0f0f0;
}
.log-time {
  color: #909399;
  white-space: nowrap;
}
.log-level {
  flex-shrink: 0;
}
.log-message {
  word-break: break-all;
}
</style>
