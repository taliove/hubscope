<template>
  <span class="status-badge" :class="[`tone-${display.tone}`, `badge-${size}`]" :title="reason">
    <span v-if="!dotless" class="dot" />
    <span class="label">{{ display.label }}</span>
    <span v-if="display.causeSuffix" class="causes">{{ display.causeSuffix }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { DegradeCause, EndpointStatus } from '@/api/types'
import { statusDisplay, type DisplayStatus } from '@/utils/statusDisplay'

// StatusBadge (rebuilt for UI v2, spec 0018 / GH #113): a small dot plus the
// Chinese status word — 稳定 / 性能下降 / 服务异常. The word and the color
// slot come from the single display-layer mapping (utils/statusDisplay.ts);
// this component never writes a status word literal.
//
// Three-state display: the domain status machine keeps four states (W5),
// but failing has no separate display identity — it renders as 服务异常 with
// the danger slot. Zero blink: the failing blink is retired wholesale (the
// --hs-blink token stays none until the display-layer rebuild batch removes
// the @keyframes), so nothing in this component animates on a timer.
//
// Color channels (graphic/text division): the WORD consumes the *-text
// grade of its slot (--hs-success-text etc. — the body grades fail AA in
// text scenarios), the DOT consumes the body grade. Status changes glide
// through a color transition (--hs-transition); the semantics.css global
// reduced-motion rule zeroes it. No resident breathing — a state-change
// pulse belongs to the later motion ticket.
//
// The optional causes prop adds the degrade-cause sub-label after the word
// (spec 0013) — plain secondary text, never a second status light; the
// mapping renders it only for the degraded state. Aggregates never pass
// causes.
//
// size is a two-stop scale (GH #54): sm (default) everywhere, md only where
// the status row is the card's primary line (EndpointCard). The md stop is
// a size variant of the same badge — never a second status light; callers
// must not :deep-override it.
//
// dotless variant (GH #81): the state-colored word is itself the text form
// of the color+word double encoding, so aggregate/repeated scenes may drop
// the dot. Closed applicability list (no spreading): ① EndpointCard status
// row, ② Hero band counts row, ③ group-header count chips. Detail page /
// quick-view dialog / admin tables keep the dot (entity signal positions).
// No a11y regression: the word is the accessible name either way.
//
// The status prop also accepts a DisplayStatus directly: merged-count rows
// (Hero band, group header) collapse down+failing into one incident count
// and render a badge for the display state itself.
const props = withDefaults(
  defineProps<{
    status: EndpointStatus | DisplayStatus
    reason?: string
    causes?: DegradeCause[]
    size?: 'sm' | 'md'
    dotless?: boolean
  }>(),
  { size: 'sm', dotless: false },
)

const display = computed(() => statusDisplay(props.status, props.causes))
const reason = computed(() => props.reason ?? '')
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--hs-text-sm);
  /* State changes glide through color (spec 0018 动效体系: color transition,
     no resident breathing); the semantics.css global rule zeroes this under
     prefers-reduced-motion. */
  transition: color var(--hs-transition);
}
/* Word follows the dot's slot but consumes the *-text grade (graphic/text
   division): color alone is never the only channel (dot + word is the
   double encoding), and the body grades fail AA in text scenarios. */
.tone-success {
  color: var(--hs-success-text);
}
.tone-warning {
  color: var(--hs-warning-text);
}
.tone-danger {
  color: var(--hs-danger-text);
}
.dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: none;
  transition: background-color var(--hs-transition);
}
/* The dot is a graphic: body grade of the slot. */
.tone-success .dot {
  background: var(--hs-success);
}
.tone-warning .dot {
  background: var(--hs-warning);
}
.tone-danger .dot {
  background: var(--hs-danger);
}
/* md stop (GH #54): the status word steps up to the card-primary scale; the
   dot grows to match. The cause sub-label stays sm/secondary — it is
   auxiliary text in every size. */
.badge-md {
  font-size: var(--hs-text-md);
  font-weight: 600;
}
.badge-md .dot {
  width: 11px;
  height: 11px;
}
/* The cause sub-label explicitly stays secondary text: without this color
   it would inherit the state-colored word above — it is auxiliary text in
   every size, never part of the state signal. */
.causes {
  /* Plain secondary text: no dot, no background, no animation. */
  font-size: var(--hs-text-sm);
  font-weight: 400;
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
</style>
