<template>
  <div id="app">
    <router-view />
  </div>
</template>

<script setup lang="ts">
// Root component; router-view renders the matched route component.
</script>

<style>
#app {
  min-height: 100vh;
  background: #f5f7fa;
}

body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue',
    Arial, sans-serif;
}
</style>
