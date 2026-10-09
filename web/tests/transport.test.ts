import { test } from 'node:test'
import assert from 'node:assert/strict'
Object.defineProperty(globalThis, '__CPH_PROFILE__', { value: 'app-full' })

const values = new Map<string, string>([['cph-admin-token', 'legacy-local']])
Object.defineProperty(globalThis, 'localStorage', { value: {
  getItem: (key: string) => values.get(key) ?? null,
  setItem: (key: string, value: string) => values.set(key, value),
  removeItem: (key: string) => values.delete(key),
} })
const c = await import('../src/api/connections.ts')
const { api, upload, downloadFile, requestStream, resourceURL, HTTPError } = await import('../src/api/client.ts')
const a = { id: 'a', name: 'A', baseURL: 'https://a.example/cph' }
const b = { id: 'b', name: 'B', baseURL: 'https://b.example' }
c.setTokenWriter(async () => {})
function select(id: string) { c.applyWorkspace(id === 'a' ? a : b, id === 'a' ? 'token-a' : 'token-b') }
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}
const cancelled = { name: 'AbortError' }

test('app credentials come only from native context and never use stored Web tokens', () => {
  assert.equal(c.getToken(), '')
  select(a.id)
  assert.equal(c.getToken(), 'token-a')
  select(b.id)
  assert.equal(c.getToken(), 'token-b')
  assert.equal(values.size, 1)
})

test('remote URLs require HTTPS, support loopback and preserve path prefixes', () => {
  for (const url of ['http://remote.example', 'https://user:pass@example.com', 'https://a.example?secret=1', 'file:///tmp/a']) assert.throws(() => c.normalizeBackendURL(url))
  assert.equal(c.normalizeBackendURL('http://127.0.0.1:8080/'), 'http://127.0.0.1:8080')
  select(a.id)
  assert.equal(resourceURL('/assets/plugins/newapi/icon'), 'https://a.example/cph/assets/plugins/newapi/icon')
  assert.equal(resourceURL('data:image/png;base64,AA'), 'data:image/png;base64,AA')
  assert.equal(resourceURL('javascript:alert(1)'), '')
})

test('switch aborts old fetch; even a late 401 cannot clear new credentials', async () => {
  select(a.id)
  const response = deferred<Response>()
  let signal: AbortSignal | undefined
  globalThis.fetch = async (_url, init) => { signal = init?.signal as AbortSignal; return response.promise }
  const request = api.get('/admin/me')
  select(b.id)
  assert.equal(signal?.aborted, true)
  response.resolve(new Response('unauthorized', { status: 401 }))
  await assert.rejects(request, cancelled)
  assert.equal(c.getToken(), 'token-b')
})

test('a body completed after switching is never returned to the caller', async () => {
  select(a.id)
  const body = deferred<unknown>()
  const reading = deferred<void>()
  globalThis.fetch = async () => ({ ok: true, json: () => { reading.resolve(); return body.promise } }) as Response
  const request = api.get('/admin/accounts')
  await reading.promise
  select(b.id)
  body.resolve({ accounts: ['a'] })
  await assert.rejects(request, cancelled)
})

test('uploads and JSON requests share URL/auth handling without multipart content-type override', async () => {
  select(a.id)
  globalThis.fetch = async (url, init) => {
    assert.equal(url, 'https://a.example/cph/admin/plugins/local')
    assert.equal(new Headers(init?.headers).get('Authorization'), 'Bearer token-a')
    assert.equal(new Headers(init?.headers).has('Content-Type'), false)
    assert.equal(init?.credentials, 'omit')
    assert.equal(init?.redirect, 'error')
    return Response.json({ created: 'test' })
  }
  assert.deepEqual(await upload('/admin/plugins/local', new FormData()), { created: 'test' })
  await assert.rejects(api.get('https://elsewhere.example/admin/me'), /Invalid backend path/)
  globalThis.fetch = async () => new Response('no', { status: 401 })
  await assert.rejects(upload('/admin/plugins/local', new FormData()), (error: unknown) => error instanceof HTTPError && error.status === 401)
  assert.equal(c.getToken(), '')
  await c.setToken('token-a')
})

test('stream stops between buffered events if the event handler switches connection', async () => {
  select(a.id)
  globalThis.fetch = async () => new Response('{"phase":"first"}\n{"phase":"second"}\n')
  const events: string[] = []
  await assert.rejects(requestStream('POST', '/admin/plugins/install-market', {}, event => {
    events.push(event.phase)
    select(b.id)
  }), cancelled)
  assert.deepEqual(events, ['first'])
})

test('an aborted caller receives no stream events', async () => {
  const controller = new AbortController()
  controller.abort()
  await assert.rejects(requestStream('POST', '/admin/plugins/install-market', {}, () => assert.fail('unexpected event'), controller.signal), cancelled)
})

test('download never creates a file if its body belongs to an old connection', async () => {
  select(a.id)
  const body = deferred<Blob>()
  const reading = deferred<void>()
  globalThis.fetch = async () => ({ ok: true, blob: () => { reading.resolve(); return body.promise } }) as Response
  const flag = { value: false }
  const request = downloadFile('/admin/system/backup', 'backup.zip', flag)
  await reading.promise
  select(b.id)
  body.resolve(new Blob(['private']))
  await assert.rejects(request, cancelled)
  assert.equal(flag.value, false)
})
