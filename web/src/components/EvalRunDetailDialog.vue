<template>
  <el-dialog
    :model-value="runId !== null"
    :title="dialogTitle"
    width="80%"
    @update:model-value="(v: boolean) => { if (!v) $emit('close') }"
  >
    <div v-loading="loading">
      <template v-if="detail">
        <div class="run-meta">
          <el-tag size="small" :type="detail.trigger === 'scheduled' ? 'info' : 'primary'">
            {{ detail.trigger === 'scheduled' ? '定时' : '手动' }}
          </el-tag>
          <span>裁判模型:{{ detail.judge_model }}</span>
          <span>开始:{{ formatTime(detail.started_at) }}</span>
          <span>结束:{{ formatTime(detail.finished_at) }}</span>
          <span>聚合分:{{ detail.score === null ? '-' : detail.score.toFixed(2) }}</span>
        </div>
        <el-table :data="detail.results" row-key="id" max-height="480">
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
              <span :class="scoreClass(row.score)">{{ row.score === null ? '-' : row.score.toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="判定理由" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ row.verdict_detail ?? '-' }}</template>
          </el-table-column>
          <el-table-column label="延迟" width="90" align="right">
            <template #default="{ row }">{{ formatMs(row.latency_ms) }}</template>
          </el-table-column>
        </el-table>
      </template>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getEvalRun } from '@/api/evals'
import { formatMs, formatTime } from '@/utils/format'
import type { EvalRunDetail, Suite } from '@/api/types'

// Run detail dialog: per-case prompt, model answer, score, and the judge's
// reasoning, grouped in an expandable table.
const props = defineProps<{
  runId: number | null
  suites: Suite[]
}>()

defineEmits<{ close: [] }>()

const detail = ref<EvalRunDetail | null>(null)
const loading = ref(false)

const dialogTitle = computed(() => {
  if (!detail.value) return '评估运行详情'
  const suite = props.suites.find(s => s.id === detail.value!.suite_id)
  return `评估运行 #${detail.value.id} · ${suite?.name ?? ''}`
})

// Load the run whenever a new one is opened; clear on close.
watch(
  () => props.runId,
  async id => {
    detail.value = null
    if (id === null) return
    loading.value = true
    try {
      detail.value = await getEvalRun(id)
    } finally {
      loading.value = false
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
</script>

<style scoped>
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
  background: var(--hs-bg-page);
  border-radius: var(--hs-radius-sm);
  padding: 8px;
}
/* Score band text colors match ScoreMatrix (ui-guidelines §3). */
.score-high {
  color: var(--el-color-success-dark-2);
}
.score-mid {
  color: var(--el-color-warning-dark-2);
}
.score-low {
  color: var(--el-color-danger-dark-2);
}
.score-none {
  color: var(--hs-text-placeholder);
}
.deleted-tag {
  margin-left: 4px;
}
</style>
