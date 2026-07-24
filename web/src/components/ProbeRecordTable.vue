<template>
  <el-table :data="records" :size="compact ? 'small' : 'default'" empty-text="暂无探测记录">
    <el-table-column label="类型" width="90">
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
    <el-table-column label="HTTP" prop="http_status" width="70" />
    <el-table-column label="错误摘要" min-width="200" show-overflow-tooltip>
      <template #default="{ row }">
        <span v-if="row.error_summary" class="fail-text">{{ row.error_summary }}</span>
        <span v-else class="muted">-</span>
      </template>
    </el-table-column>
    <el-table-column label="延迟(ms)" width="100">
      <template #default="{ row }">{{ formatMetric(row.latency_ms) }}</template>
    </el-table-column>
    <el-table-column label="TTFT(ms)" width="100">
      <template #default="{ row }">{{ formatMetric(row.ttft_ms) }}</template>
    </el-table-column>
    <el-table-column label="输入 token" width="100">
      <template #default="{ row }">{{ formatMetric(row.input_tokens) }}</template>
    </el-table-column>
    <el-table-column label="输出 token" width="100">
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
withDefaults(defineProps<{ records: ProbeRecord[]; compact?: boolean }>(), { compact: true })
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
