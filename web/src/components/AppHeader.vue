<template>
  <header class="app-header">
    <div class="header-inner">
      <!-- Brand block: app icon (same master as favicon) + wordmark, click goes home. -->
      <router-link to="/" class="brand">
        <img src="/logo.png" alt="HubScope" class="brand-logo" />
        <span class="brand-name">HubScope</span>
      </router-link>

      <nav class="main-nav">
        <router-link
          v-for="item in visibleNavItems"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ 'nav-active': isActive(item) }"
        >
          {{ item.label }}
        </router-link>
      </nav>

      <div class="header-right">
        <!-- Batch progress entry (ticket 52): rendered only for logged-in
             users while an unfinished batch exists; the rotating Loading
             icon is the only motion (no orange-red, no flashing). -->
        <el-button
          v-if="authed && activeBatch"
          link
          type="primary"
          class="batch-entry"
          @click="router.push('/eval')"
        >
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>
            批次运行中 {{ activeBatch.progress.done + activeBatch.progress.failed }}/{{ activeBatch.progress.total }}
          </span>
        </el-button>
        <el-button v-if="authed" @click="router.push('/admin')">管理视图</el-button>
        <el-button v-if="!authed" type="primary" @click="router.push('/login')">登录</el-button>
        <el-button v-else link type="primary" :loading="loggingOut" @click="onLogout">
          退出
        </el-button>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import { fetchAuthStatus, logout } from '@/api/auth'
import { listCampaigns } from '@/api/campaigns'
import type { Campaign } from '@/api/types'

// Global shell header (spec: docs/specs/0003-ui-redesign.md §4.1). Rendered by
// App.vue on every page except /login. Session state is checked locally on
// mount and re-checked on every route change — deliberately no state store.
const route = useRoute()
const router = useRouter()

interface NavItem {
  label: string
  to: string
  // Public items render for anonymous visitors; the rest would only be
  // bounced to /login by the route guard, so they are hidden instead.
  public?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { label: '状态总览', to: '/', public: true },
  { label: '评估榜单', to: '/eval' },
  { label: '任务中心', to: '/tasks' },
]

const authed = ref(false)
const loggingOut = ref(false)

// Anonymous visitors only get the public nav entries; logout flips authed
// to false and this computed collapses the nav back immediately.
const visibleNavItems = computed(() =>
  authed.value ? NAV_ITEMS : NAV_ITEMS.filter((item) => item.public),
)

// The dashboard owns both '/' and the endpoint detail pages.
function isActive(item: NavItem): boolean {
  if (item.to === '/') return route.path === '/' || route.path.startsWith('/endpoints')
  return route.path.startsWith(item.to)
}

// A failed status check is treated as unauthenticated, same as the router guard.
async function refreshAuth() {
  try {
    authed.value = (await fetchAuthStatus()).authenticated
  } catch {
    authed.value = false
  }
}

// Newest unfinished batch, if any (campaigns arrive newest first). The
// header entry is advisory: fetch failures stay silent and simply hide it.
const activeBatch = ref<Campaign | null>(null)
let batchTimer: ReturnType<typeof setInterval> | undefined

function stopBatchPolling() {
  clearInterval(batchTimer)
  batchTimer = undefined
}

// Refresh the batch entry and arm the 3s poll only while an unfinished
// batch exists; the tick that observes the settle stops the timer and hides
// the entry (ui-guidelines §5 AppHeader registration). Every setInterval
// pairs with cleanup on unmount.
async function refreshBatch() {
  if (!authed.value) {
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
    batchTimer = setInterval(() => void refreshBatch(), 3000)
  }
}

async function onLogout() {
  if (loggingOut.value) return
  loggingOut.value = true
  try {
    await logout()
  } catch (err) {
    // Keep the session UI as-is on failure: the server session may still
    // be alive, so only a successful logout clears state and redirects.
    ElMessage.error((err as Error).message)
    loggingOut.value = false
    return
  }
  loggingOut.value = false
  authed.value = false
  router.push('/')
}

// Auth gates the batch entry, so it refreshes on the same occasions: mount
// and every route change (the refreshAuth watch precedent).
async function refreshHeaderState() {
  await refreshAuth()
  await refreshBatch()
}

onMounted(refreshHeaderState)
watch(() => route.fullPath, refreshHeaderState)
onBeforeUnmount(stopBatchPolling)
</script>

<style scoped>
.app-header {
  position: sticky;
  top: 0;
  z-index: 100;
  height: 56px;
  background: var(--hs-bg-card);
  border-bottom: 1px solid var(--hs-border);
}
.header-inner {
  max-width: 1200px;
  height: 100%;
  margin: 0 auto;
  padding: 0 16px;
  display: flex;
  align-items: center;
  gap: 24px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  text-decoration: none;
}
.brand-logo {
  width: 24px;
  height: 24px;
  display: block;
  /* Apple squircle crop: master is a full-bleed square, corners rounded here */
  border-radius: 22.4%;
}
.brand-name {
  font-size: var(--hs-text-lg);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.main-nav {
  display: flex;
  align-items: center;
  gap: 24px;
  height: 100%;
}
.nav-item {
  position: relative;
  height: 100%;
  display: flex;
  align-items: center;
  font-size: var(--hs-text-md);
  color: var(--hs-text-regular);
  text-decoration: none;
}
.nav-item:hover {
  color: var(--hs-brand-hover);
}
.nav-active,
.nav-active:hover {
  color: var(--hs-brand);
}
.nav-active::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 2px;
  background: var(--hs-brand);
}
.header-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
