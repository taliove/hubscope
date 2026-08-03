<template>
  <header class="app-topbar">
    <!-- Nav-drawer toggle (2026-08-01 shell drawer batch): narrow viewports
         only — the sidebar leaves the flex row and becomes an overlay
         drawer below this bar, and this hamburger is its single toggle
         (menu icon closed, x icon open). data-drawer-toggle is the focus-
         return anchor App.vue lands on after a drawer close. -->
    <button
      type="button"
      class="icon-btn drawer-toggle"
      data-drawer-toggle
      :aria-expanded="drawerOpen"
      :aria-label="drawerOpen ? '关闭导航菜单' : '打开导航菜单'"
      @click="emit('toggle-drawer')"
    >
      <XIcon v-if="drawerOpen" class="icon-glyph" />
      <MenuIcon v-else class="icon-glyph" />
    </button>
    <!-- Brand block (GH #135): BrandMark + Wordmark, click goes home.
         BrandMark is never used bare — it always appears alongside the
         Wordmark. The brand seat moved here from the sidebar top when the
         full-width header was restored. -->
    <router-link to="/" class="brand" aria-label="HubScope 首页">
      <BrandMark class="brand-mark" />
      <Wordmark class="brand-wordmark" />
    </router-link>

    <div class="topbar-right">
      <template v-if="user">
        <!-- Alerts entry: pure navigation (GH #135) — no unread/batch badge
             semantics, the click just deep-links to /alerts. -->
        <button type="button" class="icon-btn" aria-label="故障记录" @click="goAlerts">
          <BellIcon class="icon-glyph" />
        </button>
        <!-- User chip: avatar + name + role; the single dropdown item is the
             logout action (relocated from the sidebar account row). EP's
             dropdown supplies focus/ESC/arrow-key handling, so the
             self-built modal trio (focusTrap) is not needed here. -->
        <el-dropdown trigger="click" @command="onUserCommand">
          <button type="button" class="user-chip">
            <CircleUserRoundIcon class="chip-avatar" />
            <span class="chip-name" :title="user.username">{{ user.username }}</span>
            <span class="chip-role">{{ roleLabel(user.role) }}</span>
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="logout" :disabled="loggingOut">
                <LogOutIcon class="item-icon" />
                退出
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </template>
      <!-- Anonymous right side stays blank (GH #148, 2026-08-02 user ruling):
           the 管理登录 entry moved to the sidebar footer's user slot,
           overturning the ticket 90 "login entry lives in the topbar"
           caliber. -->
    </div>
  </header>
</template>

<script setup lang="ts">
// AppTopbar (GH #135, reference-design replica): the restored full-width
// header — brand on the left, alerts bell + user chip on the right (the
// anonymous right side is blank since GH #148 moved the login entry to the
// sidebar footer). The sidebar starts below it and no longer carries the
// brand seat. Session state is checked locally on mount and re-checked on
// every route change — deliberately no state store (AppSidebar precedent).
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus/es/components/message/index'
import { fetchAuthStatus, logout } from '@/api/auth'
import type { AuthUser } from '@/api/auth'
import { dispatchAuthChanged } from '@/utils/authEvents'
import { roleLabel } from '@/utils/role'
import BrandMark from './BrandMark.vue'
import Wordmark from './Wordmark.vue'
import { BellIcon, CircleUserRoundIcon, LogOutIcon, MenuIcon, XIcon } from './icons/lucide'

// drawerOpen mirrors the shell's nav-drawer state (App.vue owns it) so the
// toggle shows menu/x and the right aria-expanded; the toggle itself only
// reports clicks upward.
withDefaults(defineProps<{ drawerOpen?: boolean }>(), { drawerOpen: false })
const emit = defineEmits<{ 'toggle-drawer': [] }>()

const route = useRoute()
const router = useRouter()

// Local session identity; `user` is null when unauthenticated and every
// authed-only branch reads it. A failed status check is treated as
// unauthenticated, same as the router guard.
const user = ref<AuthUser | null>(null)
const loggingOut = ref(false)

async function refreshAuth() {
  try {
    user.value = (await fetchAuthStatus()).user ?? null
  } catch {
    user.value = null
  }
}

function goAlerts() {
  void router.push('/alerts')
}

async function onUserCommand(command: string) {
  if (command !== 'logout' || loggingOut.value) return
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
  // Notify the shell's auth listeners (GH #148): pushing '/' changes no
  // route when the user is already there, so without this event the
  // sidebar menu, user card, and RecentEvents gate would stay logged-in.
  // Emitted only after a successful logout — a listener's re-check must
  // observe the dead session.
  dispatchAuthChanged()
  router.push('/')
}

onMounted(() => void refreshAuth())
watch(() => route.fullPath, refreshAuth)
</script>

<style scoped>
.app-topbar {
  position: sticky;
  top: 0;
  z-index: var(--hs-z-sticky);
  display: flex;
  align-items: center;
  justify-content: space-between;
  /* GH #135 reference design (~56–60px). AppSidebar's sticky top/height
     mirror this value — always change both places together. */
  height: 56px;
  flex: none;
  padding: 0 var(--hs-space-5);
  background: var(--hs-bg-card);
  border-bottom: 1px solid var(--hs-border-light);
}
.brand {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  text-decoration: none;
  /* With the narrow-viewport hamburger the bar has THREE children and
     space-between would center the brand — the auto margin pins it next to
     the hamburger (or the left edge on desktop, where the toggle is
     display:none and this changes nothing). */
  margin-right: auto;
}
.brand-mark {
  font-size: 24px;
}
.brand-wordmark {
  font-size: var(--hs-text-lg);
}
.topbar-right {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
}
.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: none;
  border-radius: var(--hs-radius-sm);
  background: none;
  color: var(--hs-text-secondary);
  cursor: pointer;
  transition:
    background-color var(--hs-transition),
    color var(--hs-transition);
}
.icon-btn:hover {
  background: var(--hs-bg-hover);
  color: var(--hs-text-regular);
}
.icon-glyph {
  width: 18px;
  height: 18px;
}
/* The drawer toggle exists only on narrow viewports (the sidebar's overlay
   form); on desktop the sidebar is always visible and there is nothing to
   toggle. */
.drawer-toggle {
  display: none;
}
@media (max-width: 1023px) {
  .drawer-toggle {
    display: inline-flex;
  }
  .app-topbar {
    padding: 0 var(--hs-space-3);
  }
}
.user-chip {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
  padding: var(--hs-space-1) var(--hs-space-2);
  border: none;
  border-radius: var(--hs-radius-full);
  background: none;
  cursor: pointer;
  transition: background-color var(--hs-transition);
}
.user-chip:hover {
  background: var(--hs-bg-hover);
}
.chip-avatar {
  width: 20px;
  height: 20px;
  flex: none;
  color: var(--hs-text-secondary);
}
.chip-name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-md);
  color: var(--hs-text-regular);
}
.chip-role {
  flex: none;
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.icon-btn:focus-visible,
.user-chip:focus-visible {
  outline: 2px solid var(--hs-brand);
  outline-offset: 1px;
}
.item-icon {
  width: 16px;
  height: 16px;
  margin-right: var(--hs-space-2);
  vertical-align: -3px;
}
</style>
