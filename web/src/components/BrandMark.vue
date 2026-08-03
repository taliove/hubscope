<template>
  <!-- BrandMark: the only graphic mark. Scope glyph (ring + crosshair
       ticks + center pulse dot) as inline SVG — the monitoring metaphor:
       a scope trained on the Hub's endpoints. Gradient stops consume the
       blue raw scale — the single sanctioned exception to "semantics layer
       only" (a graphic mark, not semantic expression). v2.0 (GH #110,
       spec 0018): the teal scale is retired, the gradient moves to the
       brand-blue steps (ADR 0015); the scope glyph itself is unchanged. -->
  <svg
    class="hs-brand-mark"
    viewBox="0 0 64 64"
    role="img"
    aria-label="HubScope"
    xmlns="http://www.w3.org/2000/svg"
  >
    <defs>
      <linearGradient :id="gradientId" x1="0" y1="0" x2="1" y2="1">
        <!-- stop-color attributes mirror tokens.css (blue-400/blue-700) as a
             capture fallback: snapdom never inlines computed styles for SVG
             inner elements, so class-based var() colors are lost in PNG
             export and the gradient falls back to black. In the live app the
             scoped CSS var() rules below stay authoritative (CSS beats the
             presentation attribute). v2.0 blue rebuild (GH #110): #549cff /
             #0062cc stay in sync with tokens.css blue-400/blue-700 — always
             change both places together; favicon.svg carries the same stops. -->
        <stop offset="0" class="hs-brand-mark__stop-start" stop-color="#549cff" />
        <stop offset="1" class="hs-brand-mark__stop-end" stop-color="#0062cc" />
      </linearGradient>
    </defs>
    <rect x="2" y="2" width="60" height="60" rx="14" :fill="`url(#${gradientId})`" />
    <g stroke="#fff" stroke-width="4" stroke-linecap="round" fill="none">
      <!-- Scope ring -->
      <circle cx="32" cy="32" r="14" />
      <!-- Crosshair ticks -->
      <line x1="32" y1="10" x2="32" y2="16" />
      <line x1="32" y1="48" x2="32" y2="54" />
      <line x1="10" y1="32" x2="16" y2="32" />
      <line x1="48" y1="32" x2="54" y2="32" />
    </g>
    <!-- Center pulse dot -->
    <circle cx="32" cy="32" r="5" fill="#fff" />
  </svg>
</template>

<script setup lang="ts">
// Unique gradient id per instance so multiple BrandMarks on one page (e.g.
// AppSidebar + StatusCard preview) never collide on the shared <defs> id.
import { useId } from 'vue'

const gradientId = `hs-brand-grad-${useId()}`
</script>

<style scoped>
.hs-brand-mark {
  width: 1em;
  height: 1em;
  display: block;
}

.hs-brand-mark__stop-start {
  stop-color: var(--hs-blue-400);
}

.hs-brand-mark__stop-end {
  stop-color: var(--hs-blue-700);
}
</style>
