<template>
  <div class="dashboard">
    <!-- First screen: one-sentence global health conclusion. The banner
         always reflects the unfiltered global picture. -->
    <HealthBanner
      :entries="entries"
      :generated-at="generatedAt"
      :loading="loading"
      :stale="error !== null"
      @inspect="onBannerInspect"
    />

    <!-- Stats strip (replaces the old summary cards): one slim row of counts.
         Status items keep the click-to-filter / click-again-to-clear toggle;
         the active item gets a 1px brand inset ring on a transparent ground
         (user feedback 2026-07-29 — no fill block fighting the status dots). -->
    <div class="stats-strip">
      <span
        class="stat-item stat-clickable"
        :class="{ 'stat-active': statusFilter === '' }"
        @click="statusFilter = ''"
      >
        总数 <span class="stat-num">{{ entries.length }}</span>
      </span>
      <span
        v-for="status in SEVERITY_ORDER"
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
      <!-- Inline labels (GH #55, Dashboard-local convention): the selects
           no longer lean on placeholders as their only name. Keyword input
           and grouping select stay as-is. -->
      <span class="filter-label">协议:</span>
      <el-select v-model="protocolFilter" placeholder="全部" clearable class="filter-select">
        <!-- Options come from the single protocol vocabulary (utils/protocol.ts)
             so new protocols appear here automatically. -->
        <el-option v-for="p in PROTOCOLS" :key="p" :label="p" :value="p" />
      </el-select>
      <span class="filter-label">状态:</span>
      <el-select v-model="statusFilter" placeholder="全部" clearable class="filter-select">
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
      <!-- Share the filtered picture as a Status Card PNG; disabled until the
           first load lands (an empty board is not shareable). -->
      <el-button
        class="share-btn"
        :disabled="loading && entries.length === 0"
        @click="openShare"
      >
        <el-icon><Share /></el-icon>
        分享状态
      </el-button>
    </div>

    <StatusShareDialog v-model:visible="shareVisible" :snapshot="shareSnapshot" />

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
          :grouping="grouping"
          @share="openGroupShare(section)"
        />
        <el-empty v-if="groupSections.length === 0 && !loading" description="暂无匹配的 Endpoint" />
      </template>

      <!-- Flat status matrix: one card per endpoint. -->
      <div v-else class="card-grid" v-loading="loading && entries.length === 0">
        <EndpointCard v-for="entry in filteredEntries" :key="entry.endpoint_id" :entry="entry" />
        <el-empty v-if="filteredEntries.length === 0 && !loading" description="暂无匹配的 Endpoint" />
      </div>

      <!-- Quiet admin entry (ticket 90): the shared PublicFooter. -->
      <PublicFooter />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Share } from '@element-plus/icons-vue'
import { useOverview } from '@/composables/useOverview'
import HealthBanner from '@/components/HealthBanner.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import EndpointCard from '@/components/EndpointCard.vue'
import OverviewGroupSection from '@/components/OverviewGroupSection.vue'
import StatusShareDialog from '@/components/StatusShareDialog.vue'
import PublicFooter from '@/components/PublicFooter.vue'
import type { StatusCardSnapshot } from '@/utils/statusCardSnapshot'
import { PROTOCOLS } from '@/utils/protocol'
import { sortEntriesBySeverity, sortGroupSections, SEVERITY_ORDER, type GroupSection } from '@/utils/severitySort'
import type { EndpointStatus, Protocol, OverviewGroup, OverviewEntry } from '@/api/types'

const { entries, byFamily, byCapability, byProtocol, generatedAt, loading, error, statusCounts, start } = useOverview()

// The stats strip renders SEVERITY_ORDER directly (GH #55): the board's
// single severity caliber, heavy → light, shared with the group header —
// the old local mild→severe STRIP_ORDER is deleted.

const keyword = ref('')
const protocolFilter = ref<Protocol | ''>('')
const statusFilter = ref<EndpointStatus | ''>('')
// Grouping dimension of the status matrix; vendor family by default.
const grouping = ref<'family' | 'capability' | 'protocol' | 'none'>('family')
const matrixRef = ref<HTMLElement | null>(null)

// Share dialog state. The snapshot freezes the filtered set and filter
// conditions at open time so polling cannot change the card mid-preview.
const shareVisible = ref(false)
const shareSnapshot = ref<StatusCardSnapshot | null>(null)

function openShare() {
  shareSnapshot.value = {
    entries: [...filteredEntries.value],
    keyword: keyword.value.trim(),
    protocol: protocolFilter.value,
    status: statusFilter.value,
    group: null,
    generatedAt: new Date().toISOString(),
  }
  shareVisible.value = true
}

// Per-group share (ticket 59): the snapshot scope is the group's filtered
// entries; the card leads its scope chips with the group identity so the
// subset can never read as the global picture.
function openGroupShare(section: { group: OverviewGroup; entries: OverviewEntry[] }) {
  if (grouping.value === 'none') return
  shareSnapshot.value = {
    entries: [...section.entries],
    keyword: keyword.value.trim(),
    protocol: protocolFilter.value,
    status: statusFilter.value,
    group: { dimension: grouping.value, key: section.group.key },
    generatedAt: new Date().toISOString(),
  }
  shareVisible.value = true
}

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

// Apply the three filters; an empty filter matches everything. The result
// is severity-ranked (GH #52) so the flat matrix leads with the most severe
// endpoints; group sections re-derive their own ordering from this set.
const filteredEntries = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return sortEntriesBySeverity(
    entries.value.filter(entry => {
      if (kw && !entry.model_id.toLowerCase().includes(kw)) return false
      if (protocolFilter.value && entry.protocol !== protocolFilter.value) return false
      if (statusFilter.value && entry.status !== statusFilter.value) return false
      return true
    }),
  )
})

// Pair each group aggregate with its filtered entries, then severity-rank
// (GH #52): groups by their most severe ENABLED entry (ties by group key,
// empty-after-filter groups sink), entries within each group by severity.
// Filtered-empty groups stay visible as headers and auto-collapse (user
// request 2026-07-29) instead of showing a large empty-state box.
const groupSections = computed<GroupSection[]>(() => {
  const groups =
    grouping.value === 'family'
      ? byFamily.value
      : grouping.value === 'capability'
        ? byCapability.value
        : byProtocol.value
  const keyOf = (e: OverviewEntry) =>
    grouping.value === 'family' ? e.family : grouping.value === 'capability' ? e.capability : e.protocol
  return sortGroupSections(
    groups.map(group => ({
      group,
      entries: filteredEntries.value.filter(e => keyOf(e) === group.key),
    })),
  )
})

onMounted(start)
</script>

<style scoped>
.dashboard {
  max-width: 1200px;
  margin: 0 auto;
  padding: var(--hs-space-5) var(--hs-space-4) var(--hs-space-7);
}
.stats-strip {
  display: flex;
  align-items: center;
  gap: var(--hs-space-4);
  flex-wrap: wrap;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
  margin-bottom: var(--hs-space-4);
}
.stat-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.stat-clickable {
  cursor: pointer;
  padding: var(--hs-space-1) var(--hs-space-2);
  border-radius: var(--hs-radius-sm);
  transition: color var(--hs-transition), background-color var(--hs-transition), box-shadow var(--hs-transition);
}
.stat-clickable:hover {
  color: var(--hs-brand-hover);
}
/* Selected filter = brand outline + transparent ground + brand text
   (user feedback 2026-07-29): brand keeps the "active selection" language,
   an inset ring avoids layout shift, and no fill block fights the status
   dots' semantic colors. */
.stat-active {
  color: var(--hs-brand);
  background-color: transparent;
  box-shadow: inset 0 0 0 1px var(--hs-brand);
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
  gap: var(--hs-space-3);
  margin-bottom: var(--hs-space-4);
  flex-wrap: wrap;
}
.filter-keyword {
  width: 220px;
}
.filter-label {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.filter-select {
  width: 140px;
}
.share-btn {
  margin-left: auto;
}
.error-alert {
  margin-bottom: var(--hs-space-4);
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--hs-space-3);
  min-height: 120px;
}
</style>
