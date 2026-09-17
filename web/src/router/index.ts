import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from '../api/client'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../views/Login.vue') },
    { path: '/setup', component: () => import('../views/Setup.vue') },
    {
      path: '/',
      component: () => import('../views/Layout.vue'),
      children: [
        { path: '', redirect: '/dashboard' },
        { path: 'dashboard', component: () => import('../views/Dashboard.vue') },
        { path: 'plugins', component: () => import('../views/Plugins.vue') },
        { path: 'accounts', component: () => import('../views/Accounts.vue') },
        { path: 'groups', component: () => import('../views/Groups.vue') },
        { path: 'proxies', component: () => import('../views/Proxies.vue') },
        { path: 'routes', component: () => import('../views/Routes.vue') },
        { path: 'keys', component: () => import('../views/Keys.vue') },
        { path: 'tasks', component: () => import('../views/Tasks.vue') },
        { path: 'logs', component: () => import('../views/Logs.vue') },
        { path: 'settings', component: () => import('../views/Settings.vue') },
      ],
    },
  ],
})

router.beforeEach((to) => {
  if (to.path !== '/login' && to.path !== '/setup' && !getToken()) return '/login'
})

export default router
