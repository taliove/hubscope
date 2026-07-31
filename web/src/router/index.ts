import { createRouter, createWebHistory } from 'vue-router'
import { fetchAuthStatus } from '@/api/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@/views/DashboardView.vue'),
      meta: { public: true },
    },
    {
      path: '/endpoints/:id',
      name: 'endpoint-detail',
      component: () => import('@/views/EndpointDetailView.vue'),
      meta: { public: true },
    },
    {
      path: '/benchmark',
      name: 'public-board',
      component: () => import('@/views/BoardView.vue'),
      // Public eval board (ticket 81, spec 0010): the newest settled batch,
      // anonymous like the status board; /eval stays the session-gated
      // full version. Renamed /board → /benchmark in the v2 IA (GH #112,
      // spec 0018).
      meta: { public: true },
    },
    {
      // Legacy path (GH #112): shared /board links keep working — the SPA
      // redirect lands them on /benchmark. The server's NotFound SPA
      // fallback serves index.html for /board, so this client redirect is
      // the 301-equivalent for deep links.
      path: '/board',
      redirect: '/benchmark',
    },
    {
      path: '/eval',
      name: 'eval-leaderboard',
      component: () => import('@/views/EvalLeaderboardView.vue'),
      // Eval APIs require a session since ticket 16; the guard bounces
      // anonymous visitors to /login.
    },
    {
      path: '/campaigns/:id/report',
      name: 'campaign-report',
      component: () => import('@/views/CampaignReportView.vue'),
      // Session-gated like the eval center it is entered from (ticket 31).
    },
    {
      path: '/report/:token',
      name: 'shared-report',
      component: () => import('@/views/CampaignReportView.vue'),
      // Token-gated read-only report (ticket 33, ADR 0006): the same view in
      // shared mode, no login required. bare = rendered outside the sidebar
      // shell (spec 0018 IA: the shared report shows no sidebar and no login
      // entry — the recipient is not a console reader).
      meta: { public: true, bare: true },
    },
    {
      path: '/models',
      name: 'models',
      component: () => import('@/views/ModelsView.vue'),
      // Session-gated admin surface (GH #112): AdminView's resources/rules
      // panels moved into the shell; the v2 rebuild lands in T8.
    },
    {
      path: '/alerts',
      name: 'alerts',
      component: () => import('@/views/AlertsView.vue'),
      // Session-gated alert history (GH #112); the timeline rebuild is T10.
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
      // Session-gated settings area (GH #112): settings/audit/users plus the
      // folded-in task center (spec 0018 decision 63); the rebuild is T11.
    },
    {
      path: '/tasks',
      name: 'task-center',
      component: () => import('@/views/TaskCenterView.vue'),
      // Task reads are session-gated monitoring data, like eval runs.
      // Legacy entry point (GH #112): the task center is folded into
      // /settings in the v2 IA; this route stays alive for deep links and
      // highlights the 系统设置 sidebar item.
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      // bare = rendered outside the sidebar shell (spec 0018 IA).
      meta: { public: true, bare: true },
    },
    {
      path: '/admin',
      name: 'admin',
      component: () => import('@/views/AdminView.vue'),
      // Transitional (GH #112): kept alive until T11 — the eval-ops and
      // case-library tabs still live only here. Not in the sidebar.
    },
  ],
})

// Page-level guard: the status board (dashboard, endpoint detail) stays
// public like its read APIs; admin and eval pages bounce unauthenticated
// visitors to /login. This is only UX guidance; the server re-validates the
// session on every protected API call.
router.beforeEach(async (to) => {
  if (to.meta.public) return true
  try {
    const status = await fetchAuthStatus()
    if (status.authenticated) return true
  } catch {
    // Fall through: a failed status check is treated as unauthenticated.
  }
  return { path: '/login', query: { redirect: to.fullPath } }
})

export default router
