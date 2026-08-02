<template>
  <div id="app">
    <!-- Shell-out pages (route meta.bare: /login and /report/:token) render
         without the header and the sidebar — the check is the generic route
         meta flag, not a per-page special case (GH #112, spec 0018 IA). -->
    <router-view v-if="isBare" />
    <div v-else class="app-shell">
      <!-- Full-width header restored per the reference design (GH #135):
           the sidebar starts below it and no longer carries the brand
           seat. On narrow viewports the topbar's hamburger toggles the
           sidebar's overlay-drawer form (2026-08-01 shell drawer batch). -->
      <AppTopbar :drawer-open="drawerOpen" @toggle-drawer="drawerOpen = !drawerOpen" />
      <div class="app-body">
        <AppSidebar :drawer-open="drawerOpen" @close-drawer="closeDrawer" />
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
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppTopbar from '@/components/AppTopbar.vue'
import AppSidebar from '@/components/AppSidebar.vue'
import { useBreakpoint } from '@/composables/useBreakpoint'

const route = useRoute()
const isBare = computed(() => Boolean(route.meta.bare))

// --- Nav drawer (2026-08-01 shell drawer batch) ------------------------------
// The shell owns the drawer state: the topbar hamburger toggles it, the
// sidebar reports close intents (ESC / scrim), and two automatic closes
// keep the state honest — a route change (a nav item was selected) and a
// viewport resize back to desktop (the drawer form no longer exists; the
// breakpoint state comes from the shared useBreakpoint mechanism, whose
// closed consumer list now includes the shell).
const { isNarrow } = useBreakpoint()
const drawerOpen = ref(false)

watch(isNarrow, (narrow) => {
  if (!narrow) drawerOpen.value = false
})
watch(
  () => route.path,
  () => {
    drawerOpen.value = false
  },
)

// Unified close path: every close intent lands here. Focus returns to the
// hamburger (data-drawer-toggle) — the topbar never unmounts inside the
// shell, so the anchor always exists.
function closeDrawer() {
  drawerOpen.value = false
  nextTick(() => {
    document.querySelector<HTMLElement>('[data-drawer-toggle]')?.focus()
  })
}
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
  /* GH #139: the skeleton ground returns to the subtle gray — white region
     tiles (hero / widgets / list / events) layer on top of it. AppTopbar
     stays white; AppSidebar carries the same subtle ground. Bare routes
     (/login, /report/:token) render outside this shell, unaffected. */
  background: var(--hs-bg-subtle);
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
