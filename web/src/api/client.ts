// 分域 API 共用传输：地址、认证、取消、流、上传和下载均绑定发起时的连接。
import { clearToken, connectionGeneration, currentConnection, getToken, onConnectionReset } from './connections.ts'
export { getToken, setToken, clearToken } from './connections.ts'

const pending = new Set<AbortController>()
onConnectionReset(() => {
  for (const controller of pending) controller.abort()
  pending.clear()
})

export class HTTPError extends Error {
  readonly status: number
  constructor(status: number, message: string) { super(message); this.status = status }
}

export function getRole(): string {
  try {
    const payload = JSON.parse(atob(getToken().split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
    return payload.role === 'admin' ? 'admin' : 'guest'
  } catch { return 'guest' }
}

function backendURL(path: string, baseURL: string): string {
  if (!path.startsWith('/') || path.startsWith('//') || /[\\#]/.test(path) || /(?:^|\/)\.\.(?:\/|$)/.test(path)) throw new Error('Invalid backend path')
  return `${baseURL}${path}`
}

// 后端资源不携带 Token；外部图片保留自己的来源，包内相对路径跟随当前后端。
export function resourceURL(path: string): string {
  if (!path) return ''
  if (/^(data:image\/|blob:|https?:\/\/)/i.test(path)) return path
  if (/^[a-z][a-z\d+.-]*:/i.test(path) || path.startsWith('//')) return ''
  try { return backendURL(`/${path.replace(/^\//, '')}`, currentConnection().baseURL) }
  catch { return '' }
}

async function withResponse<T>(path: string, init: RequestInit, consume: (response: Response, check: () => void) => Promise<T>): Promise<T> {
  const generation = connectionGeneration()
  const token = getToken()
  const controller = new AbortController()
  const abort = () => controller.abort()
  init.signal?.addEventListener('abort', abort, { once: true })
  if (init.signal?.aborted) controller.abort()
  pending.add(controller)
  const check = () => {
    if (controller.signal.aborted || generation !== connectionGeneration() || token !== getToken()) throw new DOMException('Connection changed or request cancelled', 'AbortError')
  }
  try {
    check()
    const headers = new Headers(init.headers)
    if (token) headers.set('Authorization', `Bearer ${token}`)
    const resp = await fetch(backendURL(path, currentConnection().baseURL), {
      ...init, headers, signal: controller.signal, credentials: 'omit', redirect: 'error', cache: 'no-store',
    })
    check()
    if (!resp.ok) {
      const body = await resp.text()
      check()
      let message = body || `HTTP ${resp.status}`
      try { message = JSON.parse(body).error ?? message } catch { /* 非 JSON 错误体 */ }
      if (resp.status === 401) {
        await clearToken()
        if (typeof window !== 'undefined') window.dispatchEvent(new Event('cph:unauthorized'))
        throw new HTTPError(401, 'unauthorized')
      }
      throw new HTTPError(resp.status, message)
    }
    const result = await consume(resp, check)
    check()
    return result
  } finally {
    pending.delete(controller)
    init.signal?.removeEventListener('abort', abort)
  }
}

export async function request<T = any>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  return withResponse(path, {
    method, signal, headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }, resp => resp.json())
}

export function upload<T = any>(path: string, body: FormData): Promise<T> {
  return withResponse(path, { method: 'POST', body }, async resp => {
    const text = await resp.text()
    return text ? JSON.parse(text) : undefined
  })
}

export async function downloadFile(path: string, fallbackName: string, flag: { value: boolean }) {
  flag.value = true
  try {
    await withResponse(path, {}, async (resp, check) => {
      const blob = await resp.blob()
      check()
      const name = /filename="?([^";]+)"?/.exec(resp.headers.get('Content-Disposition') ?? '')?.[1] ?? fallbackName
      const platform=(window as unknown as {cphPlatform?:{postMessage:(data:string)=>void}}).cphPlatform
      if(platform){
        if(blob.size>16*1024*1024)throw new Error('Android download exceeds 16 MiB')
        const bytes=new Uint8Array(await blob.arrayBuffer());check()
        let binary='';for(let i=0;i<bytes.length;i+=32768)binary+=String.fromCharCode(...bytes.subarray(i,i+32768))
        platform.postMessage(JSON.stringify({type:'download',name,data:btoa(binary)}));return
      }
      const url = URL.createObjectURL(blob)
      try { Object.assign(document.createElement('a'), { href: url, download: name }).click() }
      finally { URL.revokeObjectURL(url) }
    })
  } finally { flag.value = false }
}

// NDJSON 逐事件核实会话，保证取消后不再交付缓存中的事件。
export async function requestStream(method: string, path: string, body: unknown, onEvent: (ev: any) => void, signal?: AbortSignal): Promise<void> {
  return withResponse(path, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), signal }, async (resp, check) => {
    if (!resp.body) throw new Error('Missing response stream')
    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    const consume = (line: string) => {
      check()
      if (!line.trim()) return
      if (line.length > 1024 * 1024) throw new Error('Progress event exceeds size limit')
      const ev = JSON.parse(line)
      if (ev.error) throw new Error(ev.error)
      onEvent(ev)
    }
    try {
      for (;;) {
        const { value, done } = await reader.read()
        check()
        buf += decoder.decode(value, { stream: !done })
        let nl: number
        while ((nl = buf.indexOf('\n')) >= 0) {
          consume(buf.slice(0, nl))
          buf = buf.slice(nl + 1)
        }
        if (buf.length > 1024 * 1024) throw new Error('Progress event exceeds size limit')
        if (done) { consume(buf); break }
      }
    } finally {
      await reader.cancel().catch(() => {})
      reader.releaseLock()
    }
  })
}

export const api = {
  get: <T = any>(path: string) => request<T>('GET', path),
  post: <T = any>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T = any>(path: string, body?: unknown) => request<T>('PUT', path, body),
  del: <T = any>(path: string) => request<T>('DELETE', path),
}
