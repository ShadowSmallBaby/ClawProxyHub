import { createRouter, createWebHashHistory, createWebHistory } from 'vue-router'
import { getToken } from '@/api/client'
import { currentConnection, supportsConnections } from '@/api/connections'
import { features } from '@profile'
import { backendAccess, ensureAccess } from '@/features/access'
import { permits } from '@/features/policy'
import { initializeApplication } from '@/api/application'

const unavailablePath = supportsConnections ? '/connections' : '/unavailable'
const router = createRouter({
  history: supportsConnections ? createWebHashHistory() : createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/login', component: () => import('@/views/auth/Login.vue'), meta: { public: true } },
    { path: '/setup', component: () => import('@/views/auth/Setup.vue'), meta: { public: true } },
    { path: unavailablePath, component: () => import('@/views/auth/ConnectionStatus.vue'), meta: { public: true } },
    {
      path: '/', component: () => import('@/layouts/AppLayout.vue'),
      children: features.map(feature => ({ path: feature.path, component: feature.component, meta: { feature: feature.path } })),
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async to => {
  if (supportsConnections) {
    try { await initializeApplication() }
    catch { if (to.path !== '/connections') return '/connections' }
  }
  if (supportsConnections && currentConnection().id === 'same-origin' && to.path !== '/connections') return '/connections'
  if (to.path === '/login' && getToken()) return '/'
  if (to.meta.public) return
  if (!getToken()) return '/login'
  try { await ensureAccess() }
  catch (error: any) {
    if (error.name === 'AbortError') return false
    return getToken() ? unavailablePath : '/login'
  }
  const feature = features.find(f => f.path === to.meta.feature)
  if (!feature || !permits(backendAccess.value, feature)) {
    const fallback = features.find(f => permits(backendAccess.value, f))
    return fallback ? `/${fallback.path}` : unavailablePath
  }
})

window.addEventListener('cph:unauthorized', () => {
  if (!['/login', '/setup'].includes(router.currentRoute.value.path)) void router.replace('/login')
})
export default router
