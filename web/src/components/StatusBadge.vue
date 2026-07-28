<template>
  <span class="status-badge" :class="`status-${status}`" :title="reason">
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
const props = defineProps<{ status: EndpointStatus; reason?: string; causes?: DegradeCause[] }>()

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
  color: var(--hs-text-regular);
}
.dot {
  width: 10px;
  height: 10px;
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
  animation: hs-blink 0.8s ease-in-out infinite;
}
.causes {
  /* Plain secondary text: no dot, no background, no animation. */
  color: var(--hs-text-secondary);
  white-space: nowrap;
}
</style>
