<template>
  <header class="app-header">
    <div class="header-inner">
      <!-- Brand block: BrandMark + Wordmark, click goes home. BrandMark is
           never used bare — it always appears alongside the Wordmark
           (ui-guidelines §2b). Version number displays below the brand. -->
      <div class="brand-block">
        <router-link to="/" class="brand">
          <BrandMark class="brand-mark" />
          <Wordmark class="brand-wordmark" />
        </router-link>
        <span v-if="version" class="version" :title="`HubScope ${version}`">{{ shortVersion }}</span>
      </div>

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
        <!-- Theme toggle (ui-guidelines §2a): available to anonymous
             visitors too — the status board serves both audiences. Shows
             the target-state icon (Moon in light theme). -->
        <el-button
          link
          class="theme-toggle"
          :title="dark ? '切换浅色' : '切换深色'"
          @click="toggleTheme"
        >
          <el-icon :size="16"><Sunny v-if="dark" /><Moon v-else /></el-icon>
        </el-button>
        <!-- Batch progress entry (ticket 52): rendered only for logged-in
             users while an unfinished batch exists; the rotating Loading
             icon is the only motion (no orange-red, no flashing). -->
        <el-button
          v-if="user && activeBatch"
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
        <template v-if="user">
          <span class="user-name" :title="user.username">{{ user.username }}</span>
          <el-tag size="small" effect="light" :type="roleTagType(user.role)">{{ roleLabel(user.role) }}</el-tag>
        </template>
        <el-button v-if="user" @click="router.push('/admin')">管理视图</el-button>
        <!-- Login entry (ticket 90): no public page renders it — the button
             reads "this content needs an account" to the anonymous reader,
             so the entry uniformly retreats to the shared PublicFooter. The
             check is the generic route meta.public, not a per-page special
             case. -->
        <el-button v-if="!user && !isPublicPage" type="primary" @click="router.push('/login')">登录</el-button>
        <el-button v-if="user" link type="primary" :loading="loggingOut" @click="onLogout">
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
import { Loading, Moon, Sunny } from '@element-plus/icons-vue'
import { fetchAuthStatus, logout } from '@/api/auth'
import type { AuthUser } from '@/api/auth'
import { listCampaigns } from '@/api/campaigns'
import type { Campaign } from '@/api/types'
import { fetchVersion } from '@/api/version'
import { roleLabel, roleTagType } from '@/utils/role'
import { useTheme } from '@/utils/theme'
import BrandMark from './BrandMark.vue'
import Wordmark from './Wordmark.vue'

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
  // Anonymous-only items (the public eval board) hide once logged in — the
  // session audience gets the full version (/eval) instead.
  anonOnly?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { label: '状态总览', to: '/', public: true },
  // The leaderboard is public since spec 0010: anonymous visitors get the
  // settled-batch board at /board; logged-in users keep /eval.
  { label: '评估榜单', to: '/board', public: true, anonOnly: true },
  { label: '评估榜单', to: '/eval' },
  { label: '任务中心', to: '/tasks' },
]

// Local session identity (no global store by design — ticket 62 keeps the
// blast radius minimal; a useAuth composable may be extracted in 66b/67).
// `user` is null when unauthenticated; every authed-only branch reads it.
const user = ref<AuthUser | null>(null)
const loggingOut = ref(false)
const version = ref<string>('')
const { dark, toggleTheme } = useTheme()

// Anonymous visitors only get the public nav entries; logout flips user
// to null and this computed collapses the nav back immediately.
const visibleNavItems = computed(() =>
  user.value ? NAV_ITEMS.filter((item) => !item.anonOnly) : NAV_ITEMS.filter((item) => item.public),
)

// No public page renders the header login button (ticket 90, superseding
// the /board-only special case): the admin entry uniformly retreats to the
// shared PublicFooter. Non-public pages bounce anonymous visitors to /login
// via the route guard anyway, so the button effectively only ever renders
// in the brief pre-auth-check window of a gated page.
const isPublicPage = computed(() => Boolean(route.meta.public))

// Short version: extract only the tag part (e.g., "v0.2.3" from "v0.2.3-4-g1adea03-dirty")
const shortVersion = computed(() => {
  if (!version.value) return ''
  // Match vX.Y.Z at the start
  const match = version.value.match(/^v\d+\.\d+\.\d+/)
  return match ? match[0] : version.value
})

// The dashboard owns both '/' and the endpoint detail pages.
function isActive(item: NavItem): boolean {
  if (item.to === '/') return route.path === '/' || route.path.startsWith('/endpoints')
  return route.path.startsWith(item.to)
}

// A failed status check is treated as unauthenticated, same as the router guard.
async function refreshAuth() {
  try {
    user.value = (await fetchAuthStatus()).user ?? null
  } catch {
    user.value = null
  }
}

// Fetch version once on mount; it never changes during a session.
async function loadVersion() {
  try {
    const res = await fetchVersion()
    version.value = res.version
  } catch {
    // Silent failure: version display is non-critical.
    version.value = ''
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
  user.value = null
  router.push('/')
}

// Auth gates the batch entry, so it refreshes on the same occasions: mount
// and every route change (the refreshAuth watch precedent).
async function refreshHeaderState() {
  await refreshAuth()
  await refreshBatch()
}

onMounted(() => {
  void refreshHeaderState()
  void loadVersion()
})
watch(() => route.fullPath, refreshHeaderState)
onBeforeUnmount(stopBatchPolling)
</script>

<style scoped>
.app-header {
  position: sticky;
  top: 0;
  z-index: var(--hs-z-sticky);
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
.brand-block {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  text-decoration: none;
}
.brand-mark {
  font-size: 24px;
}
.brand-wordmark {
  font-size: var(--hs-text-lg);
}
.version {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  font-family: ui-monospace, 'SF Mono', 'Cascadia Mono', Consolas, monospace;
  margin-left: 32px;
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
.user-name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-md);
  color: var(--hs-text-regular);
}
</style>
