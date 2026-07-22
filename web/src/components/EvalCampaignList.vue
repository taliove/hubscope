<template>
  <div class="campaign-list" v-loading="loading">
    <!-- Run history grouped by campaign (one assessment batch per group);
         only the newest group starts expanded. -->
    <el-collapse v-if="campaignGroups.length > 0" v-model="expanded">
      <el-collapse-item v-for="group in campaignGroups" :key="group.campaign.id" :name="group.campaign.id">
        <template #title>
          <div class="campaign-header">
            <span class="campaign-title">批次 #{{ group.campaign.id }}</span>
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
        </template>
        <el-table :data="group.runs" size="small" @row-click="(row: EvalRun) => $emit('show-detail', row.id)">
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column label="评估集" min-width="130" show-overflow-tooltip>
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
      </el-collapse-item>
    </el-collapse>
    <el-empty v-else-if="!loading" description="暂无评估运行,点击「触发评估」发起首批评估" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { formatTime } from '@/utils/format'
import type { Campaign, CampaignStatus, EvalRun, EvalRunStatus, Suite } from '@/api/types'

// Eval run history grouped by campaign. Purely presentational: data comes
// from the parent panel, a row click asks it to open the run detail dialog.
const props = defineProps<{
  suites: Suite[]
  runs: EvalRun[]
  campaigns: Campaign[]
  loading: boolean
}>()

defineEmits<{
  'show-detail': [runId: number]
}>()

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
  // section synthesized from their shared batch identity. The synthetic
  // status/progress is derived from the member runs, not assumed running.
  const known = new Set(props.campaigns.map(c => c.id))
  const orphans = props.runs.filter(r => !known.has(r.campaign_id))
  if (orphans.length > 0) {
    const first = orphans[0]
    const done = orphans.filter(r => r.status === 'done').length
    const failed = orphans.filter(r => r.status === 'failed').length
    const running = orphans.length - done - failed
    const status: CampaignStatus = running > 0 ? 'running' : failed > 0 ? 'failed' : 'done'
    groups.push({
      campaign: {
        id: first.campaign_id,
        trigger: first.trigger,
        status,
        started_at: null,
        finished_at: null,
        created_at: first.started_at,
        progress: { total: orphans.length, done, failed, running },
      },
      runs: orphans,
    })
  }
  return groups
})

// The newest group starts expanded. Initialized once so poll-driven reloads
// never collapse groups the user opened or closed by hand.
const expanded = ref<number[]>([])
let expandedInitialized = false
watch(
  campaignGroups,
  groups => {
    if (!expandedInitialized && groups.length > 0) {
      expanded.value = [groups[0].campaign.id]
      expandedInitialized = true
    }
  },
  { immediate: true }
)

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
</script>

<style scoped>
.campaign-header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.campaign-title {
  font-size: var(--hs-text-sm);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.campaign-progress {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.campaign-time {
  margin-left: auto;
  padding-right: 8px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
</style>
