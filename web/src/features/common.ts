import { DashboardIcon, AppIcon, UserIcon, TimeIcon, FileIcon, RootListIcon, SettingIcon, CertificateIcon, ServerIcon } from 'tdesign-icons-vue-next'
import type { Feature } from './types'

export const commonFeatures: Feature[] = [
  { path: 'dashboard', permission: 'dashboard', capability: 'admin', label: 'menu.dashboard', desc: 'menuDesc.dashboard', icon: DashboardIcon, sidebar: true, component: () => import('@/views/dashboard/DashboardPage.vue') },
  { path: 'extensions', permission: 'extensions', capability: 'extensions', label: 'menu.extensions', desc: 'menuDesc.extensions', icon: AppIcon, component: () => import('@/views/extensions/Extensions.vue') },
  { path: 'extensions/:extensionId/:pageId', permission: 'extensions', capability: 'extensions', label: 'menu.extensions', desc: 'menuDesc.extensions', icon: AppIcon, component: () => import('@/views/extensions/ExtensionPage.vue') },
  { path: 'instances', permission: 'instances', capability: 'instances', label: 'menu.instances', desc: 'menuDesc.instances', icon: ServerIcon, sidebar: true, component: () => import('@/views/instances/Instances.vue') },
  { path: 'accounts', permission: 'accounts', capability: 'accounts', label: 'menu.accounts', desc: 'menuDesc.accounts', icon: UserIcon, sidebar: true, component: () => import('@/views/accounts/Accounts.vue') },
  { path: 'proxies', permission: 'proxies', capability: 'proxies', label: 'menu.proxies', desc: 'menuDesc.proxies', icon: RootListIcon, sidebar: true, component: () => import('@/views/proxies/Proxies.vue') },
  { path: 'oauth', permission: 'oauth', capability: 'oauth', label: 'menu.oauth', desc: 'menuDesc.oauth', icon: CertificateIcon, sidebar: true, component: () => import('@/views/oauth/OAuth.vue') },
  { path: 'tasks', permission: 'tasks', capability: 'tasks', label: 'menu.tasks', desc: 'menuDesc.tasks', icon: TimeIcon, sidebar: true, component: () => import('@/views/tasks/Tasks.vue') },
  { path: 'logs', permission: 'logs', capability: 'logs', label: 'menu.logs', desc: 'menuDesc.logs', icon: FileIcon, sidebar: true, component: () => import('@/views/logs/Logs.vue') },
  { path: 'settings', permission: 'settings', capability: 'settings', label: 'menu.settings', desc: 'menuDesc.settings', icon: SettingIcon, component: () => import('@/views/settings/Settings.vue') },
  { path: 'profile', label: 'common.profile', desc: 'menuDesc.profile', icon: UserIcon, component: () => import('@/views/profile/Profile.vue') },
]
