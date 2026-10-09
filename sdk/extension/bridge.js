// 框架无关的浏览器桥；凭据、请求传输和持久存储由宿主掌管。
let nonce = '', serial = 0, context = {}
const pending = new Map(), events = new Set(), contexts = new Set()
let resolveReady
export const ready = new Promise(resolve => { resolveReady = resolve })

window.addEventListener('message', event => {
  if (event.source !== window.parent) return
  const data = event.data
  if (!data || typeof data !== 'object') return
  if (data.type === 'cph:init' && typeof data.nonce === 'string') {
    if (nonce && nonce !== data.nonce) {
      for (const call of pending.values()) call.reject(new Error('Host session replaced'))
      pending.clear()
    }
    nonce = data.nonce; context = data.context || {}
    for (const listener of contexts) listener(context)
    resolveReady(context)
    return
  }
  if (!nonce || data.nonce !== nonce) return
  if (data.type === 'cph:context') {
    context = { ...context, ...data.context }
    for (const listener of contexts) listener(context)
  } else if (data.type === 'cph:ui-event') {
    for (const listener of events) listener(data.event)
  } else if (data.type === 'cph:result') {
    const call = pending.get(data.id)
    if (!call) return
    if (data.error) call.reject(new Error(data.error)); else call.resolve(data.result)
  }
})

function request(message, timeout, signal) {
  if (!nonce || pending.size >= 8) return Promise.reject(new Error('Host bridge unavailable'))
  const id = ++serial
  return new Promise((resolve, reject) => {
    const cleanup = () => { clearTimeout(timer); pending.delete(id); signal?.removeEventListener('abort', cancel) }
    const cancel = () => {
      cleanup(); window.parent.postMessage({ type: 'cph:cancel', nonce, id }, '*')
      reject(new Error('Action cancelled'))
    }
    const timer = setTimeout(cancel, timeout)
    pending.set(id, { resolve: value => { cleanup(); resolve(value) }, reject: error => { cleanup(); reject(error) } })
    if (signal?.aborted) { cancel(); return }
    signal?.addEventListener('abort', cancel, { once: true })
    window.parent.postMessage({ ...message, nonce, id }, '*')
  })
}
export function invoke(action, input, signal) { return request({ type: 'cph:invoke', action, input }, 300000, signal) }
export function ui(command) { return request({ type: 'cph:ui', command }, 10000) }
export function activate() { if (nonce) window.parent.postMessage({ type: 'cph:ready', nonce }, '*') }
export function onUIEvent(listener) { events.add(listener); return () => events.delete(listener) }
export function onContext(listener) { contexts.add(listener); if (nonce) listener(context); return () => contexts.delete(listener) }
export function draft(content, name, dirty) { if (nonce) window.parent.postMessage({ type: 'cph:draft', nonce, content, name, dirty }, '*') }
