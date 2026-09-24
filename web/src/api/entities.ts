// 实体资源 API（插件 / 实例 / 账号 / 分组 / 代理 / 路由 / 密钥 / OAuth 凭据 / 任务）。
import { api, getToken, requestStream } from './client'
import type {
  Account, AccountDetail, AuthMethod, DeleteImpact, GroupInfo, InstanceInfo, KeyInfo,
  LoginResp, ModelInfo, NextStep, PluginInfo, PluginSource, RequestLog,
  RouteInfo, TaskRule, TaskRun,
} from './types'

// ---------- 插件 ----------

export interface MarketEntry {
  name: string; version: string; author?: string; icon?: string; label?: Record<string, string>
  published_at?: string; source?: string; installed?: boolean; updatable?: boolean; runtime?: string
}
export interface SettingField { key: string; title: string; description: string; type: string; default: unknown; options: unknown[] }
// 市场安装进度事件：downloading（带字节数，total 未知为 -1）→ [stopping，仅升级] → installing → starting
export interface InstallProgress { phase: 'downloading' | 'stopping' | 'installing' | 'starting'; received?: number; total?: number }

export const pluginApi = {
  list: () => api.get<{ plugins: PluginInfo[] }>('/admin/plugins'),
  authMethods: (name: string) => api.get<{ auth_methods: AuthMethod[] }>(`/admin/plugins/${name}/auth-methods`),
  taskCapabilities: (name: string) => api.get<{ capabilities: { id: string; label: string }[] }>(`/admin/plugins/${name}/task-capabilities`),
  settings: (name: string) => api.get<{ schema: Record<string, any>; values: Record<string, any> }>(`/admin/plugins/${name}/settings`),
  saveSettings: (name: string, values: Record<string, unknown>) => api.put(`/admin/plugins/${name}/settings`, { values }),
  stop: (name: string) => api.post(`/admin/plugins/${name}/stop`),
  start: (name: string) => api.post(`/admin/plugins/${name}/start`),
  uninstall: (name: string) => api.del<{ impact?: DeleteImpact }>(`/admin/plugins/${name}`),
  impact: (name: string) => api.get(`/admin/plugins/${name}/impact`),
  marketplace: (source: string) =>
    api.get<{ plugins: MarketEntry[]; source?: string }>(`/admin/plugins/marketplace?source=${encodeURIComponent(source)}`),
  // 市场安装：NDJSON 进度流，每个阶段事件回调一次；出错抛 Error；signal 可中途取消下载
  installMarket: (name: string, author: string, source: string, onProgress: (p: InstallProgress) => void, signal?: AbortSignal) =>
    requestStream('POST', '/admin/plugins/install-market', { name, author, source }, (ev) => {
      if (ev.phase) onProgress(ev as InstallProgress)
    }, signal),
  // t-upload 自定义上传：multipart 直发安装端点
  uploadInstall: async (raw: File) => {
    const form = new FormData()
    form.append('package', raw)
    return fetch('/admin/plugins/install-upload', {
      method: 'POST',
      headers: { Authorization: `Bearer ${getToken()}` },
      body: form,
    })
  },
}

// ---------- 插件源 ----------

export const pluginSourceApi = {
  list: () => api.get<{ sources: PluginSource[] }>('/admin/plugin-sources'),
  save: (sources: { name: string; url: string; enabled: boolean }[]) =>
    api.put('/admin/plugin-sources', { sources }),
  probe: (url: string) => api.get(`/admin/plugin-sources/probe?url=${encodeURIComponent(url)}`),
}

// ---------- 实例 ----------

export const instanceApi = {
  list: (pluginId?: number) =>
    api.get<{ instances: InstanceInfo[] }>(`/admin/instances${pluginId ? `?plugin_id=${pluginId}` : ''}`),
  create: (body: { plugin_id: number; name: string; base_url: string; settings: Record<string, unknown> }) =>
    api.post('/admin/instances', body),
  update: (id: number, body: { plugin_id: number; name: string; base_url: string; settings: Record<string, unknown> }) =>
    api.put(`/admin/instances/${id}`, body),
  remove: (id: number) => api.del(`/admin/instances/${id}`),
  impact: (id: number) => api.get(`/admin/instances/${id}/impact`),
}

// ---------- 账号 ----------

export const accountApi = {
  list: () => api.get<{ accounts: Account[] }>('/admin/accounts'),
  detail: (id: number) => api.get<AccountDetail>(`/admin/accounts/${id}/detail`),
  login: (payload: { plugin: string; method_id: string; form: Record<string, string>; state: string; instance_id: number }) =>
    api.post<LoginResp>('/admin/accounts/login', payload),
  update: (id: number, body: Record<string, unknown>) => api.put(`/admin/accounts/${id}`, body),
  remove: (id: number) => api.del(`/admin/accounts/${id}`),
  impact: (id: number) => api.get(`/admin/accounts/${id}/impact`),
  proxies: (id: number) => api.get<{ proxy_ids: number[] }>(`/admin/accounts/${id}/proxies`),
  saveProxies: (id: number, proxy_ids: number[]) => api.put(`/admin/accounts/${id}/proxies`, { proxy_ids }),
  models: (id: number, refresh = false) =>
    api.get<{ models: ModelInfo[] | null }>(`/admin/accounts/${id}/models${refresh ? '?refresh=1' : ''}`),
  saveModels: (id: number, models: { id: string }[]) => api.put(`/admin/accounts/${id}/models`, { models }),
  pause: (id: number) => api.post(`/admin/accounts/${id}/pause`),
  resume: (id: number) => api.post(`/admin/accounts/${id}/resume`),
  refresh: (id: number) => api.post(`/admin/accounts/${id}/refresh`),
  test: (id: number, body: { endpoint: string; model: string; question: string }) =>
    api.post<{ text: string; logs: string[]; request?: string; events?: string[] }>(`/admin/accounts/${id}/test`, body),
}

// ---------- 分组 ----------

export const groupApi = {
  list: () => api.get<{ groups: GroupInfo[] }>('/admin/groups'),
  create: (body: { name: string; plugin_id: number; instance_id: number }) => api.post('/admin/groups', body),
  update: (id: number, body: { name: string; instance_id: number }) => api.put(`/admin/groups/${id}`, body),
  remove: (id: number) => api.del(`/admin/groups/${id}`),
  proxies: (id: number) => api.get<{ proxy_ids: number[] }>(`/admin/groups/${id}/proxies`),
  saveProxies: (id: number, proxy_ids: number[]) => api.put(`/admin/groups/${id}/proxies`, { proxy_ids }),
  models: (id: number) => api.get<{ models: string[] }>(`/admin/groups/${id}/models`),
}

// ---------- 代理 ----------

export interface Proxy {
  ID: number
  Name: string
  Scheme: string
  Host: string
  Port: number
  Username: string
}
export type ProxyBody = Pick<Proxy, 'Name' | 'Scheme' | 'Host' | 'Port' | 'Username'> & { password?: string }

export const proxyApi = {
  list: () => api.get<{ proxies: Proxy[] }>('/admin/proxies'),
  create: (body: ProxyBody) => api.post('/admin/proxies', body),
  update: (id: number, body: ProxyBody) => api.put(`/admin/proxies/${id}`, body),
  remove: (id: number) => api.del(`/admin/proxies/${id}`),
  test: (id: number) => api.post<{ ok: boolean; latency_ms?: number; error?: string }>(`/admin/proxies/${id}/test`),
}

// ---------- 路由 ----------

export const routeApi = {
  list: () => api.get<{ routes: RouteInfo[] }>('/admin/routes'),
  create: (body: Record<string, unknown>) => api.post('/admin/routes', body),
  update: (id: number, body: Record<string, unknown>) => api.put(`/admin/routes/${id}`, body),
  remove: (id: number) => api.del(`/admin/routes/${id}`),
}

// ---------- 密钥 ----------

export const keyApi = {
  list: () => api.get<{ keys: KeyInfo[] }>('/admin/keys'),
  create: (name: string) => api.post<{ key: string }>('/admin/keys', { name }),
  update: (id: number, body: { name: string }) => api.put(`/admin/keys/${id}`, body),
  remove: (id: number) => api.del(`/admin/keys/${id}`),
  reveal: (id: number) => api.get<{ key: string }>(`/admin/keys/${id}/reveal`),
  toggle: (id: number) => api.post(`/admin/keys/${id}/toggle`),
  routes: (id: number, route_ids: number[]) => api.put(`/admin/keys/${id}/routes`, { route_ids }),
}

// ---------- OAuth 凭据 ----------

export interface OAuthCred {
  id: number
  platform: string
  account_label: string
  has_token: boolean
  expires_at: string | null
  extra_json: string
  created_at: string
}
export type OAuthBody = Omit<OAuthCred, 'id' | 'has_token' | 'created_at'>

export const oauthApi = {
  list: () => api.get<{ credentials: OAuthCred[] }>('/admin/oauth-credentials'),
  create: (body: OAuthBody) => api.post('/admin/oauth-credentials', body),
  update: (id: number, body: OAuthBody) => api.put(`/admin/oauth-credentials/${id}`, body),
  remove: (id: number) => api.del(`/admin/oauth-credentials/${id}`),
}

// ---------- 任务 ----------

export const taskApi = {
  rules: (page: number, pageSize: number) =>
    api.get<{ rules: TaskRule[]; total: number }>(`/admin/task-rules?page=${page}&page_size=${pageSize}`),
  runs: (page: number, pageSize: number) =>
    api.get<{ runs: TaskRun[]; total: number }>(`/admin/task-runs?page=${page}&page_size=${pageSize}`),
  createRule: (body: Record<string, unknown>) => api.post('/admin/task-rules', body),
  updateRule: (id: number, body: Record<string, unknown>) => api.put(`/admin/task-rules/${id}`, body),
  removeRule: (id: number) => api.del(`/admin/task-rules/${id}`),
  toggleRule: (id: number) => api.post(`/admin/task-rules/${id}/toggle`),
  runRule: (id: number) => api.post(`/admin/task-rules/${id}/run`),
}
