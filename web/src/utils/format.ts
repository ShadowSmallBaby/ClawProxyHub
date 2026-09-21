// 通用格式化：相对时间 / 绝对时间 / 数字缩写 / 字节数。
import i18n from '../i18n'

const t = (k: string, v?: Record<string, unknown>) => i18n.global.t(k, v ?? {})

// 相对时间：如 5分钟前 / 1天前 / 3个月前
export function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts.replace(' ', 'T')).getTime()
  const min = Math.floor(diff / 60000)
  if (min < 1) return t('common.justNow')
  if (min < 60) return t('common.minutesAgo', { n: min })
  const h = Math.floor(min / 60)
  if (h < 24) return t('common.hoursAgo', { n: h })
  const d = Math.floor(h / 24)
  if (d < 30) return t('common.daysAgo', { n: d })
  const mo = Math.floor(d / 30)
  if (mo < 12) return t('common.monthsAgo', { n: mo })
  return t('common.yearsAgo', { n: Math.floor(mo / 12) })
}

// "2026-10-01T12:00:00" → "2026-10-01 12:00:00"（空值显示 -）
export function fmtTime(ts?: string | null): string {
  return ts ? ts.replace('T', ' ').slice(0, 19) : '-'
}

// 数字缩写：1.2M / 3.4K
export function fmtCompact(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}

// 千分位；未采集显示 -
export function fmtNum(v: number | string | undefined | null): string {
  if (v === undefined || v === null || v === '') return '-'
  const n = Number(v)
  return Number.isFinite(n) ? n.toLocaleString('zh-CN', { maximumFractionDigits: 2 }) : String(v)
}

// 字节数：1.2 GB / 3.4 MB
export function fmtBytes(n: number): string {
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(2) + ' GB'
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(1) + ' KB'
  return n + ' B'
}
