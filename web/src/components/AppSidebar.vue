<template>
  <!-- Nav-drawer scrim (2026-08-01 shell drawer batch): rendered only while
       the drawer is open — and the drawer only ever opens on narrow
       viewports (App.vue force-closes it when the viewport leaves the
       1024px breakpoint), so this overlay never appears on desktop. The
       click walks the same close-drawer emit as ESC. -->
  <Transition name="drawer-fade">
    <div v-if="drawerOpen" class="drawer-scrim" aria-hidden="true" @click="emit('close-drawer')" />
  </Transition>
  <aside ref="sidebarEl" class="app-sidebar" :class="{ 'drawer-open': drawerOpen }">
    <!-- The brand seat moved to AppTopbar when the full-width header was
         restored (GH #135) — the sidebar starts below the header with the
         nav itself. -->
    <nav class="side-nav" aria-label="主导航">
      <router-link
        v-for="item in visibleItems"
        :key="item.key"
        :to="item.to"
        class="nav-item"
        :class="{ 'nav-active': isSidebarItemActive(item, route.path) }"
      >
        <component :is="SIDEBAR_ICONS[item.key]" class="nav-icon" />
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
      <!-- User card (GH #135, reference design; GH #139: white tile on the
           gray sidebar ground): avatar placeholder with a presence dot,
           name and role. The logout action moved to the AppTopbar user
           chip; anonymous visitors see no card (the quiet admin entry
           lives in the topbar, ticket 90 spirit). -->
      <div v-if="user" class="user-card">
        <span class="avatar">
          <CircleUserRoundIcon class="avatar-icon" />
          <span class="avatar-dot" aria-hidden="true"></span>
        </span>
        <span class="user-meta">
          <span class="user-name" :title="user.username">{{ user.username }}</span>
          <span class="user-role">{{ roleLabel(user.role) }}</span>
        </span>
      </div>
      <!-- Legal line (GH #122 conservative restore; GH #139 single-line
           layout): a hairline separates it from the user card above, and
           the copyright and the version share ONE line — the version's
           title carries the full build string. -->
      <div class="footer-legal">
        <span>© 2026 HubScope</span>
        <template v-if="shortVersion">
          <span aria-hidden="true">·</span>
          <span class="version" :title="`HubScope ${version}`">{{ shortVersion }}</span>
        </template>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
// AppSidebar (GH #112, spec 0018; GH #135 header restore): the v2 220px
// shell sidebar, sitting below AppTopbar. Self-built signature surface — no
// Element Plus layout components; the only EP consumption is the batch
// entry's Loading spinner. Nav glyphs are the Lucide-style inline SVG set
// (components/icons/lucide.ts). Session state is checked locally on mount
// and re-checked on every route change — deliberately no state store
// (AppHeader precedent).
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Loading } from '@element-plus/icons-vue'
import { fetchAuthStatus } from '@/api/auth'
import type { AuthUser } from '@/api/auth'
import { listCampaigns } from '@/api/campaigns'
import type { Campaign } from '@/api/types'
import { fetchVersion } from '@/api/version'
import { roleLabel } from '@/utils/role'
import { shortVersion as shortenVersion } from '@/utils/version'
import { isSidebarItemActive, visibleSidebarItems } from '@/utils/sidebarNav'
import { createVisibilityPoll, type VisibilityPollHandle } from '@/utils/visibilityPoll'
import { createFocusTrap, type FocusTrapHandle } from '@/utils/focusTrap'
import { CircleUserRoundIcon, SIDEBAR_ICONS } from './icons/lucide'

// drawerOpen is owned by App.vue (the topbar hamburger toggles it, route
// changes and desktop resizes force it false); this component only renders
// the drawer form and reports close intents upward.
const props = withDefaults(defineProps<{ drawerOpen?: boolean }>(), { drawerOpen: false })
const emit = defineEmits<{ 'close-drawer': [] }>()

const route = useRoute()
const router = useRouter()

// --- Nav drawer (narrow viewports) -------------------------------------------
// The self-built modal trio applied to the drawer: focus trap (Tab cycles
// inside the drawer while it is open), unified close (ESC / scrim click /
// nav selection all end in the same close-drawer emit — nav selection
// closes via App.vue's route watch), and focus return (App.vue lands focus
// back on the hamburger via the data-drawer-toggle anchor).
const sidebarEl = ref<HTMLElement | null>(null)
let drawerTrap: FocusTrapHandle | null = null

function onDrawerKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close-drawer')
}

watch(
  () => props.drawerOpen,
  async (open) => {
    if (open) {
      await nextTick()
      document.addEventListener('keydown', onDrawerKeydown)
      if (sidebarEl.value) {
        drawerTrap = createFocusTrap(sidebarEl.value)
        sidebarEl.value.querySelector<HTMLElement>('.nav-item')?.focus()
      }
    } else {
      document.removeEventListener('keydown', onDrawerKeydown)
      drawerTrap?.deactivate()
      drawerTrap = null
    }
  },
)

// Local session identity; `user` is null when unauthenticated and every
// authed-only branch reads it. A failed status check is treated as
// unauthenticated, same as the router guard.
const user = ref<AuthUser | null>(null)

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

// Build version (spec 0018 IA: the sidebar footer carries the version — the
// single-binary deployment has no multi-environment concept, so the version
// answers "which build is this"). Fetched once; failure stays silent.
const version = ref('')
// Dev build stamps are shortened to dev-g<hash> for display; the full stamp
// stays in the title attribute (GH #146, display-layer-only change).
const shortVersion = computed(() => shortenVersion(version.value))

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
onBeforeUnmount(() => {
  stopBatchPolling()
  document.removeEventListener('keydown', onDrawerKeydown)
  drawerTrap?.deactivate()
})
</script>

<style scoped>
.app-sidebar {
  position: sticky;
  /* The sticky seat starts right below AppTopbar — the 56px mirrors the
     header height there; always change both places together. */
  top: 56px;
  align-self: flex-start;
  width: 220px;
  flex: none;
  height: calc(100vh - 56px);
  display: flex;
  flex-direction: column;
  padding: var(--hs-space-4) var(--hs-space-3);
  /* GH #139: the sidebar shares the subtle gray skeleton ground with the
     main lane; the white layering is carried by the user card tile. */
  background: var(--hs-bg-subtle);
  border-right: 1px solid var(--hs-border-light);
  overflow-y: auto;
}
.side-nav {
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  /* GH #135 reference design: ~48px rows with an 18px glyph. */
  min-height: 48px;
  padding: 0 var(--hs-space-3);
  border-radius: var(--hs-radius-lg);
  font-size: var(--hs-text-md);
  color: var(--hs-text-regular);
  text-decoration: none;
  transition:
    background-color var(--hs-transition),
    color var(--hs-transition);
}
.nav-icon {
  width: 18px;
  height: 18px;
  flex: none;
  color: var(--hs-text-secondary);
  transition: color var(--hs-transition);
}
.nav-item:hover {
  background: var(--hs-bg-hover);
}
/* Active = soft-blue pill (GH #135): brand-soft ground, brand word and
   glyph — no strong background block. */
.nav-active,
.nav-active:hover {
  background: var(--hs-brand-soft);
  color: var(--hs-brand);
}
.nav-active .nav-icon,
.nav-item:hover .nav-icon {
  color: inherit;
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
  min-height: 48px;
  padding: 0 var(--hs-space-3);
  border: none;
  border-radius: var(--hs-radius-lg);
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
  display: flex;
  flex-direction: column;
  gap: var(--hs-space-2);
}
/* White tile on the gray sidebar ground (GH #139): the same light-container
   syntax as the content regions — bg-card + 1px border + radius-lg. */
.user-card {
  display: flex;
  align-items: center;
  gap: var(--hs-space-2);
  min-width: 0;
  padding: var(--hs-space-2) var(--hs-space-3);
  background: var(--hs-bg-card);
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-lg);
}
.avatar {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  flex: none;
  border-radius: var(--hs-radius-full);
  background: var(--hs-bg-subtle);
}
.avatar-icon {
  width: 20px;
  height: 20px;
  color: var(--hs-text-secondary);
}
/* Presence dot (reference design): solid success green, ringed by the card
   ground so it reads as a badge on the avatar edge — the user card is a
   white tile (GH #139), so the bg-card ring matches its surface. */
.avatar-dot {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 8px;
  height: 8px;
  border-radius: var(--hs-radius-full);
  background: var(--hs-success);
  border: 2px solid var(--hs-bg-card);
  box-sizing: content-box;
}
.user-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.user-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--hs-text-md);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.user-role {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
/* Legal line (GH #139): the hairline moved here from the footer box — it
   now separates the white user card from the single copyright · version
   line below it. */
.footer-legal {
  display: flex;
  align-items: baseline;
  gap: var(--hs-space-1);
  padding-top: var(--hs-space-3);
  border-top: 1px solid var(--hs-border-light);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.version {
  font-family: ui-monospace, 'SF Mono', 'Cascadia Mono', Consolas, monospace;
}

/* --- Nav drawer (narrow viewports, 2026-08-01 shell drawer batch) ---------
   Below the 1024px breakpoint the sidebar leaves the flex row and becomes a
   fixed overlay drawer UNDER the topbar (top: 56px — the bar stays visible
   so its hamburger remains the toggle). Closed = translated off-canvas +
   visibility hidden (drops out of the a11y tree and the tab order); the
   visibility transition delays the hide until the slide-out finishes.
   Open = the drawer slot of the z scale + the lg overlay shadow. The
   desktop form above is pixel-untouched (>=1024px this block never
   matches). */
@media (max-width: 1023px) {
  .app-sidebar {
    position: fixed;
    left: 0;
    top: 56px;
    z-index: var(--hs-z-drawer);
    /* 260px: slightly wider than the 220px desktop rail — touch targets
       read better with the extra breathing room. */
    width: 260px;
    background: var(--hs-bg-card);
    transform: translateX(-105%);
    visibility: hidden;
    transition:
      transform var(--hs-transition),
      visibility var(--hs-transition);
  }
  .app-sidebar.drawer-open {
    transform: none;
    visibility: visible;
    box-shadow: var(--hs-shadow-lg);
  }
}
/* Scrim: covers everything below the topbar (the bar keeps its own white
   face and stays clickable). Rendered only while the drawer is open, which
   only happens on narrow viewports — App.vue force-closes on resize. */
.drawer-scrim {
  position: fixed;
  inset: 56px 0 0 0;
  z-index: var(--hs-z-overlay);
  background: var(--hs-overlay-bg);
}
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity var(--hs-transition);
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
</style>
