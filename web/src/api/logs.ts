// 日志 API（请求日志列表 / 导出 / 清空）。导出走 blob 下载（需鉴权头）。
import { api, getToken } from './client'
import type { RequestLog } from './types'

export interface LogFilters {
  key?: string
  model?: string
  route?: string
  plugin_id?: number
  protocol?: string
  status_class?: string
  from?: string
  to?: string
}

export const logsApi = {
  list: (page: number, pageSize: number, f: LogFilters) => {
    const p = new URLSearchParams()
    p.set('page', String(page))
    p.set('page_size', String(pageSize))
    if (f.key) p.set('key', f.key)
    if (f.model) p.set('model', f.model)
    if (f.route) p.set('route', f.route)
    if (f.plugin_id) p.set('plugin_id', String(f.plugin_id))
    if (f.protocol) p.set('protocol', f.protocol)
    if (f.status_class) p.set('status_class', f.status_class)
    if (f.from) p.set('from', f.from)
    if (f.to) p.set('to', f.to)
    return api.get<{ logs: RequestLog[]; total: number }>(`/admin/logs?${p.toString()}`)
  },
  // 带鉴权头下载 CSV（<a download> 带不了 Authorization），blob 落成文件
  exportCsv: async (flag: { value: boolean }) => {
    flag.value = true
    try {
      const resp = await fetch('/admin/logs/export', { headers: { Authorization: `Bearer ${getToken()}` } })
      if (!resp.ok) throw new Error((await resp.text()) || `HTTP ${resp.status}`)
      const name = /filename="?([^"]+)"?/.exec(resp.headers.get('Content-Disposition') ?? '')?.[1] ?? 'cph-logs.csv'
      const url = URL.createObjectURL(await resp.blob())
      const a = Object.assign(document.createElement('a'), { href: url, download: name })
      a.click()
      URL.revokeObjectURL(url)
    } finally {
      flag.value = false
    }
  },
  clear: () => api.del<{ deleted: number }>('/admin/logs'),
}
