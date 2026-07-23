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

      <!-- Per-group share (ticket 59): text button so it never competes with
           the StatusBadge colors; stop propagation — the whole header row is
           the collapse hot zone. -->
      <el-button text class="group-share" @click.stop="emit('share')">
        <el-icon><Share /></el-icon>
        分享
      </el-button>
    </div>

    <div v-show="!collapsed" class="card-grid">
      <EndpointCard v-for="entry in entries" :key="entry.endpoint_id" :entry="entry" />
      <el-empty v-if="entries.length === 0" description="该组无匹配的 Endpoint" :image-size="60" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Share } from '@element-plus/icons-vue'
import StatusBadge from './StatusBadge.vue'
import EndpointCard from './EndpointCard.vue'
import { formatPercent, formatMs } from '@/utils/format'
import type { OverviewGroup, OverviewEntry, EndpointStatus } from '@/api/types'

const props = defineProps<{ group: OverviewGroup; entries: OverviewEntry[] }>()

const emit = defineEmits<{ (e: 'share'): void }>()

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
  color: var(--hs-text-placeholder);
  width: 14px;
}
.group-key {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.group-count {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.group-stats {
  display: flex;
  align-items: center;
  gap: 6px;
}
.stat-num {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  margin-right: 4px;
}
.group-metrics {
  margin-left: auto;
  display: flex;
  gap: 14px;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.group-share {
  margin-left: 4px;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}
</style>
