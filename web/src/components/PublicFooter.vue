<template>
  <!-- PublicFooter is the single quiet admin entry of every public page
       (ticket 90, ui-guidelines §5): anonymous visitors get no login
       button in the header — the entry retreats to this hairline footer
       line, identical on the status board, endpoint detail and /board.
       Rendered regardless of session state (same as the /board precedent);
       /login itself carries no footer. Footer composition: copyright +
       version left, admin entry right (2026-07-28 design decision) so the
       quiet entry does not float alone. The version number moved here from
       the AppHeader brand block (GH #90): since this footer only mounts on
       the three public pages, the version is visible there only — the
       admin console shows no version (accepted scope; a separate ticket
       would be needed to surface it there). -->
  <footer class="public-footer">
    <span class="copyright">
      © 2026 HubScope<span v-if="version" class="version" :title="`HubScope ${version}`"> · {{ shortVersion }}</span>
    </span>
    <!-- An authenticated session gets the console entry; anonymous keeps
         the quiet login entry (GH #157 — “管理登录” made no sense for a
         user already logged in). -->
    <router-link v-if="authed" to="/admin" class="admin-link">进入管理台</router-link>
    <router-link v-else to="/login" class="admin-link">管理登录</router-link>
  </footer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { fetchAuthStatus } from '@/api/auth'
import { fetchVersion } from '@/api/version'

// Build version, fetched once on mount; it never changes during a session.
// Failure stays silent (empty string hides the span) — the version display
// is non-critical, same policy as the AppHeader original.
const version = ref<string>('')
const authed = ref(false)

// Short version: extract only the tag part (e.g., "v0.2.3" from
// "v0.2.3-4-g1adea03-dirty"); dev builds without a vX.Y.Z prefix show the
// full string.
const shortVersion = computed(() => {
  if (!version.value) return ''
  const match = version.value.match(/^v\d+\.\d+\.\d+/)
  return match ? match[0] : version.value
})

async function loadVersion() {
  try {
    const res = await fetchVersion()
    version.value = res.version
  } catch {
    version.value = ''
  }
}

onMounted(() => {
  void loadVersion()
  fetchAuthStatus()
    .then((s) => {
      authed.value = s.authenticated
    })
    .catch(() => {})
})
</script>

<style scoped>
.public-footer {
  margin-top: var(--hs-space-5);
  padding-top: var(--hs-space-4);
  border-top: 1px solid var(--hs-border-light);
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.copyright {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.version {
  font-family: ui-monospace, 'SF Mono', 'Cascadia Mono', Consolas, monospace;
}
.admin-link {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  text-decoration: none;
}
.admin-link:hover {
  color: var(--hs-brand-hover);
}
</style>
