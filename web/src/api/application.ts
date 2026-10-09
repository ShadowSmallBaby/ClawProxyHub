import { shallowRef } from 'vue'
import { applyWorkspace, setTokenWriter, supportsConnections, type BackendConnection } from './connections'
import { setLocale } from '@/i18n'
import { useTheme } from '@/composables/useTheme'

interface PlatformBridge { postMessage(message: string): void; onmessage?: (event: { data: string }) => void }
interface ApplicationState { version: string; package: string; remoteOnly: boolean; locale: string; theme: string; notification?: number; connection?: BackendConnection & { token: string } }
const bridge = typeof window === 'undefined' ? undefined : (window as unknown as { cphPlatform?: PlatformBridge }).cphPlatform
export const isAndroidApp = supportsConnections && !!bridge && window.top === window
export const applicationError = shallowRef('')
export const notificationToOpen = shallowRef(0)
const pending = new Map<string, { resolve(value: unknown): void; reject(error: Error): void; timer: ReturnType<typeof setTimeout> }>()
if (isAndroidApp && bridge) bridge.onmessage = event => {
  try {
    const response = JSON.parse(event.data), call = pending.get(response.id)
    if (!call) return
    clearTimeout(call.timer); pending.delete(response.id)
    if (response.error) call.reject(new Error(response.error)); else call.resolve(response.result)
  } catch { /* 忽略其他页面或过期回执。 */ }
}
function request<T>(type: string, input: Record<string, unknown> = {}): Promise<T> {
  if (!isAndroidApp || !bridge) return Promise.reject(new Error('Android application bridge unavailable'))
  const id = crypto.randomUUID()
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('Application operation timed out')) }, 60000)
    pending.set(id, { resolve: value => resolve(value as T), reject, timer })
    try { bridge.postMessage(JSON.stringify({ ...input, type, id })) }
    catch (error) { clearTimeout(timer); pending.delete(id); reject(error) }
  })
}
if (isAndroidApp) setTokenWriter((connection, token) => request('workspace-token', { connection, token }))
let initialization: Promise<void> | undefined
export function copyApplicationText(text: string) { return request('clipboard-write', { text }) }
export function syncApplicationNotifications(connection: string, notifications: import('./auth').Notification[], unread: number) {
  return request<{ native: boolean }>('notifications-sync', { connection, notifications, unread })
}
export function initializeApplication(): Promise<void> {
  if (!supportsConnections) return Promise.resolve()
  return initialization ??= (async () => {
    // 旧 Web 连接目录只用于一次迁移，原生确认后移除网页中的凭据副本。
    const legacy: Record<string, string> = {}
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)!
      if (key.startsWith('cph-token:') || ['cph-connections', 'cph-active-connection', 'cph-theme', 'cph-locale'].includes(key)) legacy[key] = localStorage.getItem(key)!
    }
    const state = await request<ApplicationState>('workspace-state', { legacy })
    for (const key of Object.keys(legacy)) if (key.startsWith('cph-token:') || ['cph-connections', 'cph-active-connection'].includes(key)) localStorage.removeItem(key)
    setLocale(state.locale === 'en' ? 'en' : 'zh')
    useTheme().dark.value = state.theme === 'dark'
    if (state.connection) applyWorkspace(state.connection, state.connection.token)
    notificationToOpen.value = state.notification ?? 0
  })().catch(error => { applicationError.value = String(error.message || error); throw error })
}
