import { createRouter, createWebHistory } from 'vue-router'
import { fetchAuthStatus } from '@/api/auth'
import { legacyAdminTarget } from '@/utils/adminNav'

// First query value of a possibly-repeated route query param (router-level
// mirror of the adminNav parsing discipline).
function firstQueryValue(raw: unknown): string | null {
  const first = Array.isArray(raw) ? raw[0] : raw
  return typeof first === 'string' ? first : null
}

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
      // Session-gated admin surface (GH #112 shell, GH #119 migration):
      // AdminView's resources/rules panes live here (spec 0018 IA).
    },
    {
      path: '/alerts',
      name: 'alerts',
      component: () => import('@/views/AlertsView.vue'),
      // Public since spec 0019 (GH #142): anonymous readers get the
      // four-kind incident narrative (down / recovered / group_down /
      // group_recovered); the seven ops-pipeline kinds stay session-gated.
      // The boundary is enforced server-side (ui-guidelines appendix item
      // 16) — this flag only stops the guard from bouncing to /login. The
      // timeline rebuild landed in GH #117.
      meta: { public: true },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
      // Session-gated settings area (GH #112 shell, GH #119 migration):
      // settings/audit/users plus the folded-in task center (spec 0018
      // decision 63).
    },
    {
      // Legacy path (GH #119): the task center folds into /settings in the
      // v2 IA; the redirect keeps shared /tasks deep links landing on the
      // tasks pane.
      path: '/tasks',
      redirect: { path: '/settings', query: { tab: 'tasks' } },
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      // bare = rendered outside the sidebar shell (spec 0018 IA).
      meta: { public: true, bare: true },
    },
    {
      // Legacy path (GH #119): AdminView retired. The redirect re-targets
      // every legacy ?tab= deep link to the console that pane landed on
      // (/models, /settings, or the /eval secondary tabs) via the pure,
      // vitest-covered legacyAdminTarget mapping; unknown tabs fall back to
      // /settings (spec 0018).
      path: '/admin',
      redirect: (to) => legacyAdminTarget(firstQueryValue(to.query.tab), firstQueryValue(to.query.item)),
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
