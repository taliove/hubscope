<template>
  <div
    class="health-banner"
    :class="[isInitialLoading || isEmpty ? 'banner-loading' : `banner-${tone}`, { 'banner-clickable': clickable }]"
    @click="onClick"
  >
    <!-- Fixed-height skeleton for the first load so the banner never jumps. -->
    <div v-if="isInitialLoading" class="banner-skeleton" aria-hidden="true">
      <div class="skeleton-line skeleton-title" />
      <div class="skeleton-line skeleton-sub" />
    </div>
    <template v-else>
      <div class="conclusion-row">
        <!-- Failing endpoints reuse the StatusBadge alert blink (W5 semantics). -->
        <span v-if="hasFailing" class="alert-dot" title="存在告警端点" />
        <span class="conclusion">{{ conclusion }}</span>
        <span v-if="stale" class="stale-note">数据非最新</span>
      </div>
      <div class="subtext">{{ subtext }}</div>
    </template>
  </div>
</template>

<script setup lang="ts">
// Global health banner: the first visual layer of the Dashboard. Renders one
// sentence conclusion (all healthy / N degraded / N abnormal) plus a subtext
// line with 24h availability, endpoint counts and the update time. Four
// states per spec 0003 §5.1: healthy / degraded / abnormal / loading.
// The banner always reflects the global picture and never the active filters.
import { computed } from 'vue'
import type { OverviewEntry, EndpointStatus } from '@/api/types'
import { formatPercent, formatTime } from '@/utils/format'
import { POLL_INTERVAL_MS } from '@/composables/useOverview'

const props = defineProps<{
  entries: OverviewEntry[]
  enabledEndpoints: number | null
  availability24h: number | null
  generatedAt: string | null
  loading: boolean
  stale: boolean // last refresh failed; showing previous successful data
}>()

const emit = defineEmits<{ (e: 'inspect', status: EndpointStatus): void }>()

// Only the very first load (no data yet) shows the skeleton; subsequent
// poll refreshes keep the previous numbers on screen.
const isInitialLoading = computed(() => props.loading && props.entries.length === 0)
// Loaded but zero endpoints (or a first-load failure with nothing to show):
// render a neutral "no data" state instead of a misleading "全部正常".
const isEmpty = computed(() => !isInitialLoading.value && props.entries.length === 0)

// Banner math only counts enabled endpoints; disabled ones show up as a
// trailing note and never enter the conclusion (spec 0003 §5.1).
const enabledEntries = computed(() => props.entries.filter(e => e.enabled))
const disabledCount = computed(() => props.entries.length - enabledEntries.value.length)

const counts = computed<Record<EndpointStatus, number>>(() => {
  const c: Record<EndpointStatus, number> = { healthy: 0, degraded: 0, down: 0, failing: 0 }
  for (const entry of enabledEntries.value) c[entry.status] += 1
  return c
})

// Prefer the server aggregate; fall back to the locally computed count.
const enabledTotal = computed(() => props.enabledEndpoints ?? enabledEntries.value.length)

const tone = computed<'healthy' | 'degraded' | 'abnormal'>(() => {
  if (counts.value.down + counts.value.failing > 0) return 'abnormal'
  if (counts.value.degraded > 0) return 'degraded'
  return 'healthy'
})

const hasFailing = computed(() => counts.value.failing > 0)
// Only the abnormal state is clickable (apply status filter + scroll).
const clickable = computed(() => !isEmpty.value && tone.value === 'abnormal')

const conclusion = computed(() => {
  if (isEmpty.value) return '暂无数据'
  if (tone.value === 'abnormal') return `${counts.value.down + counts.value.failing} 个端点异常`
  if (tone.value === 'degraded') return `${counts.value.degraded} 个端点降级`
  return '全部正常'
})

// "HH:mm" sliced out of the shared formatTime helper ("YYYY-MM-DD HH:mm:ss").
const updatedAt = computed(() => {
  if (!props.generatedAt) return null
  const full = formatTime(props.generatedAt)
  return full.length >= 16 ? full.slice(11, 16) : full
})

const subtext = computed(() => {
  if (isEmpty.value) return '没有可展示的端点数据'
  const parts: string[] = []
  if (tone.value === 'abnormal') {
    parts.push(`告警 ${counts.value.failing}`, `宕机 ${counts.value.down}`, `降级 ${counts.value.degraded}`)
  }
  // availability_24h is null when no enabled endpoint has probes; leave the
  // slot empty instead of fabricating a number.
  if (props.availability24h !== null) parts.push(`24h 可用率 ${formatPercent(props.availability24h)}`)
  if (tone.value === 'healthy') parts.push(`共 ${enabledTotal.value} 个端点`)
  if (tone.value === 'degraded') parts.push(`另有 ${counts.value.healthy} 个正常`)
  if (disabledCount.value > 0) parts.push(`${disabledCount.value} 个已停用`)
  parts.push(`每 ${POLL_INTERVAL_MS / 1000} 秒自动刷新`)
  if (updatedAt.value) parts.push(`更新于 ${updatedAt.value}`)
  return parts.join(' · ')
})

function onClick() {
  if (!clickable.value) return
  // Filter to the more urgent abnormal status first.
  emit('inspect', hasFailing.value ? 'failing' : 'down')
}
</script>

<style scoped>
.health-banner {
  border-radius: var(--hs-radius);
  padding: 16px 20px;
  margin-bottom: 16px;
  transition: box-shadow 0.15s ease;
}
.banner-clickable {
  cursor: pointer;
}
.banner-clickable:hover {
  box-shadow: var(--hs-shadow-hover);
}
/* Initial-load skeleton sits on a neutral ground (spec 0003 §5.1), never on
   a status tint — green would falsely signal "all healthy" before data. */
.banner-loading {
  background: var(--hs-bg-page);
  border: 1px solid var(--hs-border);
}
.banner-healthy {
  background: var(--el-color-success-light-9);
}
.banner-healthy .conclusion {
  color: var(--el-color-success);
}
.banner-degraded {
  background: var(--el-color-warning-light-9);
}
.banner-degraded .conclusion {
  color: var(--el-color-warning);
}
.banner-abnormal {
  background: var(--el-color-danger-light-9);
}
.banner-abnormal .conclusion {
  color: var(--el-color-danger);
}
.conclusion-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.conclusion {
  font-size: var(--hs-text-2xl);
  font-weight: 600;
  line-height: 1.5;
}
/* Same pulse as StatusBadge's failing dot (sole animated state, §3). */
.alert-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
  background: var(--hs-status-failing);
  animation: hs-blink 0.8s ease-in-out infinite;
}
.stale-note {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
  padding: 0 6px;
}
.subtext {
  font-size: var(--hs-text-md);
  color: var(--hs-text-regular);
  margin-top: 4px;
}
/* Skeleton mirrors the loaded layout heights (24px title + 14px subtext)
   so first paint does not shift content below. */
.banner-skeleton {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
  min-height: 61px;
}
.skeleton-line {
  border-radius: var(--hs-radius-sm);
  background: var(--el-border-color-extra-light);
  animation: pulse 1.2s ease-in-out infinite;
}
.skeleton-title {
  width: 220px;
  height: 24px;
}
.skeleton-sub {
  width: 340px;
  max-width: 70%;
  height: 14px;
}
@keyframes pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}
</style>
