// 能力与权限快照仅属于当前会话；连接切换和登录变更立即失效。
import { shallowRef } from 'vue'
import { authApi } from '@/api/auth'
import { capabilitiesApi } from '@/api/capabilities'
import { HTTPError } from '@/api/client'
import { connectionGeneration, onConnectionReset } from '@/api/connections'
import { availableCapabilities, permits, type Access } from './policy'

export const backendAccess = shallowRef<Access | null>(null)
let pending: Promise<void> | undefined
let loadedAt = 0
onConnectionReset(() => { backendAccess.value = null; pending = undefined; loadedAt = 0 })

export async function ensureAccess(): Promise<void> {
  if (backendAccess.value && Date.now() - loadedAt < 30_000) return
  if (pending) return pending
  const generation = connectionGeneration()
  const request = (async () => {
    const [me, caps] = await Promise.all([
      authApi.me(),
      capabilitiesApi.get().catch(error => {
        // 仅旧后端明确缺少端点时兼容；网络、鉴权和响应格式错误不能扩大权限。
        if (error instanceof HTTPError && error.status === 404) return null
        throw error
      }),
    ])
    if (generation !== connectionGeneration()) throw new DOMException('Connection changed', 'AbortError')
    backendAccess.value = {
      ...me, menus: me.menus ?? [], legacy: caps === null,
      capabilities: caps ? availableCapabilities(caps) : ['admin', 'auth', 'logs', 'settings', 'plugins', 'accounts', 'instances', 'groups', 'proxies', 'oauth', 'tasks', 'gateway', 'routes', 'keys'],
    }
    loadedAt = Date.now()
  })()
  pending = request
  try { await request }
  finally { if (pending === request) pending = undefined }
}
export function hasCapability(capability: string) { return backendAccess.value?.capabilities.includes(capability) ?? false }
export function canUse(permission: string, capability = permission) { return permits(backendAccess.value, { permission, capability }) }
