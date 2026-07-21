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

// Page-level guard: read pages (dashboard) stay public like the read APIs;
// only admin pages bounce unauthenticated visitors to /login. This is only
// UX guidance; the server re-validates the session on every write.
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
