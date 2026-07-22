<template>
  <el-card shadow="never" class="matrix-card">
    <div class="card-title">模型 × 评估集得分对比</div>
    <el-table :data="rows" v-loading="loading" class="matrix-table">
      <el-table-column label="模型" min-width="220">
        <template #default="{ row }">
          <span class="model-name" :title="row.modelId">{{ row.modelId }}</span>
        </template>
      </el-table-column>
      <el-table-column
        v-for="suite in suites"
        :key="suite.id"
        :label="suite.name"
        min-width="120"
        align="center"
      >
        <template #default="{ row }">
          <span
            class="score-cell"
            :class="scoreClass(row.scores[suite.id])"
            @click="onCellClick(row, suite)"
          >
            {{ formatScore(row.scores[suite.id]) }}
          </span>
        </template>
      </el-table-column>
    </el-table>
    <div class="matrix-hint">点击得分可查看该模型在该评估集上的历史趋势</div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { LatestScore, Model, Suite } from '@/api/types'

// Score matrix: rows are chat-capable models, columns are suites, cells hold
// the pair's latest done-run aggregate score with a red/yellow/green scale.
const props = defineProps<{
  suites: Suite[]
  models: Model[]
  latest: LatestScore[]
  loading: boolean
}>()

const emit = defineEmits<{
  select: [selection: { modelId: string; modelDbId: number; suiteId: number; suiteName: string }]
}>()

interface MatrixRow {
  modelDbId: number
  modelId: string
  scores: Record<number, number | null | undefined> // suite id -> score (undefined = no run yet)
}

// Build one row per live chat model. Deleted models are hidden from the
// comparison (the API also omits them from latest scores); their history
// stays visible in run details.
const rows = computed<MatrixRow[]>(() => {
  const byModel = new Map<number, MatrixRow>()
  for (const m of props.models) {
    if (m.capability !== 'chat') continue
    byModel.set(m.id, { modelDbId: m.id, modelId: m.model_id, scores: {} })
  }
  for (const ls of props.latest) {
    const row = byModel.get(ls.model_db_id)
    if (!row) continue
    row.scores[ls.suite_id] = ls.score
  }
  return [...byModel.values()].sort((a, b) => a.modelId.localeCompare(b.modelId))
})

// Render a 0~1 score with two decimals; a dash covers both "unscored" (null)
// and "never ran" (undefined).
function formatScore(score: number | null | undefined): string {
  if (score === null || score === undefined) return '-'
  return score.toFixed(2)
}

// Color scale: green >= 0.8, yellow >= 0.5, red below.
function scoreClass(score: number | null | undefined): string {
  if (score === null || score === undefined) return 'score-none'
  if (score >= 0.8) return 'score-high'
  if (score >= 0.5) return 'score-mid'
  return 'score-low'
}

function onCellClick(row: MatrixRow, suite: Suite) {
  if (row.scores[suite.id] === undefined) return
  emit('select', {
    modelId: row.modelId,
    modelDbId: row.modelDbId,
    suiteId: suite.id,
    suiteName: suite.name,
  })
}
</script>

<style scoped>
.matrix-card {
  margin-bottom: 16px;
}
/* Admin console compact density: 12px card padding (ui-guidelines §2). */
.matrix-card {
  --el-card-padding: 12px;
}
.card-title {
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
  margin-bottom: 8px;
}
.model-name {
  font-family: monospace;
  font-size: var(--hs-text-sm);
}
.score-cell {
  display: inline-block;
  min-width: 56px;
  padding: 2px 8px;
  border-radius: var(--hs-radius-sm);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}
/* Score bands: light-9 fills with dark-2 text (ui-guidelines §3). The
   thresholds live in scoreClass above and mirror the backend scale. */
.score-high {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success-dark-2);
}
.score-mid {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning-dark-2);
}
.score-low {
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger-dark-2);
}
.score-none {
  color: var(--hs-text-placeholder);
  cursor: default;
}
.matrix-hint {
  margin-top: 8px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
</style>
