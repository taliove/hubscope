<template>
  <el-card shadow="hover" class="endpoint-card" :class="`card-${entry.status}`" @click="goDetail">
    <div class="card-head">
      <span class="model-id" :title="entry.model_id">{{ entry.model_id }}</span>
      <el-tag :type="entry.protocol === 'anthropic' ? 'success' : 'warning'" size="small">
        {{ entry.protocol }}
      </el-tag>
    </div>

    <div class="card-status">
      <el-tooltip :content="entry.status_reason" placement="top" :show-after="200">
        <span class="status-wrap">
          <StatusBadge :status="entry.status" />
        </span>
      </el-tooltip>
      <el-tag v-if="!entry.enabled" type="info" size="small">已停用</el-tag>
    </div>

    <div class="card-metrics">
      <div class="metric">
        <span class="metric-label">24h 成功率</span>
        <span class="metric-value">{{ formatPercent(entry.success_rate_24h) }}</span>
      </div>
      <div class="metric">
        <span class="metric-label">P50</span>
        <span class="metric-value">{{ formatMs(entry.p50_ms) }}</span>
      </div>
      <div class="metric">
        <span class="metric-label">P95</span>
        <span class="metric-value">{{ formatMs(entry.p95_ms) }}</span>
      </div>
    </div>

    <div class="card-foot">最近探测:{{ formatTime(entry.last_probe_at) }}</div>
  </el-card>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import type { OverviewEntry } from '@/api/types'
import StatusBadge from './StatusBadge.vue'
import { formatPercent, formatMs, formatTime } from '@/utils/format'

// One card of the status matrix: a single Endpoint with its 24h summary.
// Clicking navigates to the endpoint detail page.
const props = defineProps<{ entry: OverviewEntry }>()
const router = useRouter()

function goDetail() {
  router.push(`/endpoints/${props.entry.endpoint_id}`)
}
</script>

<style scoped>
.endpoint-card {
  border-top: 3px solid transparent;
  cursor: pointer;
}
/* A thin colored edge mirrors the status light for quick scanning. */
.card-healthy {
  border-top-color: #67c23a;
}
.card-degraded {
  border-top-color: #e6a23c;
}
.card-down {
  border-top-color: #f56c6c;
}
.card-failing {
  border-top-color: #ff4500;
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}
.model-id {
  font-weight: 600;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card-status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.status-wrap {
  cursor: help;
}
.card-metrics {
  display: flex;
  gap: 16px;
  margin-bottom: 12px;
}
.metric {
  display: flex;
  flex-direction: column;
}
.metric-label {
  font-size: 12px;
  color: #909399;
}
.metric-value {
  font-size: 15px;
  color: #303133;
}
.card-foot {
  font-size: 12px;
  color: #909399;
}
</style>
