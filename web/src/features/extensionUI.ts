// 所有来自沙箱的 UI 描述先校验；只接受协议中的控件和有限大小的纯数据。
import type { LocalizedText, UICapabilities, UICommand } from '../../../sdk/extension/ui.ts'

export const extensionUICapabilities: UICapabilities = Object.freeze({
  version: 1,
  surfaces: ['page', 'drawer', 'notice'] as const,
  fields: ['text', 'textarea', 'select', 'toggle', 'image'] as const,
  locations: ['plugins.toolbar', 'plugins.item.actions'] as const,
  image_max_bytes: 512 * 1024,
})

type ObjectValue = Record<string, unknown>
function requireValue(condition: unknown): asserts condition {
  if (!condition) throw new Error('Invalid extension UI descriptor')
}
function object(value: unknown, keys: string[]): ObjectValue {
  requireValue(value && typeof value === 'object' && !Array.isArray(value))
  requireValue(Object.keys(value).every(key => keys.includes(key)))
  return value as ObjectValue
}
function string(value: unknown, limit: number, empty = false): asserts value is string {
  requireValue(typeof value === 'string' && value.length <= limit && (empty || value.trim().length > 0))
}
function identifier(value: unknown) {
  string(value, 96)
  requireValue(/^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/.test(value) && !['constructor', 'prototype', '__proto__'].includes(value))
}
function localized(value: unknown, limit: number) {
  if (typeof value === 'string') { string(value, limit); return }
  requireValue(value && typeof value === 'object' && !Array.isArray(value))
  const entries = Object.entries(value)
  requireValue(entries.length > 0 && entries.length <= 8)
  for (const [key, text] of entries) {
    requireValue(/^[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$/.test(key))
    string(text, limit)
  }
}
function optional(value: unknown, check: (value: unknown) => void) { if (value !== undefined) check(value) }
function boolean(value: unknown) { requireValue(typeof value === 'boolean') }
function unique(values: unknown[], limit: number) {
  requireValue(values.length <= limit)
  const ids = values.map(value => {
    requireValue(value && typeof value === 'object')
    const id = (value as ObjectValue).id
    identifier(id)
    return id
  })
  requireValue(new Set(ids).size === ids.length)
}
const controlKeys = ['label', 'hint', 'intent', 'disabled', 'loading']
function control(value: unknown, action = false) {
  const entry = object(value, action ? ['id', ...controlKeys] : controlKeys)
  if (action) identifier(entry.id)
  localized(entry.label, 160)
  optional(entry.hint, value => localized(value, 1024))
  optional(entry.intent, value => requireValue(['primary', 'default', 'danger'].includes(value as string)))
  optional(entry.disabled, boolean)
  optional(entry.loading, boolean)
}
function field(value: unknown) {
  const entry = object(value, ['id', 'kind', 'label', 'hint', 'required', 'readonly', 'value', 'max_length', 'options'])
  identifier(entry.id)
  localized(entry.label, 160)
  optional(entry.hint, value => localized(value, 1024))
  optional(entry.required, boolean)
  optional(entry.readonly, boolean)
  switch (entry.kind) {
    case 'text':
    case 'textarea':
      optional(entry.max_length, value => requireValue(Number.isSafeInteger(value) && Number(value) > 0 && Number(value) <= 16384))
      optional(entry.value, value => string(value, Number(entry.max_length || 4096), true))
      requireValue(entry.options === undefined)
      break
    case 'select': {
      requireValue(Array.isArray(entry.options) && entry.options.length > 0 && entry.options.length <= 64 && entry.max_length === undefined)
      const choices = entry.options.map(option => {
        const choice = object(option, ['value', 'label'])
        string(choice.value, 160)
        localized(choice.label, 160)
        return choice.value
      })
      requireValue(new Set(choices).size === choices.length)
      optional(entry.value, value => requireValue(value === '' || choices.includes(value as string)))
      break
    }
    case 'toggle':
      optional(entry.value, boolean)
      requireValue(entry.options === undefined && entry.max_length === undefined)
      break
    case 'image':
      requireValue(entry.value === undefined && entry.options === undefined && entry.max_length === undefined)
      break
    default: throw new Error('Unsupported extension UI field')
  }
}

export function parseUICommand(value: unknown): UICommand {
  const encoded = JSON.stringify(value)
  requireValue(encoded && encoded.length <= 64 * 1024)
  requireValue(value && typeof value === 'object')
  switch ((value as ObjectValue).kind) {
    case 'page': {
      const entry = object(value, ['kind', 'page']), page = object(entry.page, ['title', 'subtitle', 'actions'])
      localized(page.title, 160)
      optional(page.subtitle, value => localized(value, 160))
      requireValue(Array.isArray(page.actions))
      unique(page.actions, 8)
      page.actions.forEach(action => control(action, true))
      break
    }
    case 'drawer': {
      const entry = object(value, ['kind', 'drawer'])
      if (entry.drawer === null) break
      const drawer = object(entry.drawer, ['id', 'title', 'fields', 'submit'])
      identifier(drawer.id)
      localized(drawer.title, 160)
      requireValue(Array.isArray(drawer.fields))
      unique(drawer.fields, 24)
      drawer.fields.forEach(field)
      control(drawer.submit)
      break
    }
    case 'notice': {
      const entry = object(value, ['kind', 'level', 'message'])
      requireValue(['success', 'warning', 'error', 'info'].includes(entry.level as string))
      localized(entry.message, 2048)
      break
    }
    default: throw new Error('Unsupported extension UI command')
  }
  return value as UICommand
}

export function uiText(value: LocalizedText | undefined, locale: string): string {
  if (!value) return ''
  return typeof value === 'string' ? value : value[locale] || value[locale.split('-')[0]!] || value.zh || value.en || Object.values(value)[0] || ''
}
