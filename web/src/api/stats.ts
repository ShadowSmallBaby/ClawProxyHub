// 统计 API（概览卡片 / 趋势 / 按插件积分）。
import { api } from './client'
import type { RequestLog, Stats } from './types'

export interface TrendPoint { date: string; requests: number; success: number; tokens: number }
export interface QuotaPlugin {
  plugin: string
  label?: string
  instance?: string
  accounts: number
  quota: Record<string, number>
}

export const statsApi = {
  get: () => api.get<Stats>('/admin/stats'),
  trend: (days = 7) => api.get<{ trend: TrendPoint[] }>(`/admin/stats/trend?days=${days}`),
  recentLogs: (limit = 100) => api.get<{ logs: RequestLog[] }>(`/admin/logs?limit=${limit}`),
  quota: () => api.get<{ plugins: QuotaPlugin[] }>('/admin/stats/quota'),
}
