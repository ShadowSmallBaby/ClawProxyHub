import type { ExtensionState, PackageEntry } from '../api/extensions'

export interface ExtensionCard {
  id: string
  name: string
  label?: Record<string, string>
  desc?: Record<string, string>
  version: string
  publisher: string
  state?: ExtensionState
  bundled?: PackageEntry
}

function compareVersion(a: string, b: string) {
  const left = a.split('.').map(Number), right = b.split('.').map(Number)
  for (let i = 0; i < 3; i++) {
    const difference = (left[i] || 0) - (right[i] || 0)
    if (difference) return difference
  }
  return 0
}

// 同一扩展只显示一张卡片，安装状态与内置包分别保留，卸载后仍可从原包安装。
export function extensionCards(states: ExtensionState[], packages: PackageEntry[]): ExtensionCard[] {
  const cards = new Map<string, ExtensionCard>()
  for (const state of states) {
    cards.set(state.manifest.id, {
      id: state.manifest.id, name: state.manifest.name, version: state.manifest.version,
      label: state.manifest.label, desc: state.manifest.desc,
      publisher: state.publisher, state,
    })
  }
  const usable = (item: PackageEntry) => !['invalid', 'incompatible', 'conflict'].includes(item.status)
  for (const item of packages) {
    const id = item.manifest?.id || `package:${item.path}`
    let card = cards.get(id)
    if (!card) {
      card = { id, name: item.manifest?.name || item.path.split(/[\\/]/).pop() || item.path,
        version: item.manifest?.version || '', publisher: item.publisher || '' }
      cards.set(id, card)
    }
    const previous = card.bundled
    if (!previous || (usable(item) && !usable(previous)) ||
        (usable(item) === usable(previous) && compareVersion(item.manifest?.version || '', previous.manifest?.version || '') > 0)) {
      card.bundled = item
      if (!card.state) {
        card.name = item.manifest?.name || card.name
        card.label = item.manifest?.label
        card.desc = item.manifest?.desc
        card.version = item.manifest?.version || ''
        card.publisher = item.publisher || ''
      }
    }
  }
  return [...cards.values()].sort((a, b) => Number(!!b.bundled) - Number(!!a.bundled) || a.id.localeCompare(b.id))
}
