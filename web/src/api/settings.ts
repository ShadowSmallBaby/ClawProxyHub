// 系统设置 API（网关 / 网络 / 日志保留 / 站点品牌）+ 系统信息。
import { api, upload, downloadFile } from './client'

export interface AdminSettings {
  first_token_timeout: number
  first_event_timeout?: number
  max_retries?: number
  user_agent?: string
  browser_user_agent?: string
  github_proxy?: string
  log_retention_days?: number
  run_level?: string
  task_daily_jitter?: number
  timezone?: string
  context_truncate_enabled?: boolean
  context_truncate_ratio?: number
  context_bytes_per_token?: number
  plugin_lua_enabled?: boolean
  plugin_lua_isolation?: boolean
  site_name?: string
  site_abbr?: string
  site_logo?: string
}

export interface SysInfo {
  version: string; protocol_version: number; go_version: string; os: string; arch: string
  started_at: string; uptime_seconds: number
  data_dir: string; db_size_bytes: number; migration_version: number
  mem_alloc_bytes: number; goroutines: number; counts: Record<string, number>; pending_restore: boolean
}

export const settingsApi = {
  get: () => api.get<{ settings: AdminSettings }>('/admin/settings'),
  save: (patch: Record<string, unknown>) => api.put('/admin/settings', patch),
}

export const systemApi = {
  info: () => api.get<SysInfo>('/admin/system/info'),
  backup: (flag: { value: boolean }) => downloadFile('/admin/system/backup', 'cph-backup.zip', flag),
  restore: async (file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    await upload('/admin/system/restore', fd)
  },
}
