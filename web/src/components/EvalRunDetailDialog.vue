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
          <!-- Retry-any (2026-08-04 ruling): every answered row can be
               re-answered and re-judged — singly or in a checked batch,
               on running and settled batches alike. -->
          <el-button v-if="retryable" size="small" :loading="retryingAll" @click="onRetryAll">
            重跑全部失败项
          </el-button>
          <el-button
            v-if="retryable"
            size="small"
            type="primary"
            :disabled="selectedRows.length === 0"
            :loading="retryingSelected"
            @click="onRetrySelected"
          >
            重跑选中({{ selectedRows.length }})
          </el-button>
        </div>
        <!-- Jury rail (GH #179 view B「裁判席」): the viewed model's own
             ≤3 judges (per-model juries differ — each excludes its
             subject), each with vote count, failures and priced cost. -->
        <div v-if="railJudges.length > 0" class="jury-rail">
          <span class="rail-label">
            裁判团<template v-if="detail.jury_models">({{ detail.jury_models.policy }})</template>
          </span>
          <span v-for="j in railJudges" :key="j.judge_model" class="jury-card">
            <b>{{ j.judge_model }}</b>
            <em>{{ j.votes }} 票 · 失败 {{ j.fails }} · {{ j.cost === null ? '价格未登记' : '$' + j.cost.toFixed(4) }}</em>
          </span>
        </div>
        <el-table
          :data="visibleResults"
          row-key="id"
          max-height="480"
          @selection-change="(rows: EvalResult[]) => (selectedRows = rows)"
        >
          <el-table-column
            v-if="retryable"
            type="selection"
            width="36"
            :selectable="(row: EvalResult) => row.model_db_id > 0"
          />
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
                <!-- Jury breakdown (GH #179 view B「裁判席」): every jury
                     vote of the case's latest attempt, with the spread as
                     the disagreement signal. -->
                <div v-if="row.judge_scores && row.judge_scores.length > 0" class="expand-block">
                  <div class="expand-label">
                    裁判团
                    <el-tag
                      v-if="row.spread !== null && row.spread !== undefined && row.spread > 0.12"
                      size="small"
                      type="danger"
                      class="spread-tag"
                    >分歧 {{ row.spread.toFixed(2) }}</el-tag>
                    <el-tag
                      v-else-if="row.spread !== null && row.spread !== undefined"
                      size="small"
                      type="info"
                      class="spread-tag"
                    >分歧 {{ row.spread.toFixed(2) }}</el-tag>
                  </div>
                  <div class="jury-votes">
                    <span
                      v-for="(v, i) in row.judge_scores"
                      :key="i"
                      class="vote-chip"
                      :class="{ fail: v.score === null }"
                    >
                      {{ v.judge_model }} {{ v.score === null ? 'FAIL' : v.score.toFixed(2) }}
                      <em>样本 {{ v.sample_no }}</em>
                    </span>
                  </div>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="模型" min-width="170" show-overflow-tooltip>
            <template #default="{ row }">
              <VendorTile :family="row.family" />
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
          <!-- Per-judge vote columns (GH #179 view B): only judge-verdict
               rows carry votes; rule rows render a dash. -->
          <el-table-column
            v-for="j in judgeColumns"
            :key="j"
            :label="j"
            min-width="110"
            align="center"
            show-overflow-tooltip
          >
            <template #default="{ row }">
              <span :class="{ 'vote-fail-cell': voteIsFail(row, j) }">{{ voteFor(row, j) }}</span>
            </template>
          </el-table-column>
          <el-table-column v-if="judgeColumns.length > 0" label="分歧" width="80" align="center">
            <template #default="{ row }">
              <span v-if="row.spread !== null && row.spread !== undefined" :class="{ 'spread-hot': row.spread > 0.12 }">
                {{ row.spread.toFixed(2) }}
              </span>
              <span v-else class="dim">—</span>
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
                v-if="unitRetryable(row)"
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
import VendorTile from '@/components/VendorTile.vue'
import { retryCampaignFailed, retryCampaignUnits } from '@/api/campaigns'
import { formatMs, formatScore, formatTime } from '@/utils/format'
import type { EvalResult, EvalRunDetail, JurySummaryEntry, Suite } from '@/api/types'

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

// The jury's judge columns, in panel order (GH #179 view B).
const judgeColumns = computed<string[]>(() => {
  return (detail.value?.jury_summary ?? []).map(j => j.judge_model)
})

// The rail shows the viewed model's own jury (≤3): per-subject juries
// differ because each excludes its subject, so a union would read as a
// 4-5 judge panel (2026-08-04 review). Unfiltered runs show the first
// subject's panel — its tallies stay per-judge factual either way.
const railJudges = computed<JurySummaryEntry[]>(() => {
  const summary = detail.value?.jury_summary ?? []
  const juries = detail.value?.jury_models?.juries ?? {}
  const rows = detail.value?.results ?? []
  const dbid = props.modelId
    ? rows.find(r => r.model_id === props.modelId)?.model_db_id
    : rows[0]?.model_db_id
  const panel = dbid !== undefined ? (juries[String(dbid)] ?? null) : null
  if (!panel) return summary
  return summary.filter(j => panel.includes(j.judge_model))
})

// One row's vote text for one judge column: per-sample scores joined, FAIL
// for a failed judge call, a dash for rule-verdict rows.
function voteFor(row: EvalResult, judge: string): string {
  const votes = (row.judge_scores ?? []).filter(v => v.judge_model === judge)
  if (votes.length === 0) return '—'
  return votes.map(v => (v.score === null ? 'FAIL' : v.score.toFixed(2))).join(' / ')
}

function voteIsFail(row: EvalResult, judge: string): boolean {
  return (row.judge_scores ?? []).some(v => v.judge_model === judge && v.score === null)
}

// A row is retryable when its model still resolves (deleted models stay
// visible as history but cannot be re-asked).
function unitRetryable(row: EvalResult): boolean {
  return row.model_db_id > 0
}

const retryingResultId = ref<number | null>(null)
const retryingAll = ref(false)
const retryingSelected = ref(false)
const selectedRows = ref<EvalResult[]>([])

// Re-run exactly this row's (model, case) unit: the answer is re-asked and
// re-judged (2026-08-04 ruling — any score, not just nulls).
async function onRetryUnit(row: EvalResult) {
  if (!detail.value) return
  if (!unitRetryable(row)) return
  try {
    await ElMessageBox.confirm(
      `将重新作答并重新裁判「${row.model_id}」的这道题,当前结果作废重算。`,
      '重跑该题',
      { confirmButtonText: '开始重跑', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return // cancelled — no feedback needed
  }
  retryingResultId.value = row.id
  try {
    const ack = await retryCampaignUnits(detail.value.campaign_id, [
      { model_db_id: row.model_db_id, case_id: row.case_id },
    ])
    if (ack.accepted === 0) {
      ElMessage.info('该项还在作答中,无需重跑')
      return
    }
    ElMessage.success('已发起重跑')
    emit('retried')
    emit('close')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    retryingResultId.value = null
  }
}

// Re-run every checked row's unit in one request.
async function onRetrySelected() {
  if (!detail.value || selectedRows.value.length === 0) return
  const rows = selectedRows.value.filter(unitRetryable)
  try {
    await ElMessageBox.confirm(
      `将重新作答并重新裁判选中的 ${rows.length} 道题,当前结果作废重算。`,
      '重跑选中',
      { confirmButtonText: '开始重跑', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  retryingSelected.value = true
  try {
    const ack = await retryCampaignUnits(
      detail.value.campaign_id,
      rows.map(r => ({ model_db_id: r.model_db_id, case_id: r.case_id })),
    )
    if (ack.accepted === 0) {
      ElMessage.info('选中项都还在作答中,无需重跑')
      return
    }
    ElMessage.success(`已发起重跑 ${ack.accepted} 项`)
    emit('retried')
    emit('close')
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    retryingSelected.value = false
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
.spread-tag {
  margin-left: 8px;
}
.jury-votes {
  display: flex;
  flex-wrap: wrap;
  gap: var(--hs-space-2);
}
.vote-chip {
  background: var(--hs-blue-50);
  color: var(--hs-gray-900);
  border-radius: 8px;
  padding: 3px 10px;
  font-size: 12px;
}
.vote-chip.fail {
  background: #ffecea;
  color: var(--hs-danger-text-base);
}
.vote-chip em {
  font-style: normal;
  color: var(--hs-gray-500);
  margin-left: 6px;
}
.jury-rail {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--hs-space-2);
  margin-bottom: var(--hs-space-3);
  font-size: 13px;
}
.rail-label {
  color: var(--hs-gray-700);
  font-weight: 600;
}
.jury-card {
  background: var(--hs-gray-50);
  border: 1px solid var(--hs-gray-100);
  border-radius: 8px;
  padding: 4px 10px;
  display: inline-flex;
  flex-direction: column;
}
.jury-card b {
  color: var(--hs-gray-900);
  font-size: 12px;
}
.jury-card em {
  font-style: normal;
  color: var(--hs-gray-500);
  font-size: 12px;
}
.vote-fail-cell {
  color: var(--hs-danger-text-base);
  font-weight: 600;
}
.spread-hot {
  color: var(--hs-danger-text-base);
  font-weight: 600;
}
.dim {
  color: var(--hs-gray-400);
}
</style>
