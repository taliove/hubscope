<template>
  <el-card shadow="never" class="group-section">
    <div class="group-header" @click="collapsed = !collapsed">
      <span class="group-arrow">{{ collapsed ? '▸' : '▾' }}</span>
      <span class="group-key">{{ group.key }}</span>
      <span class="group-count">{{ group.endpoint_count }} 端点</span>

      <span class="group-stats">
        <template v-for="s in presentStatuses" :key="s">
          <StatusBadge :status="s" />
          <span class="stat-num">{{ group.status_counts[s] }}</span>
        </template>
        <el-tag v-if="group.status_counts['disabled']" type="info" size="small">
          禁用 {{ group.status_counts['disabled'] }}
        </el-tag>
      </span>

      <span class="group-metrics">
        <span>24h 可用率 {{ formatPercent(group.availability_24h) }}</span>
        <span>均延 {{ formatMs(group.avg_latency_ms) }}</span>
      </span>
    </div>

    <div v-show="!collapsed" class="card-grid">
      <EndpointCard v-for="entry in entries" :key="entry.endpoint_id" :entry="entry" />
      <el-empty v-if="entries.length === 0" description="该组无匹配的 Endpoint" :image-size="60" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import StatusBadge from './StatusBadge.vue'
import EndpointCard from './EndpointCard.vue'
import { formatPercent, formatMs } from '@/utils/format'
import type { OverviewGroup, OverviewEntry, EndpointStatus } from '@/api/types'

const props = defineProps<{ group: OverviewGroup; entries: OverviewEntry[] }>()

const collapsed = ref(false)

// Statuses present in this group, in display priority order.
const STATUS_PRIORITY: EndpointStatus[] = ['down', 'failing', 'degraded', 'healthy']
const presentStatuses = computed(() =>
  STATUS_PRIORITY.filter(s => (props.group.status_counts[s] ?? 0) > 0)
)
</script>

<style scoped>
.group-section {
  margin-bottom: 12px;
}
.group-header {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  user-select: none;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.group-arrow {
  color: #909399;
  width: 14px;
}
.group-key {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}
.group-count {
  font-size: 12px;
  color: #909399;
}
.group-stats {
  display: flex;
  align-items: center;
  gap: 6px;
}
.stat-num {
  font-size: 13px;
  color: #606266;
  margin-right: 4px;
}
.group-metrics {
  margin-left: auto;
  display: flex;
  gap: 14px;
  font-size: 12px;
  color: #606266;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}
</style>
