<template>
  <!-- Uniform vendor tile (GH #136; GH #139 render seat moved into the name
       cell; GH #140 three variants): one fixed 26x26 square for EVERY
       vendor — control-grade radius, neutral hover-surface ground +
       secondary initials for unknown vendors. The ground for KNOWN vendors
       is the inline vendorTileBackground: brand hex (brand variant) /
       --hs-bg-subtle (subtle variant, original-color marks) / transparent
       (none variant, hunyuan's self-grounded disc). A 3-char CJK family
       name (~36px) would otherwise spill past the fixed box (GH #131 check
       LOW-1). Extracted from ModelStatusList in the 2026-08-01 narrow-card
       batch — the group header, the desktop row, and the narrow card all
       render this same tile. -->
  <span
    class="vendor-tile"
    :class="{ 'has-icon': icon }"
    :style="icon ? { background: vendorTileBackground(icon) } : undefined"
    :title="family"
  >
    <svg v-if="icon" viewBox="0 0 24 24" role="img" :aria-label="family">
      <defs v-if="icon.gradients">
        <linearGradient
          v-for="g in icon.gradients"
          :key="g.id"
          :id="g.id"
          :x1="g.x1"
          :y1="g.y1"
          :x2="g.x2"
          :y2="g.y2"
        >
          <stop v-for="s in g.stops" :key="s.offset" :offset="s.offset" :stop-color="s.color" />
        </linearGradient>
      </defs>
      <circle
        v-for="(c, i) in icon.circles ?? []"
        :key="`c${i}`"
        :cx="c.cx"
        :cy="c.cy"
        :r="c.r"
        :fill="c.fill"
      />
      <path v-for="(p, i) in icon.paths" :key="i" :d="p.d" :fill="p.fill" />
    </svg>
    <template v-else>{{ familyInitials(family) }}</template>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { vendorIcon, vendorTileBackground } from '@/utils/vendorIcon'
import { familyInitials } from '@/utils/modelList'

const props = defineProps<{ family: string }>()
const icon = computed(() => vendorIcon(props.family))
</script>

<style scoped>
.vendor-tile {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  flex: none;
  overflow: hidden;
  border-radius: var(--hs-radius-sm);
  background: var(--hs-bg-hover);
  color: var(--hs-text-secondary);
  font-size: var(--hs-text-xs);
  font-weight: 600;
  letter-spacing: 0.02em;
}
/* Known vendor: the ground is the inline variant background
   (vendorTileBackground, vendorIcon.ts); the 16px glyph centers inside —
   the GH #134 transparent-ground form retired with the uniform tile. */
.vendor-tile.has-icon {
  color: var(--hs-bg-card);
}
.vendor-tile.has-icon svg {
  display: block;
  width: 16px;
  height: 16px;
}
</style>
