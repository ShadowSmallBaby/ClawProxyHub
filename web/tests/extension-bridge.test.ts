import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

test('shared bridge revokes old calls on reinitialization and accepts the new session', async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const listeners: ((event: any) => void)[] = []
  const sent: any[] = []
  const parent = { postMessage: (message: any) => sent.push(message) }
  Object.defineProperty(globalThis, 'window', { configurable: true, value: {
    parent, addEventListener: (_: string, listener: (event: any) => void) => listeners.push(listener),
  } })
  const receive = (data: unknown) => listeners.forEach(listener => listener({ source: parent, data }))
  try {
    const source = await readFile(new URL('../../sdk/extension/bridge.js', import.meta.url), 'utf8')
    const bridge = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'))
    receive({ type: 'cph:init', nonce: 'first', context: { locale: 'zh', dark: true } })
    await bridge.ready
    const old = bridge.invoke('read', {})
    const rejected = assert.rejects(old, /Host session replaced/)
    receive({ type: 'cph:init', nonce: 'second', context: { locale: 'en', dark: false } })
    await rejected
    const next = bridge.invoke('read', {})
    const request = sent.at(-1)
    assert.equal(request.nonce, 'second')
    receive({ type: 'cph:result', nonce: 'first', id: request.id, result: 'stale' })
    receive({ type: 'cph:result', nonce: 'second', id: request.id, result: 'current' })
    assert.equal(await next, 'current')
    const controller = new AbortController()
    const cancelled = bridge.invoke('analyze', {}, controller.signal)
    controller.abort()
    await assert.rejects(cancelled, /Action cancelled/)
    assert.equal(sent.at(-1).type, 'cph:cancel')
  } finally {
    if (original) Object.defineProperty(globalThis, 'window', original)
    else Reflect.deleteProperty(globalThis, 'window')
  }
})
