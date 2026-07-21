<template>
  <el-card shadow="never" class="runs-card">
    <div class="card-title">评估运行</div>

    <!-- Manual trigger: pick one suite and any chat-capable models. -->
    <div class="trigger-form">
      <el-select v-model="suiteId" placeholder="选择评估集" class="suite-select">
        <el-option v-for="s in suites" :key="s.id" :label="s.name" :value="s.id" />
      </el-select>
      <el-checkbox-group v-model="selectedModelIds">
        <el-tooltip
          v-for="m in models"
          :key="m.id"
          :disabled="m.capability === 'chat'"
          content="非对话模型不能参与评估"
          placement="top"
        >
          <el-checkbox :value="m.id" :disabled="m.capability !== 'chat'">
            {{ m.model_id }}
          </el-checkbox>
        </el-tooltip>
      </el-checkbox-group>
      <el-button
        type="primary"
        :disabled="!suiteId || selectedModelIds.length === 0 || tracking"
        @click="onTrigger"
      >
        触发评估
      </el-button>
    </div>

    <!-- Live progress of the just-triggered run, polled until it settles. -->
    <el-alert
      v-if="trackedRun"
      class="tracking-alert"
      :closable="false"
      :type="trackedRun.status === 'failed' ? 'error' : trackedRun.status === 'done' ? 'success' : 'info'"
      :title="trackingTitle"
    />

    <el-table :data="runs" v-loading="loading" @row-click="(row: EvalRun) => $emit('show-detail', row.id)">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column label="评估集" min-width="130">
        <template #default="{ row }">{{ suiteName(row.suite_id) }}</template>
      </el-table-column>
      <el-table-column label="触发方式" width="100">
        <template #default="{ row }">
          <el-tag size="small" :type="row.trigger === 'scheduled' ? 'info' : 'primary'">
            {{ row.trigger === 'scheduled' ? '定时' : '手动' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="judge_model" label="裁判模型" min-width="160" show-overflow-tooltip />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag size="small" :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="聚合分" width="90" align="center">
        <template #default="{ row }">{{ row.score === null ? '-' : row.score.toFixed(2) }}</template>
      </el-table-column>
      <el-table-column label="开始时间" width="170">
        <template #default="{ row }">{{ formatTime(row.started_at) }}</template>
      </el-table-column>
      <el-table-column label="结束时间" width="170">
        <template #default="{ row }">{{ formatTime(row.finished_at) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createEvalRun, getEvalRun } from '@/api/evals'
import { formatTime } from '@/utils/format'
import type { EvalRun, EvalRunStatus, Model, Suite } from '@/api/types'

// Eval run history plus the manual trigger form. A triggered run is polled
// until it leaves the running state, then the parent reloads the history.
const props = defineProps<{
  suites: Suite[]
  models: Model[]
  runs: EvalRun[]
  loading: boolean
}>()

const emit = defineEmits<{
  'show-detail': [runId: number]
  'run-settled': []
}>()

const suiteId = ref<number | null>(null)
const selectedModelIds = ref<number[]>([])
const trackedRun = ref<EvalRun | null>(null)
const tracking = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const pollIntervalMs = 1500

const trackingTitle = computed(() => {
  const run = trackedRun.value
  if (!run) return ''
  const name = suiteName(run.suite_id)
  if (run.status === 'running') return `运行中:${name}(#${run.id}),正在逐 case 评估…`
  if (run.status === 'done') {
    const score = run.score === null ? '无有效得分' : `聚合分 ${run.score.toFixed(2)}`
    return `已完成:${name}(#${run.id}),${score}`
  }
  return `运行失败:${name}(#${run.id})`
})

function suiteName(id: number): string {
  return props.suites.find(s => s.id === id)?.name ?? `#${id}`
}

function statusLabel(status: EvalRunStatus): string {
  return status === 'running' ? '运行中' : status === 'done' ? '已完成' : '失败'
}

function statusTagType(status: EvalRunStatus): 'info' | 'success' | 'danger' {
  return status === 'running' ? 'info' : status === 'done' ? 'success' : 'danger'
}

// Fire a manual run and start polling its status until it settles.
async function onTrigger() {
  if (!suiteId.value) return
  try {
    const run = await createEvalRun(suiteId.value, selectedModelIds.value)
    trackedRun.value = run
    tracking.value = true
    startPolling(run.id)
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

function startPolling(runId: number) {
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const run = await getEvalRun(runId)
      trackedRun.value = run
      if (run.status !== 'running') {
        stopPolling()
        tracking.value = false
        emit('run-settled')
      }
    } catch {
      // Transient poll failures keep the loop alive; the next tick retries.
    }
  }, pollIntervalMs)
}

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

onBeforeUnmount(stopPolling)
</script>

<style scoped>
.runs-card {
  margin-bottom: 16px;
}
.card-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}
.trigger-form {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.suite-select {
  width: 180px;
}
.tracking-alert {
  margin-bottom: 12px;
}
</style>
