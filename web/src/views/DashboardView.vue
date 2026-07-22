<template>
  <div class="dashboard">
    <!-- First screen: one-sentence global health conclusion. The banner
         always reflects the unfiltered global picture. -->
    <HealthBanner
      :entries="entries"
      :enabled-endpoints="enabledEndpoints"
      :availability24h="availability24h"
      :generated-at="generatedAt"
      :loading="loading"
      :stale="error !== null"
      @inspect="onBannerInspect"
    />

    <!-- Stats strip (replaces the old summary cards): one slim row of counts.
         Status items keep the click-to-filter / click-again-to-clear toggle;
         the active item gets a brand underline instead of a card frame. -->
    <div class="stats-strip">
      <span
        class="stat-item stat-clickable"
        :class="{ 'stat-active': statusFilter === '' }"
        @click="statusFilter = ''"
      >
        总数 <span class="stat-num">{{ entries.length }}</span>
      </span>
      <span
        v-for="status in STRIP_ORDER"
        :key="status"
        class="stat-item stat-clickable"
        :class="{ 'stat-active': statusFilter === status }"
        @click="toggleStatusFilter(status)"
      >
        <StatusBadge :status="status" />
        <span class="stat-num">{{ statusCounts[status] }}</span>
      </span>
      <span v-if="disabledCount > 0" class="stat-item stat-disabled">
        已停用 <span class="stat-num">{{ disabledCount }}</span>
      </span>
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
      <el-select v-model="grouping" class="filter-select">
        <el-option label="按厂商分组" value="family" />
        <el-option label="按能力分组" value="capability" />
        <el-option label="按协议" value="protocol" />
        <el-option label="不分组" value="none" />
      </el-select>
    </div>

    <el-alert
      v-if="error"
      :title="`刷新失败:${error}`"
      type="error"
      :closable="false"
      class="error-alert"
    />

    <div ref="matrixRef">
      <!-- Grouped status matrix: one collapsible section per group. -->
      <template v-if="grouping !== 'none'">
        <OverviewGroupSection
          v-for="section in groupSections"
          :key="section.group.key"
          :group="section.group"
          :entries="section.entries"
        />
        <el-empty v-if="groupSections.length === 0 && !loading" description="暂无匹配的 Endpoint" />
      </template>

      <!-- Flat status matrix: one card per endpoint. -->
      <div v-else class="card-grid" v-loading="loading && entries.length === 0">
        <EndpointCard v-for="entry in filteredEntries" :key="entry.endpoint_id" :entry="entry" />
        <el-empty v-if="filteredEntries.length === 0 && !loading" description="暂无匹配的 Endpoint" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useOverview } from '@/composables/useOverview'
import HealthBanner from '@/components/HealthBanner.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import EndpointCard from '@/components/EndpointCard.vue'
import OverviewGroupSection from '@/components/OverviewGroupSection.vue'
import type { EndpointStatus, Protocol, OverviewGroup, OverviewEntry } from '@/api/types'

const { entries, byFamily, byCapability, byProtocol, generatedAt, enabledEndpoints, availability24h, loading, error, statusCounts, start } = useOverview()

// Display order of the stats strip: mildest to most severe.
const STRIP_ORDER: EndpointStatus[] = ['healthy', 'degraded', 'down', 'failing']

const keyword = ref('')
const protocolFilter = ref<Protocol | ''>('')
const statusFilter = ref<EndpointStatus | ''>('')
// Grouping dimension of the status matrix; vendor family by default.
const grouping = ref<'family' | 'capability' | 'protocol' | 'none'>('family')
const matrixRef = ref<HTMLElement | null>(null)

// Disabled endpoints show up in the strip as a non-clickable count.
const disabledCount = computed(() => entries.value.filter(e => !e.enabled).length)

// Clicking a stats item filters the matrix to that status; clicking the
// active one clears the filter.
function toggleStatusFilter(status: EndpointStatus) {
  statusFilter.value = statusFilter.value === status ? '' : status
}

// Abnormal-banner click: apply the status filter and scroll to the matrix.
function onBannerInspect(status: EndpointStatus) {
  statusFilter.value = status
  matrixRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

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

// Pair each group aggregate with its filtered entries. Groups with no
// matching entries after filtering stay visible (they show an empty hint).
const groupSections = computed<{ group: OverviewGroup; entries: OverviewEntry[] }[]>(() => {
  const groups =
    grouping.value === 'family'
      ? byFamily.value
      : grouping.value === 'capability'
        ? byCapability.value
        : byProtocol.value
  const keyOf = (e: OverviewEntry) =>
    grouping.value === 'family' ? e.family : grouping.value === 'capability' ? e.capability : e.protocol
  return groups.map(group => ({
    group,
    entries: filteredEntries.value.filter(e => keyOf(e) === group.key),
  }))
})

onMounted(start)
</script>

<style scoped>
.dashboard {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}
.stats-strip {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  margin-bottom: 16px;
}
.stat-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.stat-clickable {
  cursor: pointer;
  padding-bottom: 2px;
  border-bottom: 2px solid transparent;
  transition: color 0.15s ease, border-color 0.15s ease;
}
.stat-clickable:hover {
  color: var(--hs-brand-hover);
}
.stat-active {
  color: var(--hs-brand);
  border-bottom-color: var(--hs-brand);
}
.stat-num {
  font-weight: 600;
  color: var(--hs-text-primary);
}
.stat-disabled {
  color: var(--hs-text-secondary);
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
