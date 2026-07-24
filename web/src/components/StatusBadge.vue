<template>
  <span class="status-badge" :class="`status-${status}`" :title="reason">
    <span class="dot" />
    <span class="label">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { EndpointStatus } from '@/api/types'
import { STATUS_LABELS } from '@/utils/healthConclusion'

// Colored status light with a Chinese label; the reason shows on hover.
const props = defineProps<{ status: EndpointStatus; reason?: string }>()

const label = computed(() => STATUS_LABELS[props.status])
const reason = computed(() => props.reason ?? '')
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
</style>
