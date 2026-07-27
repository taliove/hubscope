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
      path: '/board',
      name: 'public-board',
      component: () => import('@/views/BoardView.vue'),
      // Public eval board (ticket 81, spec 0010): the newest settled batch,
      // anonymous like the status board; /eval stays the session-gated
      // full version.
      meta: { public: true },
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
      // shared mode, no login required.
      meta: { public: true },
    },
    {
      path: '/tasks',
      name: 'task-center',
      component: () => import('@/views/TaskCenterView.vue'),
      // Task reads are session-gated monitoring data, like eval runs.
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/admin',
      name: 'admin',
      component: () => import('@/views/AdminView.vue'),
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
