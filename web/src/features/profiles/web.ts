import { AppIcon } from 'tdesign-icons-vue-next'
import { features as business, gateway } from '../full'
export { gateway }
export const features = [...business]
features.splice(1, 0,
  { path: 'plugins', permission: 'plugins', capability: 'plugins', label: 'menu.plugins', desc: 'menuDesc.plugins', icon: AppIcon, sidebar: true, component: () => import('@/views/plugins/Plugins.vue') },
)
