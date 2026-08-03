<template>
  <el-dialog
    :model-value="runId !== null"
    :title="dialogTitle"
    width="80%"
    @update:model-value="(v: boolean) => { if (!v) $emit('close') }"
  >
    <div v-loading="loading">
      <div v-if="error" class="load-error">
        <p class="load-error-text">加载失败:{{ error }}</p>
        <el-button size="small" @click="runId !== null && loadRun(runId)">重试</el-button>
      </div>
      <template v-else-if="detail">
        <div class="run-meta">
          <el-tag size="small" :type="detail.trigger === 'scheduled' ? 'info' : 'primary'">
            {{ detail.trigger === 'scheduled' ? '定时' : '手动' }}
          </el-tag>
          <span>裁判模型:{{ detail.judge_model }}</span>
          <span>开始:{{ formatTime(detail.started_at) }}</span>
          <span>结束:{{ formatTime(detail.finished_at) }}</span>
          <span>聚合分:{{ formatScore(detail.score === null ? null : detail.score * 100) }}</span>
          <!-- Targeted retry entries: only when the parent marks the batch
               retryable (settled, console). The per-row buttons re-run one
               failed unit; this entry re-runs every failed unit of the
               batch (existing retry-failed). Scored results never move. -->
          <el-button v-if="retryable" size="small" :loading="retryingAll" @click="onRetryAll">
            重跑全部失败项
          </el-button>
        </div>
        <el-table :data="visibleResults" row-key="id" max-height="480">
          <el-table-column type="expand">
            <template #default="{ row }">
              <div class="expand-panel">
                <div class="expand-block">
                  <div class="expand-label">题目</div>
                  <pre class="expand-text">{{ casePrompt(row.case_id) }}</pre>
                </div>
                <div class="expand-block">
                  <div class="expand-label">模型作答</div>
                  <pre class="expand-text">{{ row.answer_text ?? '(无作答)' }}</pre>
                </div>
                <div class="expand-block">
                  <div class="expand-label">判定理由</div>
                  <pre class="expand-text">{{ row.verdict_detail ?? '-' }}</pre>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="模型" min-width="170" show-overflow-tooltip>
            <template #default="{ row }">
              <span>{{ row.model_id }}</span>
              <el-tag v-if="row.model_deleted" size="small" type="info" class="deleted-tag">已删除</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="题目" min-width="240" show-overflow-tooltip>
            <template #default="{ row }">{{ casePrompt(row.case_id) }}</template>
          </el-table-column>
          <el-table-column label="得分" width="80" align="center">
            <template #default="{ row }">
              <span :class="scoreClass(row.score)">{{ formatScore(row.score === null ? null : row.score * 100) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="判定理由" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ row.verdict_detail ?? '-' }}</template>
          </el-table-column>
          <el-table-column label="延迟" width="90" align="right">
            <template #default="{ row }">{{ formatMs(row.latency_ms) }}</template>
          </el-table-column>
          <el-table-column v-if="retryable" label="操作" width="80" align="center">
            <template #default="{ row }">
              <el-button
                v-if="row.score === null && unitRetryable(row)"
                size="small"
                text
                type="primary"
                :loading="retryingResultId === row.id"
                @click="onRetryUnit(row)"
              >
                重跑
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { getEvalRun } from '@/api/evals'
import { retryCampaignFailed, retryCampaignUnits } from '@/api/campaigns'
import { formatMs, formatScore, formatTime } from '@/utils/format'
import type { EvalResult, EvalRunDetail, Suite } from '@/api/types'

// Run detail dialog: per-case prompt, model answer, score, and the judge's
// reasoning, grouped in an expandable table. The optional modelId filter
// (GH #156 block 4) narrows the table to one model — the leaderboard cell
// drill-down's shape; omitted, the dialog shows every model of the run
// (EvalOpsPanel usage, unchanged).
//
// Targeted retry (retry-units): when the parent passes retryable (a settled
// batch on the console) plus modelDbIds (model_id → model_db_id, from the
// report rows), each failed (null-score) row gets a 重跑 button re-running
// exactly that unit, and the meta row carries a 重跑全部失败项 entry (the
// existing batch retry-failed). A successful retry reverts the batch to
// running, so the dialog closes after notifying the parent (retried) — the
// parent reloads and re-arms its polling.
const props = defineProps<{
  runId: number | null
  suites: Suite[]
  modelId?: string | null
  retryable?: boolean
  modelDbIds?: Record<string, number>
}>()

const emit = defineEmits<{ close: []; retried: [] }>()

const detail = ref<EvalRunDetail | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

const dialogTitle = computed(() => {
  if (!detail.value) return '评估运行详情'
  const suite = props.suites.find(s => s.id === detail.value!.suite_id)
  const base = `评估运行 #${detail.value.id} · ${suite?.name ?? ''}`
  return props.modelId ? `${base} · ${props.modelId}` : base
})

// The cell drill-down narrows the run to the clicked model; without a
// modelId the full run table renders (backward compatible).
const visibleResults = computed(() => {
  if (!detail.value) return []
  if (!props.modelId) return detail.value.results
  return detail.value.results.filter(r => r.model_id === props.modelId)
})

// Monotonic token invalidating stale responses: when runId switches quickly,
// a slow earlier request must not overwrite the newer load's detail/error,
// and its finally must not clear the newer load's loading state.
let loadSeq = 0

// Load one run's detail; failures surface as an in-dialog error with retry
// instead of an unhandled rejection and a blank dialog.
async function loadRun(id: number) {
  const seq = ++loadSeq
  detail.value = null
  error.value = null
  loading.value = true
  try {
    const result = await getEvalRun(id)
    if (seq !== loadSeq) return
    detail.value = result
  } catch (err) {
    if (seq !== loadSeq) return
    error.value = (err as Error).message
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

// Load the run whenever a new one is opened; clear on close.
watch(
  () => props.runId,
  id => {
    detail.value = null
    error.value = null
    if (id !== null) {
      loadRun(id)
    } else {
      // Invalidate any in-flight load so a late response cannot repopulate
      // a closed dialog.
      loadSeq++
    }
  },
  { immediate: true }
)

// Resolve a case's prompt text through the suites the parent already loaded.
function casePrompt(caseId: number): string {
  for (const suite of props.suites) {
    const c = suite.cases.find(c => c.id === caseId)
    if (c) return c.prompt
  }
  return `#${caseId}`
}

function scoreClass(score: number | null): string {
  if (score === null) return 'score-none'
  if (score >= 0.8) return 'score-high'
  if (score >= 0.5) return 'score-mid'
  return 'score-low'
}

// A failed row is retryable only when its model resolves to a database id
// (deleted models stay visible as history but cannot be re-asked).
function unitRetryable(row: EvalResult): boolean {
  return (props.modelDbIds?.[row.model_id] ?? 0) > 0
}

const retryingResultId = ref<number | null>(null)
const retryingAll = ref(false)

// Re-run exactly this row's (model, case) unit. The server skips it with a
// count when a concurrent judge already scored it — then there is nothing
// to refresh, so the dialog stays open on that answer.
async function onRetryUnit(row: EvalResult) {
  if (!detail.value) return
  const modelDbId = props.modelDbIds?.[row.model_id]
  if (!modelDbId) return
  try {
    await ElMessageBox.confirm(
      `将重新评估「${row.model_id}」的这道题(只补这一道未判分的题,已判分结果不变),期间批次回到运行中。`,
      '重跑失败项',
      { confirmButtonText: '开始重跑', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return // cancelled — no feedback needed
  }
  retryingResultId.value = row.id
  try {
    const ack = await retryCampaignUnits(detail.value.campaign_id, [
      { model_db_id: modelDbId, case_id: row.case_id },
    ])
    if (ack.accepted === 0) {
      ElMessage.info('该项已有判分结果,无需重跑')
      return
    }
    ElMessage.success('已发起重跑,批次回到运行中')
    emit('retried')
    emit('close')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    retryingResultId.value = null
  }
}

// Re-run every failed unit of the batch (the existing retry-failed path),
// same confirm + feedback caliber as the report header's entry.
async function onRetryAll() {
  if (!detail.value) return
  const campaignId = detail.value.campaign_id
  try {
    await ElMessageBox.confirm(
      `将重新评估批次 #${campaignId} 的全部失败项(只补未判分的题目,已判分结果不变),期间批次回到运行中。`,
      '重跑全部失败项',
      { confirmButtonText: '开始重跑', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return // cancelled — no feedback needed
  }
  retryingAll.value = true
  try {
    await retryCampaignFailed(campaignId)
    ElMessage.success(`已发起批次 #${campaignId} 的失败项重跑`)
    emit('retried')
    emit('close')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    retryingAll.value = false
  }
}
</script>

<style scoped>
.load-error {
  padding: 24px 0;
  text-align: center;
}
.load-error-text {
  font-size: var(--hs-text-md);
  color: var(--hs-danger-text);
  margin: 0 0 12px;
  word-break: break-all;
}
.run-meta {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.expand-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 16px;
}
.expand-label {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  margin-bottom: 2px;
}
.expand-text {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-primary);
  background: var(--hs-bg-subtle);
  border-radius: var(--hs-radius-sm);
  padding: 8px;
}
/* Score band text colors follow the band mapping; on the v2 palette the
   bases are graphic-tier, so text consumes the deepened *-text steps
   (ScoreCell precedent). */
.score-high {
  color: var(--hs-success-text);
}
.score-mid {
  color: var(--hs-warning-text);
}
.score-low {
  color: var(--hs-danger-text);
}
.score-none {
  color: var(--hs-text-placeholder);
}
.deleted-tag {
  margin-left: 4px;
}
</style>
