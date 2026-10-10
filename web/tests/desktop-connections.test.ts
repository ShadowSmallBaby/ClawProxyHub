import { test } from 'node:test'
import assert from 'node:assert/strict'

// 升级前保存的远程连接不能把桌面站点的请求和凭据带到别的后端。
const values = new Map<string, string>([
  ['cph-connections', JSON.stringify([{ id: 'remote', name: 'Old connection', baseURL: 'https://old.example' }])],
  ['cph-active-connection', 'remote'],
  ['cph-token:remote', 'remote-token'],
  ['cph-token:same-origin', 'desktop-token'],
])
Object.defineProperty(globalThis, '__CPH_PROFILE__', { value: 'web-full' })
Object.defineProperty(globalThis, 'localStorage', { value: {
  getItem: (key: string) => values.get(key) ?? null,
  setItem: (key: string, value: string) => values.set(key, value),
  removeItem: (key: string) => values.delete(key),
} })
const connections = await import('../src/api/connections.ts')
const { api } = await import('../src/api/client.ts')

test('desktop stays on its own service despite saved remote selection', async () => {
  assert.equal(connections.currentConnection().id, 'same-origin')
  assert.throws(() => connections.applyWorkspace({ id: 'remote', name: 'Remote', baseURL: 'https://remote.example' }, 'remote-token'))
  globalThis.fetch = async (url, init) => {
    assert.equal(url, '/admin/me')
    assert.equal(new Headers(init?.headers).get('Authorization'), 'Bearer desktop-token')
    return Response.json({ username: 'admin' })
  }
  assert.deepEqual(await api.get('/admin/me'), { username: 'admin' })
})
