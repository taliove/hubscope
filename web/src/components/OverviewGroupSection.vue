<template>
  <el-card shadow="never" class="group-section">
    <!-- Header row (a11y harden 2026-07-29): the collapse hot zone is a real
         full-width <button> with aria-expanded; the share button moves OUT
         to a sibling — a <button> must never nest another <button>. -->
    <div class="group-header-row" :class="{ 'is-collapsed': collapsed }">
      <button
        type="button"
        class="group-header"
        :aria-expanded="!collapsed"
        @click="collapsed = !collapsed"
      >
        <el-icon class="group-arrow" :class="{ 'group-arrow-open': !collapsed }">
          <ArrowRight />
        </el-icon>
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
            <!-- Count chips dotless (GH #82, closed-list scene ③): the
                 state-colored word alone carries the double encoding; dots
                 here would be a wall of repeated lamps. -->
            <StatusBadge :status="s" dotless />
            <span class="stat-num">{{ group.status_counts[s] }}</span>
          </template>
          <el-tag v-if="group.status_counts['disabled']" type="info" size="small">
            禁用 {{ group.status_counts['disabled'] }}
          </el-tag>
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

    <!-- Group strip row (2026-07-31, GH #85 slim band + GH #87 fixed width,
         GH #88 aligned to the in-card strip spec): the strip (left-aligned,
         fixed 246px — same width as the in-card strip slot, so its ≈8.3×10px
         segments match the cards' dots pixel-for-pixel) and the "本组:"
         metrics share one row, metrics right-aligned via margin-left:auto
         with natural whitespace between — shape → reading in a single scan.
         The "本组:" prefix (GH #55) remains one container-level scope marker
         so the group's availability/latency can never be read as the global
         figures the banner carries. Metrics moved OUT of the header row;
         folding the strip into the header was rejected (per-group left
         content varies, cross-group alignment is the timeline language's
         core value). Always visible, collapsed or not (GH #64). Fixed width
         makes every group's strip identical in width with strictly aligned
         left AND right edges. -->
    <div class="strip-row">
      <UptimeStrip :dots="groupDots" />
      <span class="group-metrics">
        本组:24h 可用率 {{ formatPercent(group.availability_24h) }} · 均延 {{ formatMs(group.avg_latency_ms) }}
      </span>
    </div>

    <!-- Disclosure container (2026-07-29 /impeccable animate, ui-guidelines
         §6 扩展条): grid 0fr↔1fr height transition + inner min-height:0 /
         overflow:hidden + directional visibility delay. Collapsed state exits
         the tab order and a11y tree exactly as v-show's display:none did.
         el-empty stays INSIDE the container so auto-collapsed empty groups
         hide it, matching the old v-show behavior. -->
    <div class="collapse-wrap" :class="{ 'is-collapsed': collapsed, 'no-motion': noMotion }">
      <div class="collapse-inner">
        <div class="card-grid">
          <EndpointCard
            v-for="entry in entries"
            :key="entry.endpoint_id"
            :entry="entry"
            :show-protocol-tag="!collapseCardProtocolTag"
            @open="emit('open', $event)"
          />
          <el-empty v-if="entries.length === 0" description="该组无匹配的 Endpoint" :image-size="60" />
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Share, ArrowRight } from '@element-plus/icons-vue'
import StatusBadge from './StatusBadge.vue'
import EndpointCard from './EndpointCard.vue'
import UptimeStrip from './UptimeStrip.vue'
import { formatPercent, formatMs } from '@/utils/format'
import { protocolTagType } from '@/utils/protocol'
import { SEVERITY_ORDER } from '@/utils/severitySort'
import { aggregateDots24h } from '@/utils/overviewDots'
import type { OverviewGroup, OverviewEntry, Protocol } from '@/api/types'

const props = defineProps<{
  group: OverviewGroup
  entries: OverviewEntry[]
  grouping?: 'family' | 'capability' | 'protocol'
}>()

const emit = defineEmits<{ (e: 'share'): void; (e: 'open', entry: OverviewEntry): void }>()

const collapsed = ref(false)

// No-motion dual track (ui-guidelines §6 扩展条双轨纪律, GH #52 延伸): only
// user clicks animate; data/filter-driven collapse & re-expand switch
// instantly. While true, .no-motion strips all transitions on the wrap.
const noMotion = ref(false)

// Auto-collapse filtered-empty groups (user request 2026-07-29): a group
// with no matching entries renders collapsed by default instead of a large
// empty-state box; it re-expands the moment matches return, and stays
// manually toggleable in both states.
// BOTH paths (collapse and re-expand) go through noMotion: apply the
// collapsed styles transition-free first, then re-enable transitions after
// two frames so the state change is never tweened (miss one path and
// clearing a filter tweens every empty group open at once — noise).
watch(
  () => props.entries.length,
  (n) => {
    noMotion.value = true
    collapsed.value = n === 0
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        noMotion.value = false
      })
    })
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

// Group strip scope (batch-59 rule, ui-guidelines §5): ENABLED entries only —
// a disabled endpoint's stale probes must not color the group's health bar.
const groupDots = computed(() => aggregateDots24h(props.entries.filter(e => e.enabled)))
</script>

<style scoped>
.group-section {
  /* GH #74 breathing rhythm: section gap 12px → 32px (surface brief
     OverviewGroupSection 组头节奏节). */
  margin-bottom: var(--hs-space-6);
}
.group-header-row {
  display: flex;
  align-items: center;
  gap: 4px;
  /* GH #74 + GH #83 collapsed-header revision: 1px hairline on the ROW
     container (not inside the collapse button, not inside collapse-inner).
     Hairline is colored only in the expanded state — collapsed state keeps
     the border as a 1px transparent placeholder so the header row height is
     pixel-identical across both states (brief "折叠组头修订" section: no
     height jump, same discipline as the §6 disclosure container). Vertical
     centering: padding is symmetric top/bottom (the old bottom-only padding
     pushed content up). 12px gap to the card matrix below is unchanged.
     Decorative separator uses border-light, never border. */
  padding-top: var(--hs-space-2);
  padding-bottom: var(--hs-space-2);
  border-bottom: 1px solid var(--hs-border-light);
  margin-bottom: var(--hs-space-3);
}
/* Show/hide has NO transition (the disclosure trio only governs the card
   matrix container, not this row) — the color flips instantly with the
   class, on both the click path and the data-driven no-motion
   auto-collapse path. */
.group-header-row.is-collapsed {
  border-bottom-color: transparent;
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
  transition: transform var(--hs-transition);
}
/* Single-icon disclosure indicator (2026-07-29 /impeccable animate):
   ArrowRight rotates 0→90deg on expand, replacing the two-icon hard swap. */
.group-arrow-open {
  transform: rotate(90deg);
}
/* Disclosure container (ui-guidelines §6 扩展条): grid 0fr↔1fr + inner
   min-height:0/overflow:hidden + directional visibility delay. No
   reduced-motion media block here — the global zero lives in semantics.css. */
.collapse-wrap {
  display: grid;
  grid-template-rows: 1fr;
  /* visibility 0s 0s: expand direction becomes visible instantly. The 0.2s
     literal mirrors the --hs-transition duration (tokens.css) — the two
     reference each other and must change together. */
  transition: grid-template-rows var(--hs-transition), visibility 0s 0s;
}
.collapse-wrap.is-collapsed {
  grid-template-rows: 0fr;
  visibility: hidden;
  /* Collapse direction: hide only after the height has finished collapsing
     (0.2s); visibility keeps the collapsed content out of tab order and the
     a11y tree (equivalent to the old v-show display:none semantics). */
  transition: grid-template-rows var(--hs-transition), visibility 0s 0.2s;
}
.collapse-wrap.no-motion,
.collapse-wrap.no-motion.is-collapsed {
  transition: none;
}
.collapse-inner {
  min-height: 0;
  overflow: hidden;
}
.group-key {
  /* GH #74: group name bumps lg → xl (Title tier — group headers are the
     board's second-level anchor); weight stays 600. */
  font-size: var(--hs-text-xl);
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
  /* GH #85: moved out of the header row into the strip row. GH #87: the
     strip no longer flexes, so right alignment now comes from this element's
     own margin-left:auto (was: absorbed by the strip's flex:1). nowrap keeps
     the metric pair on one line; the fixed-width strip plus the natural
     whitespace between the two is the tool-grade calm layout. */
  flex: none;
  margin-left: auto;
  white-space: nowrap;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.strip-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  /* Breathing room between the strip row and the card matrix (the header's
     own margin-bottom handles the gap above the row). */
  margin-bottom: 10px;
}
.strip-row .uptime-strip {
  /* GH #88 fixed width (user sign-off 2026-07-31, replaces GH #87's 360px):
     segment width = strip width ÷ 24, and the "lamp" reading came from the
     segment aspect ratio — GH #85 (6px height) and GH #87 (360px) each
     pressed only one dimension. The fix aligns the strip with the in-card
     strip slot: 272px card − 26px label = 246px → slots ≈ (246 − 23×2) / 24
     ≈ 8.3px, the same near-square 8.3×10px dots as the cards. flex 0 1 +
     min-width 0 lets the strip shrink on narrow viewports (the 24 inner
     slots are flex 1 1 0 and shrink with it) — §4 no horizontal scroll. */
  flex: 0 1 246px;
  min-width: 0;
}
.group-share {
  /* Sibling of the collapse button; the header row's 4px gap carries the
     spacing (was margin-left: 4px while nested in the old header div). */
  flex: none;
}
.card-grid {
  display: grid;
  /* 272px floor keeps a stable 4-column matrix at the 1200px content width
     (GH #72, surface brief EndpointCard 卡片网格节; DashboardView flat
     grid mirrors this). */
  grid-template-columns: repeat(auto-fill, minmax(272px, 1fr));
  gap: 12px;
}
</style>
