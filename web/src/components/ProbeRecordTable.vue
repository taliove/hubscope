<template>
  <el-table :data="records" :size="compact ? 'small' : 'default'" :empty-text="emptyText">
    <el-table-column v-if="!slim" label="类型" width="90">
      <template #default="{ row }">
        {{ row.streaming ? '流式' : '非流式' }}
      </template>
    </el-table-column>
    <el-table-column label="结果" width="70">
      <template #default="{ row }">
        <span v-if="row.ok" class="ok">✓</span>
        <span v-else class="fail">✗</span>
      </template>
    </el-table-column>
    <el-table-column v-if="!slim" label="HTTP" prop="http_status" width="70" />
    <el-table-column label="错误摘要" min-width="200" show-overflow-tooltip>
      <template #default="{ row }">
        <span v-if="row.error_summary" class="fail-text">{{ row.error_summary }}</span>
        <span v-else class="muted">-</span>
      </template>
    </el-table-column>
    <el-table-column label="延迟(ms)" width="100">
      <template #default="{ row }">{{ formatMetric(row.latency_ms) }}</template>
    </el-table-column>
    <el-table-column v-if="!slim" label="TTFT(ms)" width="100">
      <template #default="{ row }">{{ formatMetric(row.ttft_ms) }}</template>
    </el-table-column>
    <el-table-column v-if="!slim" label="输入 token" width="100">
      <template #default="{ row }">{{ formatMetric(row.input_tokens) }}</template>
    </el-table-column>
    <el-table-column v-if="!slim" label="输出 token" width="100">
      <template #default="{ row }">{{ formatMetric(row.output_tokens) }}</template>
    </el-table-column>
    <el-table-column label="时间" width="180">
      <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import type { ProbeRecord } from '@/api/types'
import { formatTime, formatMetric } from '@/utils/format'

// Shared table rendering a list of ProbeRecords (probe run or history).
// Density follows the surface (ui-guidelines §2): admin consoles keep the
// compact 12px tier (`compact`, default), the public status board passes
// `:compact="false"` for the roomier default row height.
// slim (2026-07-29 main 裁决, ui-guidelines §4 禁横向滚动; 与 surface brief
// EndpointQuickViewDialog 节互指): the quick-view variant — drops the five
// detail-page material columns (类型/HTTP/TTFT/输入 token/输出 token), keeps
// 结果/错误摘要/延迟/时间 (≈550px, arithmetic fit for the 640px dialog).
// slim and compact are orthogonal axes: column trim vs. row density.
withDefaults(
  defineProps<{ records: ProbeRecord[]; compact?: boolean; slim?: boolean; emptyText?: string }>(),
  { compact: true, slim: false, emptyText: '暂无探测记录' },
)
</script>

<style scoped>
.ok {
  color: var(--hs-success);
  font-weight: 600;
}
.fail {
  color: var(--hs-danger);
  font-weight: 600;
}
.fail-text {
  color: var(--hs-danger);
}
.muted {
  color: var(--hs-text-placeholder);
}
</style>
