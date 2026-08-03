<template>
  <!-- Cost view of a campaign report: the model x suite matrix whose cells
       carry the latency + token sums already present on the report cells
       (GH #42), plus the batch-level summary line. Cost numbers are neutral
       operational metrics — never band-colored, never read as quality
       (ui-guidelines §5 成本指标条). Rendered only when the payload carries
       cost fields (console); a pending cell (nothing recorded yet on a
       running batch) renders a dash. -->
  <div class="cost-matrix">
    <div class="cost-matrix-head">
      <span class="cost-matrix-title">成本</span>
      <span class="cost-matrix-summary">{{ summary }}</span>
    </div>
    <el-table :data="report.rows" size="small">
      <el-table-column label="模型" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <span>{{ row.model_id }}</span>
          <span class="model-family">{{ row.family }}</span>
        </template>
      </el-table-column>
      <el-table-column
        v-for="suite in report.suites"
        :key="suite.key"
        :label="suite.name"
        min-width="160"
        align="right"
      >
        <template #default="{ row }">{{ cellText(row, suite.key) }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { CampaignReport, ReportRow } from '@/api/types'
import { cellCostText } from '@/utils/scoreTier'
import { batchCostSummary } from '@/utils/evalCost'

const props = defineProps<{ report: CampaignReport }>()

// Batch summary row (main ruling: judging time and wall-clock side by side,
// plus the token split) — the same evalCost aggregation the board header
// and the cost detail table's title row use.
const summary = computed(() => {
  const cost = props.report.cost
  if (!cost) return ''
  return batchCostSummary(cost, props.report.started_at, props.report.finished_at, Date.now())
})

// Cell = latency + token total ("耗时 X · Token Y", the shared scoreTier
// cost fragment). A cell that never recorded a result (pending on a running
// batch) or carries no cost fields renders a dash.
function cellText(row: ReportRow, suiteKey: string): string {
  const cell = row.cells.find(c => c.suite_key === suiteKey)
  if (!cell || cell.status === 'pending') return '-'
  return cellCostText(cell) || '-'
}
</script>

<style scoped>
/* Light container (v2 Apple syntax): white surface, 1px border, radius-lg,
   no shadow — same caliber as the report page's cost detail block. */
.cost-matrix {
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
  padding: var(--hs-space-5) var(--hs-space-6);
  margin-bottom: var(--hs-space-4);
}
@media (max-width: 1023px) {
  .cost-matrix {
    padding: var(--hs-space-4);
  }
}
.cost-matrix-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.cost-matrix-title {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.cost-matrix-summary {
  margin-left: auto;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.model-family {
  margin-left: 6px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
</style>
