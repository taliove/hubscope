<template>
  <div id="app">
    <AppHeader v-if="route.name !== 'login'" />
    <router-view />
  </div>
</template>

<script setup lang="ts">
// Root component: shared AppHeader on every page except login, then the
// matched route component. Theme (light/dark) initializes here once —
// index.html's inline script already applied the persisted class, this
// just syncs the reactive ref.
import { useRoute } from 'vue-router'
import AppHeader from '@/components/AppHeader.vue'
import { useTheme } from '@/utils/theme'

const route = useRoute()
const { initTheme } = useTheme()
initTheme()
</script>

<style>
#app {
  min-height: 100vh;
  background: var(--hs-bg-page);
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
