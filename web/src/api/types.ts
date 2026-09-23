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
  running: boolean // 已停止的插件仍列出（可启动/卸载），能力与授权方式为空
  capabilities: string[]
  auth_methods: AuthMethod[] | null
  instance_schema?: string // 实例级设置 JSON Schema（空 = 实例只有名称 + 地址）
  protocol_version?: number
  multi_instance?: boolean // 声明 instances 能力且契约 ≥2；否则只有默认实例
}

// 实例：插件下的一个站点/部署（Plugin → Instance → Account）
export interface InstanceInfo {
  id: number
  plugin_id: number
  name: string
  base_url: string
  settings: Record<string, unknown>
  account_count: number
}

// 删除影响面：级联移除的数量 + 引用了被删分组、需人工复核的路由/密钥名
export interface DeleteImpact {
  instances: number
  groups: number
  accounts: number
  task_rules: number
  task_runs: number
  routes: string[]
  keys: string[]
}

// 插件源（index.json 地址）
export interface PluginSource {
  name: string
  url: string
  enabled: boolean
  // 以下由列表接口实时计算
  plugin_count?: number
  installed_count?: number
  reachable?: boolean
}

export interface Account {
  id: number
  plugin_id: number
  instance_id: number
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
  instance_id: number
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
  instance_id: number
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
  UserAgent: string
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
  target_scope: string
  target_json: string // account_ids 原始范围（编辑回填用）
  auto: boolean // true = 系统自动生成（编辑锁定触发类型/能力/范围）
  instance: string // account_ids 范围下账号所属实例名；空 = 全部实例
  accounts: string[]
  enabled: boolean
  next_run_at: string | null
  last_run_at: string | null
}

// 任务执行历史（语义视图：不暴露业务 id）
export interface TaskRun {
  id: number
  plugin: string
  capability: string
  instance: string // 账号所属实例名（账号已删为空）
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
  PluginID: number | null
  Model: string
  RouteName: string
  Protocol: string
  Status: number
  InputTokens: number
  OutputTokens: number
  CachedTokens: number // 缓存读取
  CacheCreationTokens: number // 缓存写入
  FirstTokenMs: number
  LatencyMs: number
  ClientIP: string
  UserAgent: string
  ErrorBrief: string
  Stream: boolean // 同步/流式
  CreatedAt: string
  key_name?: string // 密钥名称（列表接口附带）
  instance_name?: string // 账号所属实例（列表接口附带；空 = 账号已删/无账号）
}

// 运行日志（系统级：账号刷新/登录失败、插件日志、核心内部事件）
export interface RunLog {
  ID: number
  Level: string // error / warn / debug / info
  Module: string // 功能模块（account / plugin / task …）
  Action: string // 操作（refresh / login / 插件名 …）
  Message: string // 精简消息
  Detail: string // 调试明细（上游响应体 / err 全文）
  AccountID: number | null
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
