<template>
  <aside class="app-sidebar">
    <!-- Brand block: BrandMark + Wordmark, click goes home. BrandMark is
         never used bare — it always appears alongside the Wordmark. -->
    <router-link to="/" class="brand">
      <BrandMark class="brand-mark" />
      <Wordmark class="brand-wordmark" />
    </router-link>

    <nav class="side-nav">
      <router-link
        v-for="item in visibleItems"
        :key="item.key"
        :to="item.to"
        class="nav-item"
        :class="{ 'nav-active': isSidebarItemActive(item, route.path) }"
      >
        <component :is="ICONS[item.key]" class="nav-icon" />
        <span class="nav-label">{{ item.label }}</span>
      </router-link>
    </nav>

    <!-- Batch progress entry (ticket 52, migrated from AppHeader in GH #112):
         rendered only for logged-in users while an unfinished batch exists;
         the click deep-links to that very batch (issue #16). -->
    <button v-if="user && activeBatch" type="button" class="batch-entry" @click="goToActiveBatch">
      <Loading class="batch-icon" />
      <span>批次运行中 {{ activeBatch.progress.done + activeBatch.progress.failed }}/{{ activeBatch.progress.total }}</span>
    </button>

    <div class="side-footer">
      <template v-if="user">
        <div class="account-row">
          <span class="account-name" :title="user.username">{{ user.username }}</span>
          <span class="account-role">{{ roleLabel(user.role) }}</span>
          <button type="button" class="logout-btn" :disabled="loggingOut" @click="onLogout">退出</button>
        </div>
      </template>
      <!-- Anonymous readers get the quiet admin entry here (ticket 90 spirit,
           relocated from PublicFooter): no public page renders a prominent
           login button. -->
      <router-link v-else to="/login" class="login-link">管理登录</router-link>
      <!-- Copyright line (GH #122, conservative restore): PublicFooter's
           retirement dropped the © line with it and spec 0018 does not
           record an intentional removal — the sidebar footer inherits it. -->
      <span class="copyright">© 2026 HubScope</span>
      <span v-if="shortVersion" class="version" :title="`HubScope ${version}`">{{ shortVersion }}</span>
    </div>
  </aside>
</template>

<script setup lang="ts">
// AppSidebar (GH #112, spec 0018): the v2 macOS Settings-style 220px shell
// sidebar, replacing the top AppHeader. Self-built signature surface — no
// Element Plus layout components; only EP line icons are consumed. Session
// state is checked locally on mount and re-checked on every route change —
// deliberately no state store (AppHeader precedent).
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { Bell, Box, DataLine, Loading, Monitor, Setting, TrendCharts } from '@element-plus/icons-vue'
import { fetchAuthStatus, logout } from '@/api/auth'
import type { AuthUser } from '@/api/auth'
import { listCampaigns } from '@/api/campaigns'
import type { Campaign } from '@/api/types'
import { fetchVersion } from '@/api/version'
import { roleLabel } from '@/utils/role'
import { isSidebarItemActive, visibleSidebarItems } from '@/utils/sidebarNav'
import { createVisibilityPoll, type VisibilityPollHandle } from '@/utils/visibilityPoll'
import BrandMark from './BrandMark.vue'
import Wordmark from './Wordmark.vue'

const route = useRoute()
const router = useRouter()

// Line-icon per nav key (spec 0018: line icons + text); the key set mirrors
// sidebarNav.ts one-to-one.
const ICONS: Record<string, unknown> = {
  dashboard: Monitor,
  benchmark: TrendCharts,
  eval: DataLine,
  models: Box,
  alerts: Bell,
  settings: Setting,
}

// Local session identity; `user` is null when unauthenticated and every
// authed-only branch reads it. A failed status check is treated as
// unauthenticated, same as the router guard.
const user = ref<AuthUser | null>(null)
const loggingOut = ref(false)

const visibleItems = computed(() => visibleSidebarItems(user.value))

async function refreshAuth() {
  try {
    user.value = (await fetchAuthStatus()).user ?? null
  } catch {
    user.value = null
  }
}

// Newest unfinished batch, if any (campaigns arrive newest first). The entry
// is advisory: fetch failures stay silent and simply hide it.
const activeBatch = ref<Campaign | null>(null)
let batchPoll: VisibilityPollHandle | null = null

function stopBatchPolling() {
  batchPoll?.clear()
  batchPoll = null
}

// Arm the 3s poll only while an unfinished batch exists; the tick that
// observes the settle stops the timer and hides the entry. The poll pauses
// in a hidden tab and refreshes immediately on return (visibilityPoll
// discipline); clear() runs on unmount like the old clearInterval pair.
async function refreshBatch() {
  if (!user.value) {
    activeBatch.value = null
    stopBatchPolling()
    return
  }
  try {
    const list = await listCampaigns()
    activeBatch.value = list.find((c) => c.status === 'running' || c.status === 'pending') ?? null
  } catch {
    activeBatch.value = null
  }
  stopBatchPolling()
  if (activeBatch.value) {
    batchPoll = createVisibilityPoll(() => void refreshBatch(), { intervalMs: 3000 })
  }
}

// Deep-link to the batch the entry is showing (issue #16): /eval resolves
// ?batch=<id> into its initial selection.
function goToActiveBatch() {
  if (!activeBatch.value) return
  void router.push({ path: '/eval', query: { batch: String(activeBatch.value.id) } })
}

async function onLogout() {
  if (loggingOut.value) return
  loggingOut.value = true
  try {
    await logout()
  } catch (err) {
    // Keep the session UI as-is on failure: the server session may still be
    // alive, so only a successful logout clears state and redirects.
    ElMessage.error((err as Error).message)
    loggingOut.value = false
    return
  }
  loggingOut.value = false
  user.value = null
  router.push('/')
}

// Build version (spec 0018 IA: the sidebar footer carries the version — the
// single-binary deployment has no multi-environment concept, so the version
// answers "which build is this"). Fetched once; failure stays silent.
const version = ref('')
const shortVersion = computed(() => {
  if (!version.value) return ''
  const match = version.value.match(/^v\d+\.\d+\.\d+/)
  return match ? match[0] : version.value
})

async function loadVersion() {
  try {
    version.value = (await fetchVersion()).version
  } catch {
    version.value = ''
  }
}

// Auth gates the batch entry, so it refreshes on the same occasions: mount
// and every route change (the refreshAuth watch precedent).
async function refreshShellState() {
  await refreshAuth()
  await refreshBatch()
}

onMounted(() => {
  void refreshShellState()
  void loadVersion()
})
watch(() => route.fullPath, refreshShellState)
onBeforeUnmount(stopBatchPolling)
</script>

<style scoped>
.app-sidebar {
  position: sticky;
  top: 0;
  align-self: flex-start;
  width: 220px;
  flex: none;
  height: 100vh;
  display: flex;
  flex-direction: column;
  padding: var(--hs-space-4) var(--hs-space-3);
  background: var(--hs-bg-card);
  border-right: 1px solid var(--hs-border);
  overflow-y: auto;
}
.brand {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  padding: var(--hs-space-1) var(--hs-space-2);
  text-decoration: none;
}
.brand-mark {
  font-size: 24px;
}
.brand-wordmark {
  font-size: var(--hs-text-lg);
}
.side-nav {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-1);
  margin-top: var(--hs-space-5);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  padding: var(--hs-space-2) var(--hs-space-3);
  border-radius: var(--hs-radius-sm);
  font-size: var(--hs-text-md);
  color: var(--hs-text-regular);
  text-decoration: none;
  transition:
    background-color var(--hs-transition),
    color var(--hs-transition);
}
.nav-icon {
  width: 16px;
  height: 16px;
  flex: none;
  color: var(--hs-text-secondary);
}
/* Lightweight highlight (spec 0018 user story 2): the soft hover ground
   plus brand-colored text/icon — no strong background block. */
.nav-item:hover {
  background: var(--hs-bg-hover);
}
.nav-active,
.nav-active:hover {
  background: var(--hs-bg-hover);
  color: var(--hs-brand);
}
.nav-active .nav-icon {
  color: var(--hs-brand);
}
.nav-item:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: -2px;
}
.batch-entry {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  margin-top: var(--hs-space-4);
  padding: var(--hs-space-2) var(--hs-space-3);
  border: none;
  border-radius: var(--hs-radius-sm);
  background: none;
  font-size: var(--hs-text-sm);
  color: var(--hs-brand);
  cursor: pointer;
  text-align: left;
}
.batch-entry:hover {
  background: var(--hs-bg-hover);
}
.batch-icon {
  width: 14px;
  height: 14px;
  flex: none;
  animation: side-spin 1s linear infinite;
}
@keyframes side-spin {
  to {
    transform: rotate(360deg);
  }
}
/* The global reduced-motion rule zeroes transitions only; the rotation is
   an animation and needs its own gate. */
@media (prefers-reduced-motion: reduce) {
  .batch-icon {
    animation: none;
  }
}
.side-footer {
  margin-top: auto;
  padding-top: var(--hs-space-3);
  border-top: 1px solid var(--hs-border-light);
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.account-row {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
}
.account-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-sm);
  color: var(--hs-text-regular);
}
.account-role {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.logout-btn {
  margin-left: auto;
  flex: none;
  padding: 0;
  border: none;
  background: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  cursor: pointer;
}
.logout-btn:hover {
  color: var(--hs-brand-hover);
}
.login-link {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  text-decoration: none;
}
.login-link:hover {
  color: var(--hs-brand-hover);
}
.copyright {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.version {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  font-family: ui-monospace, 'SF Mono', 'Cascadia Mono', Consolas, monospace;
}
</style>
