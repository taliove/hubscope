<template>
  <span class="status-badge" :class="[`status-${status}`, `badge-${size}`]" :title="reason">
    <span class="dot" />
    <span class="label">{{ label }}</span>
    <span v-if="causeSuffix" class="causes">{{ causeSuffix }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { DegradeCause, EndpointStatus } from '@/api/types'
import { STATUS_LABELS } from '@/utils/healthConclusion'
import { degradeCauseSuffix } from '@/utils/degradeCauses'

// Colored status light with a Chinese label; the reason shows on hover.
// The optional causes prop adds a plain-text degrade-cause sub-label after
// the status word (spec 0013) — secondary text only, never a second status
// light. Aggregates never pass causes.
// size is a two-stop scale (GH #54): sm (default) everywhere, md only where
// the status row is the card's primary line (EndpointCard). The md stop is
// a size variant of the same badge — same dot, same label — never a second
// status light; callers must not :deep-override it.
const props = withDefaults(
  defineProps<{ status: EndpointStatus; reason?: string; causes?: DegradeCause[]; size?: 'sm' | 'md' }>(),
  { size: 'sm' },
)

const label = computed(() => STATUS_LABELS[props.status])
const reason = computed(() => props.reason ?? '')
// Defense in depth: the sub-label only exists for degraded endpoints, even
// if a caller passes causes for another status.
const causeSuffix = computed(() =>
  props.status === 'degraded' && props.causes ? degradeCauseSuffix(props.causes) : '',
)
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--hs-text-sm);
}
/* Word follows the light (GH #71): the status word carries the same semantic
   color as the dot, so color alone is never the only channel (dot + word is
   the double encoding). healthy uses the deepened text grade --hs-success-text
   (the --hs-success body fails AA on white in text scenarios; dots keep the
   body — graphic/text division, ui-guidelines §3). */
.status-healthy {
  color: var(--hs-success-text);
}
.status-degraded {
  color: var(--hs-warning);
}
.status-down {
  color: var(--hs-danger);
}
.status-failing {
  color: var(--hs-status-failing);
}
.dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: none;
}
.status-healthy .dot {
  background: var(--hs-success);
}
.status-degraded .dot {
  background: var(--hs-warning);
}
.status-down .dot {
  background: var(--hs-danger);
}
.status-failing .dot {
  background: var(--hs-status-failing);
  /* --hs-blink (semantics.css) goes still under prefers-reduced-motion. */
  animation: var(--hs-blink);
}
/* md stop (GH #54): the status word steps up to the card-primary scale; the
   dot grows to match. The cause sub-label stays sm/secondary — it is
   auxiliary text in every size (ui-guidelines §5 StatusBadge). */
.badge-md {
  font-size: var(--hs-text-md);
  font-weight: 600;
}
.badge-md .dot {
  width: 11px;
  height: 11px;
}
/* The cause sub-label explicitly stays secondary text (GH #71): the status
   word above is now state-colored, and without this explicit color the
   sub-label would inherit it — it is auxiliary text in every size, never
   part of the state signal. */
.causes {
  /* Plain secondary text: no dot, no background, no animation. */
  font-size: var(--hs-text-sm);
  font-weight: 400;
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
</style>
