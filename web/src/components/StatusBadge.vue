<template>
  <span class="status-badge" :class="`status-${status}`" :title="reason">
    <span class="dot" />
    <span class="label">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { EndpointStatus } from '@/api/types'

// Colored status light with a Chinese label; the reason shows on hover.
const props = defineProps<{ status: EndpointStatus; reason?: string }>()

const LABELS: Record<EndpointStatus, string> = {
  healthy: '正常',
  degraded: '降级',
  down: '宕机',
  failing: '告警',
}

const label = computed(() => LABELS[props.status])
const reason = computed(() => props.reason ?? '')
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #606266;
}
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}
.status-healthy .dot {
  background: #67c23a;
}
.status-degraded .dot {
  background: #e6a23c;
}
.status-down .dot {
  background: #f56c6c;
}
.status-failing .dot {
  background: #ff4500;
  animation: blink 0.8s ease-in-out infinite;
}
/* Failing pulses to draw attention without changing layout. */
@keyframes blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.2;
  }
}
</style>
