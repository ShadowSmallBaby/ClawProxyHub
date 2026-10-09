// 沙箱只持消息会话，无 Token、HTTP 请求或原生宿主对象。
import { shallowRef } from 'vue'
import type { HostContext, UICommand, UIEvent } from '../../../sdk/extension/ui'

export const appearance = shallowRef({ locale: 'zh', dark: true })
const listeners = new Set<(event: UIEvent) => void>()
const pending = new Map<number, { resolve: (value: any) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> }>()
let nonce = '', serial = 0
let context: Partial<HostContext> = {}

function updateAppearance(value: Partial<HostContext> | undefined) {
  context = { ...context, ...value }
  if (typeof context.locale !== 'string' || typeof context.dark !== 'boolean') return
  const theme = context.settings?.theme ?? 'dark'
  appearance.value = { locale: context.locale, dark: theme === 'dark' || theme !== 'light' && context.dark }
}
export const ready = new Promise<HostContext>(resolve => {
  window.addEventListener('message', event => {
    if (event.source !== window.parent) return
    const data = event.data
    if (!data || typeof data !== 'object') return
    if (data.type === 'cph:init' && !nonce && typeof data.nonce === 'string') {
      nonce = data.nonce; updateAppearance(data.context); resolve(data.context); return
    }
    if (!nonce || data.nonce !== nonce) return
    if (data.type === 'cph:context') { updateAppearance(data.context); return }
    if (data.type === 'cph:ui-event') { for (const listener of listeners) listener(data.event); return }
    if (data.type !== 'cph:result') return
    const call = pending.get(data.id)
    if (!call) return
    clearTimeout(call.timer); pending.delete(data.id)
    if (data.error) call.reject(new Error(data.error)); else call.resolve(data.result)
  })
})

function request<T>(message: Record<string, unknown>, timeout: number, signal?: AbortSignal): Promise<T> {
  if (!nonce || pending.size >= 8) return Promise.reject(new Error('Host bridge unavailable'))
  const id = ++serial
  return new Promise((resolve, reject) => {
    const stop = () => {
      clearTimeout(timer); pending.delete(id); signal?.removeEventListener('abort', stop)
      window.parent.postMessage({ type: 'cph:cancel', nonce, id }, '*')
      reject(new Error('Action cancelled'))
    }
    const timer = setTimeout(stop, timeout)
    pending.set(id, {
      resolve: value => { signal?.removeEventListener('abort', stop); resolve(value) },
      reject: error => { signal?.removeEventListener('abort', stop); reject(error) }, timer,
    })
    if (signal?.aborted) { stop(); return }
    signal?.addEventListener('abort', stop, { once: true })
    window.parent.postMessage({ ...message, nonce, id }, '*')
  })
}
export function invoke<T = any>(action: string, input: unknown, signal?: AbortSignal): Promise<T> {
  return request<T>({ type: 'cph:invoke', action, input }, action.startsWith('ai-') ? 130000 : 35000, signal)
}
export function ui(command: UICommand): Promise<void> { return request({ type: 'cph:ui', command }, 10000) }
export function onUIEvent(listener: (event: UIEvent) => void) { listeners.add(listener); return () => { listeners.delete(listener) } }
export function activate() { if (nonce) window.parent.postMessage({ type: 'cph:ready', nonce }, '*') }
export function draft(content: string, name: string, dirty: boolean) {
  if (nonce) window.parent.postMessage({ type: 'cph:draft', nonce, content, name, dirty }, '*')
}
