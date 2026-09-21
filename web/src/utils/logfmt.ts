// 日志/最近请求共用的格式化。
import type { RequestLog } from '../api/types'

// 大数缩写：如 35.9K / 110.2K
export function fmtTokens(n: number): string {
  if (!n) return '0'
  if (n < 1000) return String(n)
  if (n < 1000000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}K`
  return `${(n / 1000000).toFixed(2)}M`
}

// 毫秒展示：<1s 展示 ms，否则秒（如 3.50s）
export function fmtMs(ms: number): string {
  if (!ms) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

// 总 token：信封为 Anthropic 语义，输入 / 缓存读 / 缓存写互不包含，直接相加
export function sumTokens(row: RequestLog): number {
  return (row.InputTokens || 0) + (row.OutputTokens || 0) + (row.CachedTokens || 0) + (row.CacheCreationTokens || 0)
}

// 模型列：路由名与真实模型一致（或无路由）只显模型，否则 "路由 -> 模型"
export function modelLabel(row: Pick<RequestLog, 'Model' | 'RouteName'>): string {
  if (row.RouteName && row.RouteName !== row.Model) return `${row.RouteName} -> ${row.Model}`
  return row.Model || row.RouteName || '-'
}
