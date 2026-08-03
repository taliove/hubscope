<template>
  <!-- Shared board header of an unfinished batch (2026-08-03 design review,
       GH live-board batch): the view switch (实时分数 default first), the
       batch-level progress line and the console cost summary, elevated from
       EvalProgressGrid so batch meta stays visible on the default scores
       view. Readonly mode (shared report page, ticket 54) drops the switch
       and the cost — progress metadata is all the shared boundary
       publishes. -->
  <div class="board-header">
    <div class="header-row">
      <el-radio-group
        v-if="!readonly"
        :model-value="view"
        size="small"
        @update:model-value="emit('update:view', $event as EvalBoardView)"
      >
        <el-radio-button value="scores">实时分数</el-radio-button>
        <el-radio-button value="grid">进度网格</el-radio-button>
      </el-radio-group>
      <span class="batch-note">
        批次{{ statusWord }}:已判分 {{ units.judged }}/{{ units.expected }} 题 · 已结束 {{ report.progress.done + report.progress.failed }}/{{ report.progress.total }} 个运行<template v-if="report.progress.failed > 0">(失败 {{ report.progress.failed }})</template>
      </span>
      <span v-if="!readonly && costText" class="batch-note">{{ costText }}</span>
      <!-- Batch timing (2026-08-03 live-board batch): start, ETA from the
           observed pace spread over the worker pool, actual finish. -->
      <span class="batch-note timing-note">
        开始 {{ formatTimeMinute(report.started_at) }}<template v-if="report.finished_at"> · 实际结束 {{ formatTimeMinute(report.finished_at) }}</template><template v-else-if="etaText"> · {{ etaText }}</template>
      </span>
    </div>
    <el-progress
      :percentage="progressPercent"
      :status="report.progress.failed > 0 ? 'exception' : undefined"
      class="batch-progress"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { CampaignReport, EvalBoardView } from '@/api/types'
import { batchCostSummary } from '@/utils/evalCost'
import { avgUnitMs, batchRemainingMs, unitProgress } from '@/utils/batchEta'
import { formatDuration, formatTimeMinute } from '@/utils/format'
import { getSettings } from '@/api/settings'

// Batch-level header, view-agnostic: the progress bar and counts are batch
// metadata, not grid content, so they live above both views (design ruling:
// the two per-component radio copies retire into this single source).
const props = withDefaults(
  defineProps<{
    view: EvalBoardView
    report: CampaignReport
    readonly?: boolean
  }>(),
  { readonly: false },
)

const emit = defineEmits<{
  (e: 'update:view', view: EvalBoardView): void
}>()

// Progress caliber (2026-08-03): judged units, not settled runs — under
// model-major feeding every run settles near the tail, so a run-count bar
// pins at zero for most of the batch.
const units = computed(() => unitProgress(props.report))
const progressPercent = computed(() => {
  const u = units.value
  if (u.expected === 0) return 0
  return Math.round((u.judged / u.expected) * 100)
})

// Batch ETA: remaining unit-seconds spread over the eval worker pool. The
// pool size comes from settings once; the estimate hides until a pace
// exists (first judged case).
const concurrency = ref(4)
onMounted(() => {
  getSettings()
    .then((s) => {
      concurrency.value = Math.max(1, s.eval_concurrency)
    })
    .catch(() => {})
})
const etaText = computed(() => {
  if (avgUnitMs(props.report) === null) return ''
  const remaining = batchRemainingMs(props.report, concurrency.value)
  if (remaining === null) return ''
  return `预估剩余 ${formatDuration(remaining)}(按当前速度)`
})

// Batch/campaign status vocabulary (ui-guidelines §7), never mixed with the
// endpoint status words.
const statusWord = computed(() => (props.report.status === 'pending' ? '等待中' : '运行中'))

// Batch cost summary (GH #42), console-only: empty when the payload carries
// no cost (shared/public surfaces omit the fields anyway).
const costText = computed(() => {
  if (!props.report.cost) return ''
  return batchCostSummary(props.report.cost, props.report.started_at, props.report.finished_at, Date.now())
})
</script>

<style scoped>
.board-header {
  margin-bottom: var(--hs-space-4);
}
.header-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.batch-note {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
</style>
