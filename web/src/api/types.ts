// 与后端 admin API 对齐的类型。

export interface AuthField {
  name: string
  label: Record<string, string>
  type: string // text / password / textarea / file / phone
  required: boolean
  placeholder: string
}

export interface AuthMethod {
  id: string
  label: Record<string, string>
  fields: AuthField[] | null
  capabilities: string[]
  callback?: string // auto / wait / auto_wait（浏览器授权回调形态）
}

export interface NextStep {
  action: 'open_url' | 'input_form'
  url?: string
  prompt?: Record<string, string>
  fields?: AuthField[] | null
  state?: string
  wait?: boolean
}

export interface LoginResp {
  done: boolean
  account_id?: number
  next?: NextStep
}

export interface PluginInfo {
  id: number
  name: string
  label: string // 品牌名（关联字段统一显示）
  version: string
  author: string
  icon: string // 包内相对路径（空 = 前端兜底首字母）
  capabilities: string[]
  auth_methods: AuthMethod[] | null
}

export interface Account {
  id: number
  plugin_id: number
  group_ids: number[] | null
  display_name: string
  status: string
  pause_reason: string
  paused_until: string | null
  last_refresh_at: string | null
  credits?: { remaining?: string; total?: string; free_limit?: string; free_used?: string } | null
}

export interface AccountRun {
  id: number
  plugin: string
  capability: string
  account: string
  status: string
  summary: string
  detail?: { items?: unknown[] } | null
  error_message: string
  started_at: string
  finished_at: string | null
}

export interface AccountDetail {
  id: number
  plugin_id: number
  group_ids: number[]
  display_name: string
  status: string
  pause_reason: string
  paused_until: string | null
  manual_pause: boolean
  last_refresh_at: string | null
  last_used_at: string | null
  created_at: string
  profile: {
    displayName?: string
    quota?: Record<string, string>
    // 动态渲染块（插件声明的 ProfileSection，核心随 profile_json 持久化）
    sections?: {
      id: string
      title: Record<string, string>
      entries?: { label: Record<string, string>; value: string; kind?: string }[]
      columns?: { key: string; title: Record<string, string>; kind?: string }[]
      items?: { cells: Record<string, string> }[]
    }[]
    [key: string]: unknown
  }
  // 积分明细快照（插件解析上游后持久化；读取只走库不请求上游）
  credits: {
    total?: string
    used?: string
    remaining?: string
    packages?: { total?: string; used: string; remaining?: string; expiresAt?: string }[]
    [key: string]: unknown
  } | null
  models?: ModelInfo[] | null
  runs: AccountRun[]
}

export interface ModelInfo {
  id: string
  label?: Record<string, string>
  contextWindow?: number
  supportsTools?: boolean
  supportsStream?: boolean
}

// 账号积分包（credits.packages 元素）
export interface CreditPackage {
  total?: string
  used: string
  remaining?: string
  expiresAt?: string
}

export interface KeyInfo {
  id: number
  name: string
  enabled: boolean
  expires_at: string | null
  created_at: string
  last_used_at: string // 最后调用（空 = 从未）
  key_mask: string // 掩码（cph-****abcd）
  route_ids: number[] | null
}

export interface GroupInfo {
  id: number
  name: string
  plugin_id: number
  plugin: string
  plugin_label: string
  accounts: number
}

export interface RouteGroupEntry {
  group_id: number | null | undefined
  weight: number
  model: string
}

export interface RouteInfo {
  ID: number
  Name: string
  Strategy: string
  GroupsJSON: string
  TimeoutSeconds: number
  FailoverEnabled: boolean
  FailoverOn4xx: boolean
  FailoverOn5xx: boolean
  FailoverGroupID: number | null
  FailoverModel: string
}

export interface TaskRule {
  id: number
  plugin_id: number
  plugin: string
  capability_id: string
  capability: string // 展示名（插件声明的 label）
  trigger_type: string
  trigger_value: string
  enabled: boolean
  next_run_at: string | null
  last_run_at: string | null
}

// 任务执行历史（语义视图：不暴露业务 id）
export interface TaskRun {
  id: number
  plugin: string
  capability: string
  account: string
  status: string
  summary: string
  detail?: { items?: unknown[] } | null
  error_message: string
  started_at: string
  finished_at: string | null
}

export interface RequestLog {
  ID: number
  KeyID: number | null
  Model: string
  Protocol: string
  Status: number
  InputTokens: number
  OutputTokens: number
  CachedTokens: number
  FirstTokenMs: number
  LatencyMs: number
  ClientIP: string
  UserAgent: string
  ErrorBrief: string
  CreatedAt: string
}

export interface Stats {
  total_requests: number
  today_requests: number
  success_rate: number
  total_tokens: number
  active_keys: number
  active_accounts: number
  running_plugins: number
}
