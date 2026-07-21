<template>
  <div class="dashboard">
    <header class="dashboard-header">
      <h1>AI Hub Checker</h1>
      <span class="subtitle">状态总览</span>
      <router-link to="/admin" class="nav-link">管理视图</router-link>
    </header>

    <!-- Summary row: total endpoint count plus per-status counts. -->
    <div class="summary-row">
      <el-card shadow="never" class="summary-card">
        <div class="summary-value">{{ entries.length }}</div>
        <div class="summary-label">总端点数</div>
      </el-card>
      <el-card
        v-for="status in STATUS_ORDER"
        :key="status"
        shadow="never"
        class="summary-card"
      >
        <div class="summary-value">
          <StatusBadge :status="status" />
          <span>{{ statusCounts[status] }}</span>
        </div>
      </el-card>
    </div>

    <!-- Filters: model keyword, protocol, status. -->
    <div class="filter-row">
      <el-input
        v-model="keyword"
        placeholder="按模型名过滤"
        clearable
        class="filter-keyword"
      />
      <el-select v-model="protocolFilter" placeholder="协议" clearable class="filter-select">
        <el-option label="anthropic" value="anthropic" />
        <el-option label="openai" value="openai" />
      </el-select>
      <el-select v-model="statusFilter" placeholder="状态" clearable class="filter-select">
        <el-option label="正常" value="healthy" />
        <el-option label="降级" value="degraded" />
        <el-option label="宕机" value="down" />
        <el-option label="告警" value="failing" />
      </el-select>
      <span class="refresh-info">每 10 秒自动刷新<template v-if="generatedAt"> · 更新于 {{ formatTime(generatedAt) }}</template></span>
    </div>

    <el-alert
      v-if="error"
      :title="`刷新失败:${error}`"
      type="error"
      :closable="false"
      class="error-alert"
    />

    <!-- Status matrix: one card per endpoint. -->
    <div class="card-grid" v-loading="loading && entries.length === 0">
      <EndpointCard v-for="entry in filteredEntries" :key="entry.endpoint_id" :entry="entry" />
      <el-empty v-if="filteredEntries.length === 0 && !loading" description="暂无匹配的 Endpoint" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useOverview } from '@/composables/useOverview'
import StatusBadge from '@/components/StatusBadge.vue'
import EndpointCard from '@/components/EndpointCard.vue'
import { formatTime } from '@/utils/format'
import type { EndpointStatus, Protocol } from '@/api/types'

const { entries, generatedAt, loading, error, statusCounts, STATUS_ORDER, start } = useOverview()

const keyword = ref('')
const protocolFilter = ref<Protocol | ''>('')
const statusFilter = ref<EndpointStatus | ''>('')

// Apply the three filters; an empty filter matches everything.
const filteredEntries = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return entries.value.filter(entry => {
    if (kw && !entry.model_id.toLowerCase().includes(kw)) return false
    if (protocolFilter.value && entry.protocol !== protocolFilter.value) return false
    if (statusFilter.value && entry.status !== statusFilter.value) return false
    return true
  })
})

onMounted(start)
</script>

<style scoped>
.dashboard {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.dashboard-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 20px;
}
.dashboard-header h1 {
  margin: 0;
  font-size: 22px;
  color: #303133;
}
.subtitle {
  color: #909399;
  font-size: 14px;
}
.nav-link {
  margin-left: auto;
  font-size: 14px;
  color: #409eff;
  text-decoration: none;
}
.summary-row {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.summary-card {
  min-width: 120px;
}
.summary-value {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}
.summary-label {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}
.filter-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.filter-keyword {
  width: 220px;
}
.filter-select {
  width: 140px;
}
.refresh-info {
  margin-left: auto;
  font-size: 12px;
  color: #909399;
}
.error-alert {
  margin-bottom: 16px;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
  min-height: 120px;
}
</style>
