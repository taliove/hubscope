<template>
  <el-card shadow="never" class="group-section">
    <!-- Header row (a11y harden 2026-07-29): the collapse hot zone is a real
         full-width <button> with aria-expanded; the share button moves OUT
         to a sibling — a <button> must never nest another <button>. -->
    <div class="group-header-row">
      <button
        type="button"
        class="group-header"
        :aria-expanded="!collapsed"
        @click="collapsed = !collapsed"
      >
        <el-icon class="group-arrow"><ArrowRight v-if="collapsed" /><ArrowDown v-else /></el-icon>
        <span class="group-key">{{ group.key }}</span>
        <span class="group-count">{{ group.endpoint_count }} 端点</span>
        <!-- Uniform-protocol collapse (GH #54): when every filtered entry in
             the group shares one protocol, one tag in the header replaces the
             per-card tags. Skipped when grouping by protocol — the group key
             already names it. -->
        <el-tag
          v-if="uniformProtocol && grouping !== 'protocol'"
          :type="protocolTagType(uniformProtocol)"
          size="small"
        >
          {{ uniformProtocol }}
        </el-tag>

        <span class="group-stats">
          <template v-for="s in presentStatuses" :key="s">
            <StatusBadge :status="s" />
            <span class="stat-num">{{ group.status_counts[s] }}</span>
          </template>
          <el-tag v-if="group.status_counts['disabled']" type="info" size="small">
            禁用 {{ group.status_counts['disabled'] }}
          </el-tag>
        </span>

        <!-- "本组:" prefix (GH #55): one container-level scope marker for both
             metrics, so the group's availability/latency can never be read as
             the global figures the banner carries. -->
        <span class="group-metrics">
          本组:24h 可用率 {{ formatPercent(group.availability_24h) }} · 均延 {{ formatMs(group.avg_latency_ms) }}
        </span>
      </button>

      <!-- Per-group share (ticket 59): text button so it never competes with
           the StatusBadge colors; sibling of the collapse button, never
           nested inside it. -->
      <el-button text class="group-share" @click.stop="emit('share')">
        <el-icon><Share /></el-icon>
        分享
      </el-button>
    </div>

    <div v-show="!collapsed" class="card-grid">
      <EndpointCard
        v-for="entry in entries"
        :key="entry.endpoint_id"
        :entry="entry"
        :show-protocol-tag="!collapseCardProtocolTag"
      />
      <el-empty v-if="entries.length === 0" description="该组无匹配的 Endpoint" :image-size="60" />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Share, ArrowDown, ArrowRight } from '@element-plus/icons-vue'
import StatusBadge from './StatusBadge.vue'
import EndpointCard from './EndpointCard.vue'
import { formatPercent, formatMs } from '@/utils/format'
import { protocolTagType } from '@/utils/protocol'
import { SEVERITY_ORDER } from '@/utils/severitySort'
import type { OverviewGroup, OverviewEntry, Protocol } from '@/api/types'

const props = defineProps<{
  group: OverviewGroup
  entries: OverviewEntry[]
  grouping?: 'family' | 'capability' | 'protocol'
}>()

const emit = defineEmits<{ (e: 'share'): void }>()

const collapsed = ref(false)

// Auto-collapse filtered-empty groups (user request 2026-07-29): a group
// with no matching entries renders collapsed by default instead of a large
// empty-state box; it re-expands the moment matches return, and stays
// manually toggleable in both states.
watch(
  () => props.entries.length,
  (n) => {
    collapsed.value = n === 0
  },
  { immediate: true },
)

// Statuses present in this group, in the board's single severity caliber
// (GH #55 — SEVERITY_ORDER, heavy → light, shared with the stats strip;
// the local STATUS_PRIORITY list is deleted).
const presentStatuses = computed(() =>
  SEVERITY_ORDER.filter(s => (props.group.status_counts[s] ?? 0) > 0)
)

// Uniform-protocol collapse (GH #54): the protocol shared by every filtered
// entry, or null for mixed/empty groups. Based on the filtered entries prop
// (the same set the cards render), never on the unfiltered group aggregate.
const uniformProtocol = computed<Protocol | null>(() => {
  if (props.entries.length === 0) return null
  const first = props.entries[0].protocol
  // Defensive: the Protocol type forbids '', but API payloads are untyped at
  // runtime — an empty protocol must not collapse into a header tag.
  if ((first as string) === '') return null
  return props.entries.every((e) => e.protocol === first) ? first : null
})

// Card tags collapse when the header carries the tag (uniform group) or
// when the group key already names the protocol (grouping = 'protocol').
// Flat mode never reaches this component, and mixed groups keep card tags.
const collapseCardProtocolTag = computed(
  () => props.grouping === 'protocol' || uniformProtocol.value !== null,
)
</script>

<style scoped>
.group-section {
  margin-bottom: var(--hs-space-3);
}
.group-header-row {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 10px;
}
.group-header {
  /* Full-width button reset (a11y harden 2026-07-29): the collapse hot zone
     is a real <button> — strip UA chrome, inherit card typography. */
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
  font: inherit;
  color: inherit;
  text-align: left;
  background: none;
  border: none;
  padding: 0;
  border-radius: var(--hs-radius-sm);
  cursor: pointer;
  user-select: none;
  flex-wrap: wrap;
}
/* Keyboard focus = the board's single focus language: 1px brand inset ring. */
.group-header:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--hs-brand);
}
.group-arrow {
  color: var(--hs-text-placeholder);
  font-size: 14px;
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
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.group-share {
  /* Sibling of the collapse button; the header row's 4px gap carries the
     spacing (was margin-left: 4px while nested in the old header div). */
  flex: none;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 12px;
}
</style>
