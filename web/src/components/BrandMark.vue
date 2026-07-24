<template>
  <!-- BrandMark: the only graphic mark. hub glyph (center ring + three
       spokes + P arc) as inline SVG; gradient stops consume the teal raw
       scale — the single sanctioned exception to "semantics layer only"
       (a graphic mark, not semantic expression; ui-guidelines §2b).
       Same glyph and gradient as ProxyHub, fully isomorphic by design. -->
  <svg
    class="hs-brand-mark"
    viewBox="0 0 64 64"
    role="img"
    aria-label="HubScope"
    xmlns="http://www.w3.org/2000/svg"
  >
    <defs>
      <linearGradient :id="gradientId" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" class="hs-brand-mark__stop-start" />
        <stop offset="1" class="hs-brand-mark__stop-end" />
      </linearGradient>
    </defs>
    <rect x="2" y="2" width="60" height="60" rx="14" :fill="`url(#${gradientId})`" />
    <g stroke="#fff" stroke-width="4" stroke-linecap="round" fill="none">
      <!-- Center ring -->
      <circle cx="32" cy="32" r="6" />
      <!-- Three spokes to the left nodes -->
      <line x1="16" y1="16" x2="26.5" y2="26.5" />
      <line x1="14" y1="32" x2="25" y2="32" />
      <line x1="16" y1="48" x2="26.5" y2="37.5" />
      <!-- P arc: sweep from the ring top to the right node -->
      <path d="M 32 26 A 11 11 0 0 1 51 32" />
      <!-- Spoke from center to the right node -->
      <line x1="39" y1="32" x2="46" y2="32" />
    </g>
    <g fill="#fff">
      <circle cx="13" cy="13" r="4" />
      <circle cx="10" cy="32" r="4" />
      <circle cx="13" cy="51" r="4" />
      <circle cx="51" cy="32" r="4" />
    </g>
  </svg>
</template>

<script setup lang="ts">
// Unique gradient id per instance so multiple BrandMarks on one page (e.g.
// AppHeader + StatusCard preview) never collide on the shared <defs> id.
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
  stop-color: var(--hs-teal-400);
}

.hs-brand-mark__stop-end {
  stop-color: var(--hs-teal-700);
}
</style>
