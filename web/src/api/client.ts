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

export async function request<T = any>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${getToken()}`,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
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
  return resp.json()
}

export const api = {
  get: <T = any>(path: string) => request<T>('GET', path),
  post: <T = any>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T = any>(path: string, body?: unknown) => request<T>('PUT', path, body),
  del: <T = any>(path: string) => request<T>('DELETE', path),
}
