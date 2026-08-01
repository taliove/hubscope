<template>
  <div id="app">
    <!-- Shell-out pages (route meta.bare: /login and /report/:token) render
         without the header and the sidebar — the check is the generic route
         meta flag, not a per-page special case (GH #112, spec 0018 IA). -->
    <router-view v-if="isBare" />
    <div v-else class="app-shell">
      <!-- Full-width header restored per the reference design (GH #135):
           the sidebar starts below it and no longer carries the brand
           seat. -->
      <AppTopbar />
      <div class="app-body">
        <AppSidebar />
        <main class="app-main">
          <!-- Page transition (spec 0018 §15): fade + ≤10px shift, 300ms slow
               token; reduced-motion is zeroed globally by semantics.css. The
               key remounts only on path changes, never on query changes. -->
          <router-view v-slot="{ Component }">
            <transition name="page" mode="out-in">
              <component :is="Component" :key="route.path" />
            </transition>
          </router-view>
        </main>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// Root component (GH #112; GH #135 header restore): the v2 shell — a
// full-width top bar on top, then a row of the 220px sidebar and the routed
// page; bare routes (login, shared report) render outside the shell with no
// header and no sidebar.
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppTopbar from '@/components/AppTopbar.vue'
import AppSidebar from '@/components/AppSidebar.vue'

const route = useRoute()
const isBare = computed(() => Boolean(route.meta.bare))
</script>

<style>
#app {
  min-height: 100vh;
  background: var(--hs-bg-page);
}

.app-shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-body {
  display: flex;
  flex: 1;
  min-height: 0;
}

.app-main {
  flex: 1;
  min-width: 0;
}

/* Page transition (spec 0018 §15): fade + 8px enter shift on the shared
   slow token (300ms, inside the 200–300ms window). Leave is fade-only so
   the outgoing page never slides. semantics.css zeroes every transition
   under prefers-reduced-motion, so no component-level gate is needed. */
.page-enter-active,
.page-leave-active {
  transition:
    opacity var(--hs-transition-slow),
    transform var(--hs-transition-slow);
}
.page-enter-from {
  opacity: 0;
  transform: translateY(8px);
}
.page-leave-to {
  opacity: 0;
}

body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue',
    Arial, sans-serif;
  background: var(--hs-bg-page);
  color: var(--hs-text-primary);
  transition:
    background-color var(--hs-transition),
    color var(--hs-transition);
}
</style>
