// Web 固定同源；App 仅接收原生选中的工作台上下文，页面不管理连接目录。
export interface BackendConnection { id: string; name: string; baseURL: string }
export const supportsConnections = typeof __CPH_PROFILE__ !== 'undefined' && __CPH_PROFILE__.startsWith('app-')
export const DEVICE_CONNECTION_ID = 'android-local-core'
const LOCAL: BackendConnection = { id: 'same-origin', name: '', baseURL: '' }
let active = LOCAL
let appToken = ''
let generation = 0
let tokenWriter: ((id: string, token: string) => Promise<unknown>) | undefined
const listeners = new Set<() => void>()
const TOKEN_KEY = 'cph-token:same-origin'
if (!supportsConnections && localStorage.getItem('cph-admin-token')) {
  if (!localStorage.getItem(TOKEN_KEY)) localStorage.setItem(TOKEN_KEY, localStorage.getItem('cph-admin-token')!)
  localStorage.removeItem('cph-admin-token')
}
export function normalizeBackendURL(value: string): string {
  const url = new URL(value.trim())
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && loopback)) throw new Error('connections.httpsRequired')
  if (url.username || url.password || url.search || url.hash) throw new Error('connections.invalidURL')
  return url.href.replace(/\/+$/, '')
}
export function currentConnection(): BackendConnection { return { ...active } }
export function connectionGeneration() { return generation }
export function onConnectionReset(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener) } }
function reset() { generation++; for (const listener of listeners) listener() }
export function applyWorkspace(connection: BackendConnection, token: string) {
  if (!supportsConnections || !connection.id || connection.id === LOCAL.id) throw new Error('Invalid application workspace')
  active = { ...connection, baseURL: normalizeBackendURL(connection.baseURL) }
  appToken = token
  reset()
}
export function setTokenWriter(writer: typeof tokenWriter) { tokenWriter = writer }
export function getToken() { return supportsConnections ? appToken : localStorage.getItem(TOKEN_KEY) ?? '' }
export async function setToken(token: string) {
  if (supportsConnections) {
    const id = active.id, revision = generation
    if (!tokenWriter) throw new Error('Native credential storage unavailable')
    await tokenWriter(id, token)
    if (active.id !== id || generation !== revision) throw new DOMException('Workspace changed', 'AbortError')
    appToken = token
  } else if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
  reset()
}
export function clearToken() { return setToken('') }
if (typeof window !== 'undefined') {
  window.addEventListener('cph:workspace-changing', reset)
  window.addEventListener('storage', event => {
    if (!supportsConnections && (event.key === TOKEN_KEY || event.key === null)) {
      reset(); window.location.reload()
    }
  })
}
