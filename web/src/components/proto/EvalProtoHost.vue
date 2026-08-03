<!-- PROTOTYPE — throwaway. Spec-0020 eval-center variants, mounted on /eval
     behind ?proto=A|B|C (dev-only, see EvalLeaderboardView). Flip variants
     with the bottom bar or ←/→. Mock data only, no backend wiring. -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import VariantA from './EvalProtoVariantA.vue'
import VariantB from './EvalProtoVariantB.vue'
import VariantC from './EvalProtoVariantC.vue'

const route = useRoute()
const router = useRouter()

const order = ['A', 'B', 'C'] as const
const names: Record<string, string> = {
  A: '管线指挥舱',
  B: '裁判席',
  C: '账本',
}

const current = computed<(typeof order)[number]>(() => {
  const v = route.query.proto
  return v === 'B' || v === 'C' ? v : 'A'
})

const component = computed(() => {
  if (current.value === 'B') return VariantB
  if (current.value === 'C') return VariantC
  return VariantA
})

function cycle(delta: number) {
  const i = order.indexOf(current.value)
  const next = order[(i + delta + order.length) % order.length]
  void router.replace({ query: { ...route.query, proto: next } })
}

function onKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement | null
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
  if (e.key === 'ArrowLeft') cycle(-1)
  if (e.key === 'ArrowRight') cycle(1)
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="proto-host">
    <div class="proto-banner">原型 · 评估中心(spec 0020)— 数据为静态样例,非真实接口</div>
    <component :is="component" />
    <div class="proto-switcher">
      <button class="sw-btn" @click="cycle(-1)">←</button>
      <span class="sw-label">{{ current }} — {{ names[current] }}</span>
      <button class="sw-btn" @click="cycle(1)">→</button>
    </div>
  </div>
</template>

<style scoped>
.proto-host {
  padding-bottom: 72px; /* room for the switcher */
}

.proto-banner {
  margin-bottom: var(--hs-space-4);
  padding: var(--hs-space-2) var(--hs-space-3);
  border: 1px dashed var(--hs-warning-base);
  border-radius: 8px;
  color: var(--hs-warning-text-base);
  font-size: 13px;
}

.proto-switcher {
  position: fixed;
  bottom: 20px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 2000;
  display: flex;
  align-items: center;
  gap: var(--hs-space-3);
  padding: 6px 10px;
  background: var(--hs-gray-900);
  color: #fff;
  border-radius: 999px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 28%);
  font-size: 13px;
}

.sw-btn {
  border: none;
  background: var(--hs-gray-700);
  color: #fff;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
}

.sw-btn:hover {
  background: var(--hs-blue-600);
}

.sw-label {
  min-width: 140px;
  text-align: center;
  font-weight: 600;
}
</style>
