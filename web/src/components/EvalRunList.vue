<template>
  <el-card shadow="never" class="runs-card">
    <div class="card-title">评估运行</div>

    <!-- Manual triggers: a single suite against picked chat models, or the
         one-click full sweep (every suite x every chat model). -->
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
      <el-button
        type="warning"
        :disabled="tracking || chatModelCount === 0"
        @click="onFullSweep"
      >
        一键全量评估
      </el-button>
    </div>

    <!-- Live progress of the just-triggered campaign, polled until settled. -->
    <el-alert
      v-if="trackedCampaign"
      class="tracking-alert"
      :closable="false"
      :type="trackedCampaign.status === 'failed' ? 'error' : trackedCampaign.status === 'done' ? 'success' : 'info'"
      :title="trackingTitle"
    />

    <!-- Run history grouped by campaign (one assessment batch per group). -->
    <div v-for="group in campaignGroups" :key="group.campaign.id" class="campaign-group">
      <div class="campaign-header">
        <span class="campaign-title">考核批次 #{{ group.campaign.id }}</span>
        <el-tag size="small" :type="group.campaign.trigger === 'scheduled' ? 'info' : 'primary'">
          {{ group.campaign.trigger === 'scheduled' ? '定时' : '手动' }}
        </el-tag>
        <el-tag size="small" :type="campaignTagType(group.campaign.status)">
          {{ campaignStatusLabel(group.campaign.status) }}
        </el-tag>
        <span class="campaign-progress">
          进度 {{ group.campaign.progress.done + group.campaign.progress.failed }}/{{ group.campaign.progress.total }}
        </span>
        <span class="campaign-time">{{ formatTime(group.campaign.started_at) }}</span>
      </div>
      <el-table :data="group.runs" v-loading="loading" size="small" @row-click="(row: EvalRun) => $emit('show-detail', row.id)">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="评估集" min-width="130">
          <template #default="{ row }">{{ suiteName(row.suite_id) }}</template>
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
    </div>
    <el-empty v-if="!loading && campaignGroups.length === 0" description="暂无评估运行" />
  </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createEvalRun, createFullSweep } from '@/api/evals'
import { getCampaign } from '@/api/campaigns'
import { formatTime } from '@/utils/format'
import type { Campaign, CampaignStatus, EvalRun, EvalRunStatus, Model, Suite } from '@/api/types'

// Eval run history grouped by campaign, plus the manual trigger forms. A
// triggered campaign is polled until it settles, then the parent reloads the
// history.
const props = defineProps<{
  suites: Suite[]
  models: Model[]
  runs: EvalRun[]
  campaigns: Campaign[]
  loading: boolean
}>()

const emit = defineEmits<{
  'show-detail': [runId: number]
  'run-settled': []
}>()

const suiteId = ref<number | null>(null)
const selectedModelIds = ref<number[]>([])
const trackedCampaign = ref<Campaign | null>(null)
const tracking = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const pollIntervalMs = 1500

const chatModelCount = computed(() => props.models.filter(m => m.capability === 'chat').length)

// Campaign groups in display order (the API serves newest first), each with
// its member runs from the flat run history.
const campaignGroups = computed(() => {
  const byCampaign = new Map<number, EvalRun[]>()
  for (const run of props.runs) {
    const bucket = byCampaign.get(run.campaign_id) ?? []
    bucket.push(run)
    byCampaign.set(run.campaign_id, bucket)
  }
  const groups = props.campaigns.map(campaign => ({
    campaign,
    runs: byCampaign.get(campaign.id) ?? [],
  }))
  // Runs whose campaign fell out of the listing still show up, in a tail
  // section synthesized from their shared batch identity.
  const known = new Set(props.campaigns.map(c => c.id))
  const orphans = props.runs.filter(r => !known.has(r.campaign_id))
  if (orphans.length > 0) {
    const first = orphans[0]
    groups.push({
      campaign: {
        id: first.campaign_id,
        trigger: first.trigger,
        status: 'running',
        started_at: null,
        finished_at: null,
        created_at: first.started_at,
        progress: { total: orphans.length, done: 0, failed: 0, running: orphans.length },
      },
      runs: orphans,
    })
  }
  return groups
})

const trackingTitle = computed(() => {
  const campaign = trackedCampaign.value
  if (!campaign) return ''
  const p = campaign.progress
  const settled = p.done + p.failed
  if (campaign.status === 'running') {
    return `批次 #${campaign.id} 运行中:已完成 ${settled}/${p.total} 个评估运行…`
  }
  if (campaign.status === 'done') {
    return `批次 #${campaign.id} 已完成:${p.total} 个评估运行全部结束`
  }
  return `批次 #${campaign.id} 有失败:成功 ${p.done},失败 ${p.failed},共 ${p.total}`
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

function campaignTagType(status: CampaignStatus): 'info' | 'success' | 'danger' {
  return status === 'done' ? 'success' : status === 'failed' ? 'danger' : 'info'
}

// Fire a manual single-suite trigger and poll its campaign until it settles.
async function onTrigger() {
  if (!suiteId.value) return
  try {
    const campaign = await createEvalRun(suiteId.value, selectedModelIds.value)
    startTracking(campaign)
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

// Fire the one-click full sweep (every suite x every chat model).
async function onFullSweep() {
  try {
    const campaign = await createFullSweep()
    startTracking(campaign)
  } catch (err) {
    ElMessage.error((err as Error).message)
  }
}

function startTracking(campaign: Campaign) {
  trackedCampaign.value = campaign
  tracking.value = true
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const latest = await getCampaign(campaign.id)
      trackedCampaign.value = latest
      if (latest.status !== 'running' && latest.status !== 'pending') {
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
.campaign-group {
  margin-bottom: 16px;
}
.campaign-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
}
.campaign-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}
.campaign-progress {
  font-size: 12px;
  color: #909399;
}
.campaign-time {
  margin-left: auto;
  font-size: 12px;
  color: #909399;
}
</style>
