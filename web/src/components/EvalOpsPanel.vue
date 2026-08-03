<template>
  <!-- Light container (GH #120, v2 Apple syntax): white surface, 1px border,
       radius-lg, no shadow; the EP complex parts inside stay token-driven. -->
  <div class="ops-panel">
    <div class="panel-header">
      <div class="card-title">评估批次</div>
      <el-button type="primary" :disabled="tracking" @click="triggerDialogVisible = true">触发评估</el-button>
    </div>

    <el-alert
      v-if="error"
      class="load-alert"
      type="error"
      :closable="false"
      :title="`加载失败:${error}`"
    >
      <el-button size="small" @click="reload">重试</el-button>
    </el-alert>

    <!-- Live progress of the just-triggered campaign, polled until settled.
         The settled conclusion stays dismissible so it never lingers. -->
    <el-alert
      v-if="trackedCampaign"
      class="tracking-alert"
      :closable="!tracking"
      :type="trackedCampaign.status === 'failed' ? 'error' : trackedCampaign.status === 'done' ? 'success' : 'info'"
      :title="trackingTitle"
      @close="trackedCampaign = null"
    >
      <!-- Cancel (GH #152): stop the tracked batch while it runs. -->
      <el-button v-if="tracking" size="small" type="danger" plain :loading="canceling" @click="onCancelTracked">
        取消批次
      </el-button>
    </el-alert>

    <EvalCampaignList
      :suites="suites"
      :runs="runs"
      :campaigns="campaigns"
      :loading="loading"
      @show-detail="detailRunId = $event"
    />

    <EvalTriggerDialog
      v-model="triggerDialogVisible"
      :suites="suites"
      :models="models"
      @triggered="startTracking"
    />
    <EvalRunDetailDialog :run-id="detailRunId" :suites="suites" @close="detailRunId = null" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ApiError } from '@/api/client'
import { listEvalRuns, listSuites } from '@/api/evals'
import { cancelCampaign, getCampaign, listCampaigns } from '@/api/campaigns'
import { listModels } from '@/api/models'
import EvalCampaignList from '@/components/EvalCampaignList.vue'
import EvalTriggerDialog from '@/components/EvalTriggerDialog.vue'
import EvalRunDetailDialog from '@/components/EvalRunDetailDialog.vue'
import { createVisibilityPoll } from '@/utils/visibilityPoll'
import type { Campaign, EvalRun, Model, Suite } from '@/api/types'

// Eval operations panel (admin console): manual triggering, live progress of
// the tracked campaign, and the campaign-grouped run history. The panel
// fetches its own data so the admin view's resource data flow stays
// untouched; a settled campaign reloads everything.
const suites = ref<Suite[]>([])
const models = ref<Model[]>([])
const runs = ref<EvalRun[]>([])
const campaigns = ref<Campaign[]>([])
const loading = ref(false)
const error = ref('')

const triggerDialogVisible = ref(false)
const detailRunId = ref<number | null>(null)
const trackedCampaign = ref<Campaign | null>(null)
const tracking = ref(false)
let pollHandle: { clear(): void } | null = null

const pollIntervalMs = 1500

const trackingTitle = computed(() => {
  const campaign = trackedCampaign.value
  if (!campaign) return ''
  const p = campaign.progress
  const settled = p.done + p.failed
  if (campaign.status === 'running') {
    return `批次 #${campaign.id} 运行中:已完成 ${settled}/${p.total} 个评估运行…`
  }
  if (campaign.status === 'pending') {
    return `批次 #${campaign.id} 等待中:共 ${p.total} 个评估运行待执行…`
  }
  if (campaign.status === 'done') {
    return `批次 #${campaign.id} 已完成:${p.total} 个评估运行全部结束`
  }
  return `批次 #${campaign.id} 有失败:成功 ${p.done},失败 ${p.failed},共 ${p.total}`
})

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const [s, m, r, c] = await Promise.all([
      listSuites(),
      listModels(),
      listEvalRuns(),
      listCampaigns(),
    ])
    suites.value = s
    models.value = m
    runs.value = r
    campaigns.value = c
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

// Poll a freshly triggered campaign until it settles, then refresh the
// history. Batch-class poll (ui-guidelines §6): paused while the tab is
// hidden, immediate refresh on return; the handle is always cleared on
// unmount.
function startTracking(campaign: Campaign) {
  trackedCampaign.value = campaign
  tracking.value = true
  stopPolling()
  pollHandle = createVisibilityPoll(
    async () => {
      try {
        const latest = await getCampaign(campaign.id)
        trackedCampaign.value = latest
        if (latest.status !== 'running' && latest.status !== 'pending') {
          stopPolling()
          tracking.value = false
          // reload swallows its own errors into the error alert, so no
          // try/catch is needed here.
          await reload()
        }
      } catch (err) {
        // Permanent failures (GH #157: batch deleted, session expired)
        // stop the loop — otherwise the "批次运行中" alert would hang
        // forever with no way out. Transient failures keep it alive.
        if (err instanceof ApiError && (err.status === 401 || err.status === 404)) {
          stopPolling()
          tracking.value = false
          error.value = `批次 #${campaign.id} 追踪中断:${err.message}`
        }
      }
    },
    { intervalMs: pollIntervalMs },
  )
}

function stopPolling() {
  if (pollHandle !== null) {
    pollHandle.clear()
    pollHandle = null
  }
}

// Cancel the tracked batch (GH #152): unstarted cells are dropped and the
// batch settles failed; the poll loop observes the settle as usual.
const canceling = ref(false)
async function onCancelTracked() {
  const campaign = trackedCampaign.value
  if (!campaign) return
  try {
    await ElMessageBox.confirm(
      `将停止批次 #${campaign.id}:未开始的评估单元放弃,在飞的跑完后批次判失败;已判分结果保留。`,
      '取消批次',
      { confirmButtonText: '停止批次', cancelButtonText: '返回', type: 'warning' },
    )
  } catch {
    return // dismissed — no feedback needed
  }
  canceling.value = true
  try {
    await cancelCampaign(campaign.id)
    ElMessage.success(`已发起批次 #${campaign.id} 的取消`)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : String(err))
  } finally {
    canceling.value = false
  }
}

onMounted(reload)
onBeforeUnmount(stopPolling)
</script>

<style scoped>
/* Light container (GH #120, v2 Apple syntax): white surface, 1px border,
   radius-lg, no shadow. The compact working density is carried by the EP
   tables and forms inside, not by a tighter card padding. */
.ops-panel {
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
  padding: var(--hs-space-5) var(--hs-space-6);
}
.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.card-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.load-alert {
  margin-bottom: 12px;
}
.tracking-alert {
  margin-bottom: 12px;
}
</style>
