// API 客户端：核心请求器 + token 管理。分域接口见同目录 auth/stats/logs/entities/settings.ts。

// 管理员 token 经 localStorage 持久化，请求走 Bearer。
const TOKEN_KEY = 'cph-admin-token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

// getRole 从 JWT 载荷解出角色（仅前端展示/守卫用，真正边界在服务端）。
// 非 JWT（旧 user:password 过渡态）或解析失败一律按 admin，避免误挡。
export function getRole(): string {
  const t = getToken()
  const parts = t.split('.')
  if (parts.length !== 3) return 'admin'
  try {
    const payload = JSON.parse(atob(parts[1].replace(/-/g, '+').replace(/_/g, '/')))
    return payload.role || 'admin'
  } catch {
    return 'admin'
  }
}

// throwIfFailed 非 2xx 统一处理：401 清 token 跳登录，其余抛 {error} 或原文。
async function throwIfFailed(resp: Response) {
  if (resp.status === 401) {
    clearToken()
    if (location.pathname !== '/login') location.href = '/login'
    throw new Error('unauthorized')
  }
  if (!resp.ok) {
    const text = await resp.text()
    let msg = text
    try {
      msg = JSON.parse(text).error ?? text
    } catch { /* 非 JSON 错误体 */ }
    throw new Error(msg)
  }
}

function authHeaders() {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` }
}

export async function request<T = any>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(path, {
    method,
    headers: authHeaders(),
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  await throwIfFailed(resp)
  return resp.json()
}

// requestStream 读 NDJSON 进度流：每行一个 JSON 事件回调；流开始后 HTTP 状态已定，服务端以 {error} 行报错。
// signal 支持中途取消（abort 后 fetch/读取抛 AbortError，调用方据此区分取消与失败）。
export async function requestStream(method: string, path: string, body: unknown, onEvent: (ev: any) => void, signal?: AbortSignal): Promise<void> {
  const resp = await fetch(path, { method, headers: authHeaders(), body: JSON.stringify(body), signal })
  await throwIfFailed(resp)
  const reader = resp.body!.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let nl: number
    while ((nl = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, nl).trim()
      buf = buf.slice(nl + 1)
      if (!line) continue
      const ev = JSON.parse(line)
      if (ev.error) throw new Error(ev.error)
      onEvent(ev)
    }
  }
}

export const api = {
  get: <T = any>(path: string) => request<T>('GET', path),
  post: <T = any>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T = any>(path: string, body?: unknown) => request<T>('PUT', path, body),
  del: <T = any>(path: string) => request<T>('DELETE', path),
}
