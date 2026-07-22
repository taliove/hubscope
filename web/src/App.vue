<template>
  <div id="app">
    <AppHeader v-if="route.name !== 'login'" />
    <router-view />
  </div>
</template>

<script setup lang="ts">
// Root component: shared AppHeader on every page except login, then the
// matched route component.
import { useRoute } from 'vue-router'
import AppHeader from '@/components/AppHeader.vue'

const route = useRoute()
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
}
</style>
