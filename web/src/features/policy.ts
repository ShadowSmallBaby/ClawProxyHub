// 菜单与路由共用同一判定，未安装、停用或不可用的模块不贡献能力。
export interface ModuleState {
  id: string
  capabilities: string[]
  installed: boolean
  enabled: boolean
  available: boolean
  restart_required?: boolean
}
export interface Capabilities { profile: string; modules: ModuleState[] }
export interface Access { role: string; menus: string[]; capabilities: string[]; legacy: boolean; username: string }
export interface Requirement { permission?: string; capability?: string }

export function availableCapabilities(response: Capabilities): string[] {
  if (!response || !Array.isArray(response.modules)) throw new Error('Invalid capabilities response')
  return [...new Set(response.modules.filter(m => m.installed && m.enabled && m.available).flatMap(m => m.capabilities ?? []))]
}
export function permits(access: Access | null, feature: Requirement): boolean {
  if (!access) return false
  if (feature.permission && !access.menus.includes(feature.permission)) return false
  if (access.role !== 'admin' && feature.permission && !['dashboard', 'logs'].includes(feature.permission)) return false
  return !feature.capability || access.capabilities.includes(feature.capability)
}
