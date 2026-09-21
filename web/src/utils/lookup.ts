// 关联字段展示辅助：插件品牌名 / 实例名 / 实例下拉项（Accounts/Groups/Instances/Logs/Tasks 复用）。
import type { GroupInfo, InstanceInfo } from '../api/types'
import type { PluginInfo } from '../api/types'

// 最小插件结构（列表页只有 {id,label,name} 也够）
interface PluginLite { id: number; name: string; label?: string }
import type { Proxy } from '../api/entities'

// 插件品牌名映射：关联字段统一显示品牌而非 id；无插件（0/null）显示 -
export function pluginLabelOf(plugins: (PluginInfo | PluginLite)[], id: number | null | undefined): string {
  if (!id) return '-'
  const p = plugins.find((x) => x.id === id)
  return p?.label || p?.name || `#${id}`
}

// 实例名映射
export function instanceNameOf(instances: InstanceInfo[], id: number): string {
  return instances.find((i) => i.id === id)?.name ?? (id ? `#${id}` : '-')
}

// 实例下拉：按插件过滤，展示名称（+ 地址）
export function instanceOptionsOf(instances: InstanceInfo[], pluginID: number | undefined) {
  return instances
    .filter((i) => i.plugin_id === pluginID)
    .map((i) => ({ value: i.id, label: i.base_url ? `${i.name} · ${i.base_url}` : i.name }))
}

// 分组下拉：名称 + 品牌后缀
export function groupOptionsOf(groups: GroupInfo[]) {
  return groups.map((g) => ({ value: g.id, label: `${g.name} (${g.plugin_label || g.plugin})` }))
}

// 代理下拉：scheme://host:port
export function proxyOptionsOf(proxies: Proxy[]) {
  return proxies.map((px) => ({ value: px.ID, label: `${px.Scheme}://${px.Host}:${px.Port}` }))
}
