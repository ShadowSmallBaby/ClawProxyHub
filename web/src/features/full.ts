import { defineAsyncComponent } from 'vue'
import { modelsApi } from '@/api/gateway'
import { InternetIcon, LockOnIcon, FolderIcon } from 'tdesign-icons-vue-next'
import { commonFeatures } from './common'
import type { Feature, GatewayUI } from './types'

export const features: Feature[] = [...commonFeatures]
features.splice(5, 0,
  { path: 'groups', permission: 'groups', capability: 'groups', label: 'menu.groups', desc: 'menuDesc.groups', icon: FolderIcon, sidebar: true, component: () => import('@/views/groups/Groups.vue') },
  { path: 'routes', permission: 'routes', capability: 'routes', label: 'menu.routes', desc: 'menuDesc.routes', icon: InternetIcon, sidebar: true, component: () => import('@/views/routes/Routes.vue') },
  { path: 'keys', permission: 'keys', capability: 'keys', label: 'menu.keys', desc: 'menuDesc.keys', icon: LockOnIcon, sidebar: true, component: () => import('@/views/keys/Keys.vue') },
)
export const gateway: GatewayUI = {
  models: modelsApi,
  dashboard: defineAsyncComponent(() => import('@/views/dashboard/Dashboard.vue')),
  logs: defineAsyncComponent(() => import('@/views/logs/RequestLogs.vue')),
  accountTest: defineAsyncComponent(() => import('@/views/accounts/AccountTest.vue')),
  logActions: defineAsyncComponent(() => import('@/views/settings/GatewayLogActions.vue')),
  settings: defineAsyncComponent(() => import('@/views/settings/GatewaySettings.vue')),
}
