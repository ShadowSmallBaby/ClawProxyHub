import { test, expect, type Page } from '@playwright/test'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'


// 使用真实跨域 HTTP 响应验证浏览器行为；不接触用户运行中的后端或凭据。
async function backend(profile = 'full', role = 'admin') {
  const requests: { path: string; token: string }[] = []
  const settings: Record<string, unknown> = {}
  let capabilityStatus = 200
  const extensions = { extensions: [] as unknown[], system_components: [] as unknown[], runtime_management: 'core' }
  const packages: unknown[] = []
  const identities: { code: string; config: Record<string, unknown>; read_only: boolean }[] = []
  const notifications = [] as { id: number; title: string; content: string; created_at: string; read: boolean; level: string }[]
  const mcpTools = [{ id: 'core.status', owner: 'core', title: 'Core status', permission: 'status.read', effect: 'read' }]
  const mcp = { enabled: false, has_key: false, allowed_actions: [] as string[], updated_at: '' }
  const server = createServer(async (req, res) => {
    const path = new URL(req.url!, 'http://localhost').pathname
    requests.push({ path, token: req.headers.authorization || '' })
    res.setHeader('Access-Control-Allow-Origin', req.headers.origin || '*')
    res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type')
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE')
    if (req.method === 'OPTIONS') { res.writeHead(204).end(); return }
    const json = (body: unknown, status = 200) => { res.writeHead(status, { 'Content-Type': 'application/json' }).end(JSON.stringify(body)) }
    if (path.startsWith('/assets/')) { res.writeHead(200, { 'Content-Type': 'image/svg+xml' }).end('<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="blue"/></svg>'); return }
    if (path === '/admin/branding') { json({ name: `Backend ${profile}`, abbr: 'CPH', logo: '/assets/plugins/demo/icon' }); return }
    if (path === '/admin/setup-status') { json({ initialized: true }); return }
    const token = `${profile}-${role}-token`
    if (path === '/admin/login') { json({ token, role }); return }
    if (req.headers.authorization !== `Bearer ${token}`) { json({ error: 'unauthorized' }, 401); return }
    if (path === '/admin/me') { json({ username: role, role, menus: role === 'admin' ? ['dashboard', 'plugins', 'extensions', 'instances', 'accounts', 'groups', 'proxies', 'routes', 'keys', 'oauth', 'tasks', 'logs', 'settings'] : ['dashboard', 'logs'] }); return }
    if (path === '/admin/capabilities') {
      json({ profile, modules: [{ id: 'common', installed: true, enabled: true, available: true, capabilities: ['admin', 'logs', 'settings', 'plugins', 'extensions', 'instances', 'accounts', 'groups', 'proxies', 'oauth', 'tasks', 'gateway', 'routes', 'keys'] }] }, capabilityStatus)
      return
    }
    if (path === '/admin/extensions/catalog') { json({ packages }); return }
    if (path === '/admin/extension-trust') { json({ identities }); return }
    if (path.startsWith('/admin/extension-trust/') && req.method === 'PUT') {
      const chunks = []
      for await (const chunk of req) chunks.push(chunk)
      const code = decodeURIComponent(path.split('/').pop()!)
      const config = JSON.parse(Buffer.concat(chunks).toString())
      const existing = identities.find(identity => identity.code === code)
      if (existing) existing.config = config
      else identities.push({ code, config, read_only: false })
      json({ ok: true }); return
    }
    if (path === '/admin/extensions/marketplace') { json({ packages: [], cached: false, checked_at: '', release_url: '' }); return }
    if (path === '/admin/mcp/config') {
      if (req.method === 'PUT') {
        const chunks = []
        for await (const chunk of req) chunks.push(chunk)
        Object.assign(mcp, JSON.parse(Buffer.concat(chunks).toString()))
        json({ ok: true }); return
      }
      json({ config: mcp, tools: mcpTools, endpoint: '/admin/mcp', execution: 'in-process' }); return
    }
    if (path === '/admin/mcp/key') {
      mcp.has_key = req.method === 'POST'
      json(mcp.has_key ? { key: 'cph_mcp_browser_fixture' } : { ok: true }); return
    }
    if (path === '/admin/settings') {
      if (req.method === 'PUT') {
        const chunks = []
        for await (const chunk of req) chunks.push(chunk)
        Object.assign(settings, JSON.parse(Buffer.concat(chunks).toString()))
      }
      json({ settings }); return
    }
    if (path === '/admin/accounts') { json({ accounts: [{ id: 1, plugin_id: 1, instance_id: 1, display_name: 'Demo', status: 'active', group_ids: [] }] }); return }
    if (path === '/admin/accounts/1/detail') { json({ models: [{ id: 'test-model' }], account: { id: 1 } }); return }
    if (path === '/admin/accounts/1/test') { json({ text: 'Test reply', logs: [] }); return }
    if (path === '/admin/plugins') { json({ plugins: [{ id: 1, name: 'demo', label: 'Demo', icon: '/assets/plugins/demo/icon', running: true }] }); return }
    if (path === '/admin/extensions') { json(extensions); return }
    if (path === '/admin/notifications') { json({ notifications, unread: notifications.filter(n => !n.read).length }); return }
    if (/^\/admin\/notifications\/\d+\/read$/.test(path)) { const item = notifications.find(n => n.id === Number(path.split('/')[3])); if (item) item.read = true; json({ ok: true }); return }
    json({ accounts: [], instances: [], groups: [], proxies: [], routes: [], keys: [], logs: [], rules: [], runs: [], credentials: [], plugins: [], notifications: [], unread: 0, trend: [], total: 0, active_accounts: 1, running_plugins: 1 })
  })
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
  return {
    url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`, token: `${profile}-${role}-token`, requests, settings, mcp, mcpTools, extensions, packages, identities, notifications,
    capabilities: (status: number) => { capabilityStatus = status },
    close: () => new Promise<void>(resolve => { server.close(() => resolve()); server.closeAllConnections() }),
  }
}

async function connect(page: Page, target: Awaited<ReturnType<typeof backend>>, clientPort = 4173, path = '/') {
  // 服务器版在同源路径访问 API；浏览器路由模拟反向代理到测试后端。
  await page.route(new RegExp(`^http://127\\.0\\.0\\.1:${clientPort}/(?:admin/|assets/plugins/)`), async route => {
    const request = new URL(route.request().url())
    await route.fulfill({ response: await route.fetch({ url: target.url + request.pathname + request.search }) })
  })
  await page.addInitScript(({ url, token }) => {
    if (window.top !== window) return
    if (localStorage.getItem('cph-test-seeded')) return
    localStorage.setItem('cph-test-seeded', '1')
    localStorage.setItem('cph-locale', 'en')
    localStorage.setItem('cph-connections', JSON.stringify([{ id: 'test', name: 'Test backend', baseURL: url }]))
    localStorage.setItem('cph-active-connection', 'test')
    localStorage.setItem('cph-token:test', 'unused-remote-token')
    localStorage.setItem('cph-token:same-origin', token)
  }, { url: target.url, token: target.token })
  await page.goto(`http://127.0.0.1:${clientPort}${path}`)
  await expect(page.locator('.aside-menu')).toBeVisible()
  await expect(page.locator('.connection-button')).toHaveCount(0)
}

test('Web retains gateway capabilities when connected to a headless backend', async ({ page }) => {
  const target = await backend()
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  try {
    await connect(page, target, 4173)
    await expect(page.locator('.aside-menu')).toContainText('Routes')
    await page.goto('http://127.0.0.1:4173/keys')
    await expect(page).toHaveURL(/\/keys$/)
    await expect.poll(() => target.requests.some(r => r.path === '/admin/keys')).toBe(true)
    await page.goto('http://127.0.0.1:4173/logs')
    await expect(page.locator('.logs-body')).toBeVisible()
    await expect(page.getByText('Request Logs', { exact: true })).toBeVisible()
    await page.goto('http://127.0.0.1:4173/settings')
    await expect(page.locator('.settings-tabs')).toBeVisible()
    await expect(page.locator('.settings-tabs .t-tabs__header').getByText('Gateway', { exact: true })).toBeVisible()
    await page.goto('http://127.0.0.1:4173/accounts')
    await expect(page.getByText('Demo', { exact: true }).first()).toBeVisible()
    await expect(page.getByText('Test', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('full UI retains request logs, gateway settings and online account testing', async ({ page }) => {
  const target = await backend()
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  try {
    await connect(page, target)
    await expect(page.locator('.aside-menu')).toContainText('Routes')
    await expect(page.locator('.aside-menu')).not.toContainText('Extensions')
    await page.locator('.user-chip').click()
    await expect(page.locator('.user-menu-item').filter({ hasText: 'Extensions' })).toBeVisible()
    await page.locator('.user-chip').click()
    await page.goto('http://127.0.0.1:4173/extensions')
    await expect(page.getByText('Current site', { exact: false })).toHaveCount(0)
    await expect(page.getByText('Install on current backend', { exact: false })).toHaveCount(0)
    await page.goto('http://127.0.0.1:4173/logs')
    await page.getByText('Run Logs', { exact: true }).click()
    await expect.poll(() => target.requests.some(r => r.path === '/admin/run-logs')).toBe(true)
    await page.goto('http://127.0.0.1:4173/settings')
    await expect(page.locator('.settings-tabs .t-tabs__header').getByText('Gateway', { exact: true })).toBeVisible()
    for (const name of ['Network', 'System']) await expect(page.locator('.settings-tabs .t-tabs__header').getByText(name, { exact: true })).toBeVisible()
    await expect(page.locator('.settings-tabs .t-tabs__header').getByText('Plugins', { exact: true })).toHaveCount(0)
    await page.locator('.save-row:visible button').click()
    await expect.poll(() => target.settings.first_token_timeout).toBe(120)
    await page.goto('http://127.0.0.1:4173/accounts')
    await page.getByText('Test', { exact: true }).click()
    await expect(page.getByText('Online Test', { exact: true })).toBeVisible()
    await page.locator('.t-drawer button').filter({ hasText: 'Send Test' }).click()
    await expect(page.getByText('Test reply', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('guest permissions constrain direct routes; capability failures do not enable legacy fallback', async ({ page }) => {
  const target = await backend('full', 'guest')
  try {
    await connect(page, target)
    await page.goto('http://127.0.0.1:4173/plugins/editor')
    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.locator('.aside-menu')).not.toContainText('Plugins')
    target.capabilities(500)
    await page.reload()
    await expect(page).toHaveURL(/\/unavailable$/)
    target.capabilities(404)
    await page.goto('http://127.0.0.1:4173/')
    await expect(page.locator('.t-alert')).toContainText('Legacy')
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})


async function nativeWorkspace(page: Page, target: Awaited<ReturnType<typeof backend>>, connected = true, systemNotifications = false, notification = 0) {
  await page.addInitScript(({ url, token, connected, systemNotifications, notification }) => {
    const calls: string[] = []
    ;(window as any).nativeCalls = calls
    ;(window as any).nativeNotifications = systemNotifications
    const bridge = {
      onmessage: undefined as undefined | ((event: { data: string }) => void),
      postMessage(message: string) {
        const request = JSON.parse(message); calls.push(request.type)
        const result = request.type === 'notifications-sync' ? { native: (window as any).nativeNotifications } : { version: '1.0.0', package: 'github.shadowbaby.clawproxyhub.core', remoteOnly: !connected, notification,
          locale: 'en', theme: 'light', connection: connected ? { id: 'native-selected', name: 'Workspace', baseURL: url, token } : undefined }
        setTimeout(() => bridge.onmessage?.({ data: JSON.stringify({ id: request.id, result }) }), 0)
      },
    }
    ;(window as any).cphPlatform = bridge
  }, { url: target.url, token: target.token, connected, systemNotifications, notification })
}

test('Android notifications open the selected message and fall back to workspace delivery', async ({ page }) => {
  const target = await backend()
  target.notifications.push({ id: 7, title: 'Task completed', content: 'Result from selected workspace', created_at: '2026-10-06T12:00:00Z', read: false, level: 'info' })
  try {
    await nativeWorkspace(page, target, true, true, 7)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('http://127.0.0.1:4174/')
    await expect(page.locator('.notif-content')).toHaveText('Result from selected workspace')
    await expect.poll(() => target.notifications[0]!.read).toBe(true)
    await expect(page.getByRole('button', { name: 'Notifications', exact: true })).toHaveCount(0)
    await page.evaluate(() => { (window as any).nativeNotifications = false; window.dispatchEvent(new Event('cph:notifications-changed')) })
    await expect(page.getByRole('button', { name: 'Notifications', exact: true })).toHaveCount(1)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

for (const [profile, port] of [['app-full', 4174]] as const) {
test(`${profile} retains gateway business pages without duplicating native plugin management`, async ({ page }) => {
  const target = await backend()
  target.extensions.runtime_management = 'native'
  try {
    await nativeWorkspace(page, target)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`http://127.0.0.1:${port}/`)
    await expect(page).toHaveURL(/#\/dashboard$/)
    await expect(page.locator('.application-tabs, .connection-button, .aside-menu')).toHaveCount(0)
    await page.goto(`http://127.0.0.1:${port}/#/plugins`)
    await expect(page).toHaveURL(/#\/dashboard$/)
    await page.goto(`http://127.0.0.1:${port}/#/routes`)
    await expect(page).toHaveURL(/#\/routes$/)
    await expect.poll(() => target.requests.some(r => r.path === '/admin/routes')).toBe(true)
    await page.goto(`http://127.0.0.1:${port}/#/settings`)
    await page.locator('.mobile-fab button').first().click()
    await expect(page.locator('.phone-menu-item')).toHaveText(['Gateway', 'Logs', 'Tasks', 'MCP'])
    await page.locator('.phone-menu-item').filter({ hasText: 'MCP' }).click()
    await expect(page.locator('.mobile-fab button')).toHaveCount(2)
    await page.goto(`http://127.0.0.1:${port}/#/extensions`)
    await expect(page.getByText('Install on current backend', { exact: false })).toHaveCount(0)
    await expect(page.locator('.mobile-fab').getByRole('button', { name: 'Local Install', exact: true })).toBeEnabled()
    await expect(page.locator('input[type=file]')).toHaveAttribute('accept', '.cphext')
    await expect(page.getByText('Manage runtimes and the workspace interface in the native APP settings.')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    target.extensions.runtime_management = 'core'
    await page.reload()
    await expect(page.locator('input[type=file]')).toHaveAttribute('accept', '.cphext,.cphhost')
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})
}

test('app without a selected workspace never assumes same-origin credentials or starts local core itself', async ({ page }) => {
  const target = await backend()
  try {
    await nativeWorkspace(page, target, false)
    await page.goto('http://127.0.0.1:4174/')
    await expect(page).toHaveURL(/#\/connections$/)
    expect(await page.evaluate(() => (window as any).nativeCalls.includes('connect-local'))).toBe(false)
    expect(target.requests.length).toBe(0)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})


test('MCP settings manage an independent key and online permissions without built-in AI', async ({ page, context }) => {
  const target = await backend()
  try {
    await connect(page, target, 4173, '/settings')
    await page.locator('.settings-tabs .t-tabs__header').getByText('MCP', { exact: true }).click()
    const panel = page.locator('.mcp-settings')
    await expect(panel).toContainText('Disabled')
    await panel.locator('.t-switch').click()
    await expect(panel).toContainText('Enabled')
    await panel.locator('.t-checkbox').click()
    await expect(panel.getByRole('checkbox')).toBeChecked()
    await panel.getByRole('button', { name: 'Save', exact: true }).click()
    await panel.getByRole('button', { name: 'Generate', exact: true }).click()
    await expect(panel).toContainText('Copy and save the full key now')
    await expect(panel.locator('.mcp-key input')).toHaveValue('cph_mcp_••••••••ture')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await panel.getByRole('button', { name: 'Copy key', exact: true }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('cph_mcp_browser_fixture')
    await expect(page.locator('body')).not.toContainText('AI and external tools')
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})


test('mobile MCP uses compact rows, scrollable permissions and floating save', async ({ page }) => {
  const target = await backend()
  for (let i = 0; i < 20; i++) target.mcpTools.push({ id: `demo.task${i}`, owner: 'demo', title: `Read task ${i}`, permission: 'tasks.read', effect: 'read' })
  try {
    await connect(page, target, 4173, '/settings')
    await page.locator('.settings-tabs .t-tabs__header').getByText('MCP', { exact: true }).click()
    await page.setViewportSize({ width: 390, height: 844 })
    const panel = page.locator('.mcp-settings')
    await expect(panel.locator('.s-form-item')).toHaveCount(1)
    await panel.locator('.t-switch').click()
    await expect(panel.locator('.s-form-item')).toHaveCount(4)
    await expect(panel.locator('.s-form-label')).toHaveText(['MCP status', 'MCP connection', 'MCP authentication', 'MCP permissions'])
    const key = panel.locator('.mcp-key input')
    const width = (await key.boundingBox())!.width
    await panel.getByRole('button', { name: 'Generate', exact: true }).click()
    await expect(key).toHaveValue('cph_mcp_••••••••ture')
    expect(Math.abs((await key.boundingBox())!.width - width)).toBeLessThan(1)
    const scroll = await panel.locator('.mcp-tools').evaluate(node => ({ height: node.clientHeight, scroll: node.scrollHeight }))
    expect(scroll.height).toBeLessThanOrEqual(374)
    expect(scroll.scroll).toBeGreaterThan(scroll.height)
    await panel.locator('.t-checkbox').first().click()
    await page.locator('.mobile-fab').getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => target.mcp.allowed_actions).toEqual(['core.status'])
    await panel.locator('.t-switch').click()
    await expect(panel.locator('.s-form-item')).toHaveCount(1)
    await expect.poll(() => target.mcp.enabled).toBe(false)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('mobile extension cards keep gaps and wrap their actions', async ({ page }) => {
  const target = await backend()
  target.extensions.system_components.push(...['core', 'lua'].map(id => ({ id, name: id, version: '1.0.0', installed: true, available: true })))
  for (let i = 0; i < 3; i++) target.extensions.extensions.push({ manifest: { id: `example.long-extension-name-${i}`, version: '1.0.0', name: `Example extension ${i}`, kind: 'frontend', target: 'core', permissions: ['workspace.read'] }, publisher: 'Example', status: 'enabled', enabled: true, bytes: 123 })
  try {
    await connect(page, target, 4173, '/extensions')
    await page.setViewportSize({ width: 360, height: 800 })
    const cards = page.locator('.extension-list .c-card')
    await expect(cards).toHaveCount(3)
    await expect(page.locator('.page-layout-header')).toHaveCount(0)
    const install = page.locator('.mobile-fab').getByRole('button', { name: 'Local Install', exact: true })
    await expect(install).toBeVisible()
    await expect(install).toBeEnabled()
    await expect(page.locator('.mobile-fab').getByRole('button', { name: 'Trust Sources', exact: true })).toBeVisible()
    await expect(page.locator('.mobile-fab').getByRole('button', { name: 'Extension Market', exact: true })).toBeVisible()
    for (const theme of ['light', 'dark']) {
      await page.evaluate(value => document.documentElement.setAttribute('theme-mode', value), theme)
      const rects = await cards.evaluateAll(nodes => nodes.map(node => { const r = node.getBoundingClientRect(); return { top: r.top, bottom: r.bottom, right: r.right } }))
      expect(rects[1]!.top - rects[0]!.bottom).toBeGreaterThanOrEqual(15)
      expect(rects[2]!.top - rects[1]!.bottom).toBeGreaterThanOrEqual(15)
      expect(rects.every(rect => rect.right <= 360)).toBe(true)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    }
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})


test('empty pages hide pagination and broken entity icons fall back to initials', async ({ page }) => {
  const target = await backend()
  try {
    await connect(page, target, 4173, '/instances')
    await expect(page.locator('.c-pagination')).toHaveCount(0)
    await page.route('**/assets/plugins/demo/icon', route => route.fulfill({ status: 404, body: '' }))
    await page.goto('http://127.0.0.1:4173/accounts')
    await expect(page.locator('.c-pagination')).toBeVisible()
    await page.getByRole('button', { name: 'Add Account', exact: true }).click()
    await expect(page.locator('.entity-icon img')).toHaveCount(0)
    await expect(page.locator('.entity-icon').filter({ hasText: 'D' }).first()).toBeVisible()
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('desktop extension installation requires explicit permission grants', async ({ page }) => {
  const target = await backend()
  try {
    await connect(page, target, 4173, '/extensions')
    await expect(page.getByRole('button', { name: 'Local Install', exact: true })).toBeEnabled()
    await expect(page.getByText('No data', { exact: true }).first()).toBeVisible()
    await page.route('**/admin/extensions/inspect', route => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ manifest: { id: 'example', name: 'Example', label: { zh: '示例扩展', en: 'Example extension' }, version: '0.1.0', kind: 'data', target: 'backend', activation: 'hot', permissions: ['workspace.read'] }, hash: 'a'.repeat(64), signer: 'private-signer-code', publisher: 'Example publisher' }) }))
    await page.locator('input[type=file]').setInputFiles({ name: 'example.cphext', mimeType: 'application/zip', buffer: Buffer.from('fixture') })
    const dialog = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Review signature and permissions' })
    await expect(dialog.getByText('Example extension · 0.1.0', { exact: true })).toBeVisible()
    await expect(dialog.getByText('Example publisher', { exact: true })).toBeVisible()
    await expect(dialog.getByText('private-signer-code', { exact: true })).toHaveCount(0)
    await expect(dialog.getByText('a'.repeat(64), { exact: true })).toHaveCount(0)
    await expect(dialog.locator('.t-tag')).toHaveText('Read Lua plugin source')
    await expect(dialog.getByRole('button', { name: 'Install', exact: true })).toBeDisabled()
    await dialog.locator('.t-checkbox').click()
    await expect(dialog.getByRole('checkbox')).toBeChecked()
    await expect(dialog.getByRole('button', { name: 'Install', exact: true })).toBeEnabled()
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('bundled extension cards survive removal and the market loads only when opened', async ({ page }) => {
  const target = await backend()
  const manifest = { id: 'lua-editor', name: 'Lua Editor', version: '0.1.0', kind: 'frontend-sandbox', target: 'backend', activation: 'hot', permissions: ['workspace.read'] }
  const editor = { path: '/packages/lua-editor.cphext', manifest, sha256: 'a'.repeat(64), publisher: 'ClawProxyHub', status: 'installed' }
  target.packages.push(editor, { path: '/packages/lua-runtime.cphhost', manifest: { ...manifest, id: 'lua-runtime', name: 'Lua Host', version: '0.2.0', kind: 'runtime' }, sha256: 'b'.repeat(64), publisher: 'ClawProxyHub', status: 'available' })
  target.extensions.extensions.push({ manifest, publisher: 'ClawProxyHub', status: 'disabled', enabled: false, available: false, bytes: 2048 })
  try {
    await connect(page, target, 4173, '/extensions')
    await expect(page.locator('.page-layout-header button')).toHaveText(['Trust Sources', 'Extension Market', 'Local Install'])
    const card = page.locator('[data-extension="lua-editor"]')
    await expect(card).toHaveCount(1)
    await expect(card.getByText('Built-in', { exact: true })).toBeVisible()
    await expect(card.getByText('Disabled', { exact: true })).toBeVisible()
    expect(target.requests.some(request => request.path.includes('marketplace') || request.path === '/admin/extension-trust')).toBe(false)
    for (const theme of ['light', 'dark']) {
      await page.evaluate(value => localStorage.setItem('cph-theme', value), theme)
      await page.reload()
      await expect(card.getByText('Disabled', { exact: true })).toBeVisible()
      await page.screenshot({ path: '../build/verification/extension-center-' + theme + '.png', fullPage: true })
    }
    target.extensions.extensions.length = 0
    editor.status = 'removed'
    await page.reload()
    await expect(card).toHaveCount(1)
    await expect(card.getByText('Not installed', { exact: true })).toBeVisible()
    await expect(card.locator('.t-link').filter({ hasText: /^Install$/ })).toBeVisible()
    await page.setViewportSize({ width: 360, height: 800 })
    await expect(page.locator('.mobile-fab button')).toHaveCount(3)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: '../build/verification/extension-center-mobile.png', fullPage: true })
    await page.getByRole('button', { name: 'Extension Market', exact: true }).click()
    await expect.poll(() => target.requests.some(request => request.path === '/admin/extensions/marketplace')).toBe(true)
    await expect(page.locator('.c-drawer.t-drawer--open .t-empty')).toContainText('No online extensions')
    await expect(page.locator('.c-drawer.t-drawer--open').getByRole('button', { name: 'Refresh', exact: true })).toHaveCount(0)
    await page.route('**/admin/extensions/marketplace?*', route => route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'release package index is unavailable; mounted packages remain available offline' }) }))
    await page.reload()
    await page.getByRole('button', { name: 'Extension Market', exact: true }).click()
    await expect(page.locator('.c-drawer.t-drawer--open .t-empty')).toContainText('Online source is unavailable')
    await expect(page.getByText('release package index is unavailable', { exact: false })).toHaveCount(0)
    await expect(page.locator('.c-drawer.t-drawer--open .t-alert')).toHaveCount(0)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('same-version updates use the install review and uninstall reports completion with a toast', async ({ page }) => {
  const target = await backend()
  const manifest = { id: 'lua-editor', name: 'Lua Editor', version: '0.1.0', kind: 'frontend-sandbox', permissions: ['workspace.read'], pages: [{ id: 'editor', title: 'Lua Editor', entry: 'frontend/index.html' }] }
  const installed = { manifest, hash: 'a'.repeat(64), publisher: 'Publisher', status: 'disabled', enabled: false, available: false, bytes: 100 }
  const bundled = { path: 'packages/lua-editor.cphext', manifest, sha256: 'b'.repeat(64), status: 'update', installed_version: '0.1.0' }
  target.packages.push(bundled)
  target.extensions.extensions.push(installed)
  const installs: unknown[] = []
  try {
    await connect(page, target, 4173, '/extensions')
    await page.route('**/admin/extensions/install-mounted', async route => {
      installs.push(route.request().postDataJSON())
      installed.hash = bundled.sha256; bundled.status = 'installed'
      await route.fulfill({ json: installed })
    })
    await page.route('**/admin/actions/core.extensions.impact', route => route.fulfill({ json: { extensions: [], plugins: [], tasks: [], pages: [], actions: [] } }))
    await page.route('**/admin/actions/core.extensions.uninstall', async route => {
      target.extensions.extensions.length = 0; bundled.status = 'removed'
      await route.fulfill({ json: { ok: true } })
    })
    const card = page.locator('[data-extension="lua-editor"]')
    await expect(card.locator('.extension-actions').getByText('Lua Editor', { exact: true })).toHaveCount(0)
    await card.locator('.t-link').filter({ hasText: /^Update$/ }).click()
    const review = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Review signature and permissions' })
    await expect(review.locator('.t-alert')).toContainText('keeping data')
    await expect(review.getByRole('button', { name: 'Update', exact: true })).toBeDisabled()
    await review.locator('.t-checkbox').click()
    await review.getByRole('button', { name: 'Update', exact: true }).click()
    await expect(review).toHaveCount(0)
    expect(installs).toEqual([{ sha256: bundled.sha256, grants: ['workspace.read'] }])
    await expect(card.getByText('Disabled', { exact: true })).toBeVisible()
    await expect(card.locator('.t-link').filter({ hasText: /^Update$/ })).toHaveCount(0)
    await expect(page.locator('.t-message').filter({ hasText: 'Updated' })).toBeVisible()
    Object.assign(installed, { enabled: true, available: true, status: 'enabled' })
    await page.reload()
    await expect(card.getByText('Enabled', { exact: true })).toBeVisible()
    await expect(card.locator('.extension-actions').getByText('Lua Editor', { exact: true })).toHaveCount(0)
    await card.locator('.t-link').filter({ hasText: /^Uninstall$/ }).click()
    const removal = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Dependencies' })
    await removal.getByRole('button', { name: 'Uninstall', exact: true }).click()
    await expect(removal).toHaveCount(0)
    await expect(page.locator('.t-message').filter({ hasText: 'Extension uninstalled.' })).toBeVisible()
    await expect(page.locator('.page-layout-body > .extension-notice')).toHaveCount(0)
    await expect(card.locator('.t-link').filter({ hasText: /^Install$/ })).toBeVisible()
    await expect(card.locator('.t-link').filter({ hasText: /^Clean up$/ })).toHaveCount(1)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('runtime settings render signed readonly fields in the shared drawer', async ({ page }) => {
  const target = await backend()
  const field = { id: 'isolation', kind: 'toggle', label: { zh: '隔离运行', en: 'Isolated execution' }, default: true, readonly: true }
  const hash = 'b'.repeat(64)
  target.extensions.extensions.push({ manifest: { id: 'lua-runtime', name: 'Lua Host', version: '0.2.0', kind: 'runtime', permissions: [], settings: [field] },
    hash, enabled: true, available: true, status: 'active', publisher: 'ClawProxyHub', bytes: 100 })
  try {
    await connect(page, target, 4173, '/extensions')
    await page.route('**/admin/extensions/lua-runtime/settings', route => route.fulfill({ json: { hash, fields: [field], values: { isolation: true } } }))
    await page.locator('[data-extension="lua-runtime"]').getByText('Settings', { exact: true }).click()
    const drawer = page.locator('.extension-settings.t-drawer--open')
    await expect(drawer).toContainText('Isolated execution')
    await expect(drawer.locator('.t-switch')).toHaveClass(/t-is-disabled/)
    await drawer.locator('.t-switch').click()
    await expect(drawer.locator('.t-switch')).toHaveClass(/t-is-checked/)
    await expect(drawer.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0)
  } finally { await target.close() }
})

test('extension dependencies show localized page and action names with legacy fallbacks', async ({ page }) => {
  const target = await backend()
  const manifest = {
    id: 'tool.box', name: 'Toolbox', label: { zh: '工具箱', en: 'Toolbox' }, version: '0.1.0', kind: 'frontend-sandbox', permissions: [],
    pages: [{ id: 'editor', title: 'Editor', labels: { zh: '编辑工作区', en: 'Editor workspace' }, entry: 'frontend/index.html' }],
    actions: [
      { id: 'create', target: 'core.workspace.create', title: 'create', labels: { zh: '新建插件', en: 'Create plugin' } },
      { id: 'read', target: 'core.workspace.read', title: 'read' },
      { id: 'export', target: 'custom.export', title: 'Export' },
    ],
  }
  target.extensions.extensions.push({ manifest, publisher: 'Publisher', status: 'enabled', enabled: true, available: true, bytes: 100 })
  try {
    await connect(page, target, 4173, '/extensions')
    await page.route('**/admin/actions/core.extensions.impact', route => route.fulfill({ json: {
      extensions: ['tool.box'], plugins: [], tasks: [], pages: ['tool.box/editor'], actions: ['tool.box.create', 'tool.box.read', 'tool.box.export'],
    } }))
    const card = page.locator('[data-extension="tool.box"]')
    await card.locator('.t-link').filter({ hasText: /^Dependencies$/ }).click()
    const drawer = page.locator('.c-drawer.t-drawer--open')
    await expect(drawer).toContainText('Dependencies: Toolbox')
    await expect(drawer).toContainText('Pages: Editor workspace')
    await expect(drawer).toContainText('Actions: Create plugin, Read scripts, Export')
    await expect(drawer).not.toContainText('tool.box.create')
    await drawer.locator('.t-drawer__close-btn').click()
    await page.locator('button:has(.t-icon-translate)').click()
    await page.locator('.lang-menu-item').filter({ hasText: '中文' }).click()
    await card.locator('.t-link').filter({ hasText: /^依赖$/ }).click()
    await expect(drawer).toContainText('依赖: 工具箱')
    await expect(drawer).toContainText('页面: 编辑工作区')
    await expect(drawer).toContainText('操作: 新建插件, 读取脚本, Export')
    await expect(drawer).not.toContainText('CLI')
    await expect(drawer).not.toContainText('tool.box/editor')
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('cleanup protects installed data and offers retained data cleanup after uninstall', async ({ page }) => {
  const target = await backend()
  const manifest = { id: 'example', name: 'Example', version: '0.1.0', kind: 'data', permissions: [] }
  const bundled = { path: 'packages/example.cphext', manifest, sha256: 'a'.repeat(64), status: 'installed' }
  target.packages.push(bundled)
  target.extensions.extensions.push({ manifest, publisher: 'Publisher', status: 'enabled', enabled: true, available: true, bytes: 100 })
  try {
    await connect(page, target, 4173, '/extensions')
    const card = page.locator('[data-extension="example"]')
    await page.route('**/admin/actions/core.extensions.obsolete', route => route.fulfill({ json: { hash: bundled.sha256, tables: [] } }))
    await card.getByText('Clean up', { exact: true }).click()
    const drawer = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Retained data' })
    await expect(drawer.getByRole('checkbox', { name: 'Cache', exact: true })).toBeChecked()
    await expect(drawer.getByRole('checkbox', { name: 'Retained data', exact: true })).toBeDisabled()
    await drawer.getByRole('button', { name: 'Clean up', exact: true }).click()
    await expect(drawer).toHaveCount(0)
    expect(target.requests.some(request => request.path === '/admin/actions/core.extensions.cache')).toBe(true)
    expect(target.requests.some(request => request.path === '/admin/actions/core.extensions.data')).toBe(false)
    target.extensions.extensions.length = 0; bundled.status = 'removed'
    await page.reload()
    await card.getByText('Clean up', { exact: true }).click()
    await drawer.locator('.t-checkbox').filter({ hasText: 'Retained data' }).click()
    await expect(drawer.getByRole('checkbox', { name: 'Retained data', exact: true })).toBeChecked()
    await expect(drawer.locator('.t-alert')).toContainText('permanently deleted')
    await drawer.getByRole('button', { name: 'Clean up', exact: true }).click()
    await expect.poll(() => target.requests.some(request => request.path === '/admin/actions/core.extensions.data')).toBe(true)
    await expect(card.getByText('Not installed', { exact: true })).toBeVisible()
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

test('trust sources extract Code from signing JSON and reject invalid configuration before saving', async ({ page }) => {
  const target = await backend()
  const preset = { code: 'bundled-signer', config: { public_key: Buffer.alloc(32, 2).toString('base64'), publisher: 'Bundled publisher', ids: ['lua-editor'], permissions: [] }, read_only: true }
  target.identities.push(preset)
  try {
    await connect(page, target, 4173, '/extensions')
    await page.getByRole('button', { name: 'Trust Sources', exact: true }).click()
    const presetCard = page.locator('.trust-card').filter({ hasText: 'Bundled publisher' })
    await expect(presetCard.getByText('Preset', { exact: true })).toBeVisible()
    await expect(presetCard.getByText('Edit', { exact: true })).toHaveCount(0)
    await expect(presetCard.getByText('Delete', { exact: true })).toHaveCount(0)
    await expect(presetCard.getByText('bundled-signer', { exact: true })).toHaveCount(0)
    await expect(presetCard.getByText('lua-editor', { exact: true })).toHaveCount(0)
    await presetCard.click()
    const details = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Trust source details' })
    await expect(details.locator('textarea')).toHaveJSProperty('readOnly', true)
    await expect(details.locator('textarea')).toHaveValue(/"key_id": "bundled-signer"/)
    await details.locator('.t-drawer__close-btn').click()
    await page.getByRole('button', { name: 'Add trust source', exact: true }).click()
    const field = page.locator('textarea[name="signing-json"]')
    await expect(field).toHaveAttribute('placeholder', /"key_id"/)
    const save = page.getByRole('button', { name: 'Save', exact: true })
    await field.fill('{')
    await expect(page.getByText('Enter valid JSON.', { exact: true })).toBeVisible()
    await expect(save).toBeDisabled()
    const config = { public_key: Buffer.alloc(32, 1).toString('base64'), publisher: 'Test publisher', ids: ['lua-editor'], permissions: [] }
    await field.fill(JSON.stringify({ key_id: 'test-signer', ...config, permissions: 'workspace.read' }))
    await expect(save).toBeDisabled()
    expect(target.identities).toEqual([preset])
    await field.fill(JSON.stringify({ key_id: 'test-signer', ...config }, null, 2))
    const code = page.locator('.s-form-item').filter({ hasText: 'Code' }).locator('input')
    await expect(code).toHaveValue('test-signer')
    await expect(code).toHaveJSProperty('readOnly', true)
    await expect(save).toBeEnabled()
    await page.screenshot({ path: '../build/verification/extension-trust-form.png', fullPage: true })
    await save.click()
    await expect.poll(() => target.identities).toEqual([preset, { code: 'test-signer', config, read_only: false }])
    const customCard = page.locator('.trust-card').filter({ hasText: 'Test publisher' })
    await expect(customCard).toBeVisible()
    await expect(customCard.getByText('Third-party', { exact: true })).toBeVisible()
    await customCard.click()
    await field.fill(JSON.stringify({ key_id: 'another-signer', ...config }))
    await expect(save).toBeDisabled()
    expect(target.identities[1]?.code).toBe('test-signer')
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

function environmentState(id: string, environments: ('app' | 'web-desktop' | 'web-mobile')[]) {
  return {
    manifest: { id, name: id, version: '0.1.0', kind: 'frontend-sandbox', target: 'backend', activation: 'hot', permissions: [], environments,
      pages: [{ id: 'editor', title: id, entry: 'frontend/index.html' }],
      contributions: [{ id: 'open', location: 'plugins.toolbar', label: id, page: 'editor' }] },
    hash: id.charCodeAt(0).toString(16).repeat(32), signer: 'test', publisher: 'Publisher', enabled: true, available: true, status: 'enabled', bytes: 12,
  }
}

async function environmentSandbox(page: Page) {
  await page.route('**/extension-assets/**', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><output></output><script>
    addEventListener('message', event => {
      if (event.data.type === 'cph:init' || event.data.type === 'cph:context') {
        document.querySelector('output').textContent = event.data.context.environment;
        if (event.data.type === 'cph:init') parent.postMessage({type:'cph:ready', nonce:event.data.nonce}, '*');
      }
    });
  </script>` }))
}

test('extension environments filter Web contributions and revoke incompatible pages without disabling the backend', async ({ page }) => {
  const target = await backend()
  const desktop = environmentState('desktop-feature', ['web-desktop'])
  const mobile = environmentState('mobile-feature', ['web-mobile'])
  const app = environmentState('app-feature', ['app'])
  const universal = environmentState('universal-feature', ['app', 'web-desktop', 'web-mobile'])
  target.extensions.extensions.push(desktop, mobile, app, universal)
  try {
    await environmentSandbox(page)
    await connect(page, target, 4173, '/plugins')
    await expect(page.getByRole('button', { name: desktop.manifest.name, exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: mobile.manifest.name, exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: app.manifest.name, exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: desktop.manifest.name, exact: true }).click()
    await expect(page.frameLocator('iframe').locator('output')).toHaveText('web-desktop')
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.locator('iframe')).toHaveCount(0)
    await expect(page.getByText('Not supported in this interface', { exact: true })).toBeVisible()
    await page.setViewportSize({ width: 1280, height: 900 })
    await expect(page.frameLocator('iframe').locator('output')).toHaveText('web-desktop')
    await page.goto('http://127.0.0.1:4173/extensions/universal-feature/editor')
    await expect(page.frameLocator('iframe').locator('output')).toHaveText('web-desktop')
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.frameLocator('iframe').locator('output')).toHaveText('web-mobile')
    await page.goto('http://127.0.0.1:4173/plugins')
    await expect(page.getByRole('button', { name: mobile.manifest.name, exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: desktop.manifest.name, exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: app.manifest.name, exact: true })).toHaveCount(0)
    await page.goto('http://127.0.0.1:4173/extensions/desktop-feature/editor')
    await expect(page.locator('iframe')).toHaveCount(0)
    await expect(page.getByText('Not supported in this interface', { exact: true })).toBeVisible()
    expect(target.requests.some(request => request.path.includes('/actions/'))).toBe(false)
    expect([desktop, mobile, app, universal].every(state => state.enabled && state.available)).toBe(true)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})

for (const surface of ['web', 'app'] as const) {
test(`extension environments enforce ${surface} installation using the verified manifest`, async ({ page }) => {
  const target = await backend()
  const supported = environmentState('supported-feature', surface === 'app' ? ['app'] : ['web-desktop'])
  const unsupported = environmentState('unsupported-feature', surface === 'app' ? ['web-desktop'] : ['app'])
  const managed = { ...supported, enabled: false, available: false }
  target.extensions.extensions.push(managed)
  target.packages.push({ path: 'packages/unsupported.cphext', manifest: unsupported.manifest, sha256: unsupported.hash, status: 'available' })
  let preview = unsupported, installed = 0
  try {
    if (surface === 'app') {
      await nativeWorkspace(page, target)
      await page.goto('http://127.0.0.1:4174/#/extensions')
    } else await connect(page, target, 4173, '/extensions')
    await page.setViewportSize({ width: 390, height: 844 })
    const card = page.locator('[data-extension="unsupported-feature"]')
    await expect(card).toContainText('Built-in')
    await expect(card).toContainText('Not supported in this interface')
    await expect(card.getByText('Install', { exact: true })).toHaveCount(0)
    const installedCard = page.locator('[data-extension="supported-feature"]')
    await expect(installedCard.getByText('Enable', { exact: true })).not.toHaveClass(/t-is-disabled/)
    await expect(installedCard.getByText('Clean up', { exact: true })).toBeVisible()
    await page.route('**/admin/extensions/inspect', route => route.fulfill({ json: preview }))
    await page.route('**/admin/extensions/install', route => { installed++; return route.fulfill({ json: supported }) })
    const upload = () => page.locator('input[type=file]').setInputFiles({ name: 'example.cphext', mimeType: 'application/zip', buffer: Buffer.from('fixture') })
    const drawer = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Review signature and permissions' })
    await upload()
    await expect(drawer.getByText('Not supported in this interface', { exact: true })).toBeVisible()
    await expect(drawer.getByRole('checkbox')).toBeDisabled()
    await expect(drawer.getByRole('button', { name: 'Install', exact: true })).toBeDisabled()
    await drawer.locator('.t-drawer__mask').click({ position: { x: 8, y: 8 } })
    preview = { ...supported, manifest: { ...supported.manifest, id: 'new-feature' } }
    await upload()
    if (surface === 'web') await expect(drawer.getByText('You can install and manage this extension. Its features appear only in supported interfaces.', { exact: true })).toBeVisible()
    await drawer.locator('.t-checkbox').click()
    await expect(drawer.getByRole('checkbox')).toBeChecked()
    await expect(drawer.getByRole('button', { name: 'Install', exact: true })).toBeEnabled()
    await drawer.getByRole('button', { name: 'Install', exact: true }).click()
    await expect.poll(() => installed).toBe(1)
    await expect(drawer).toHaveCount(0)
    await page.route('**/admin/extensions/marketplace?*', route => route.fulfill({ json: { packages: [{ ...supported.manifest, display_name: 'Market feature', name: 'market.cphext', sha256: supported.hash, status: 'available', size: 12 }], cached: false } }))
    await page.route('**/admin/extensions/inspect-market', route => route.fulfill({ json: { ...unsupported, ticket: 'verified-ticket' } }))
    await page.getByRole('button', { name: 'Extension Market', exact: true }).click()
    const market = page.locator('.market-card').filter({ hasText: 'Market feature' })
    await market.getByText('Install', { exact: true }).click()
    await expect(drawer.getByRole('button', { name: 'Install', exact: true })).toBeDisabled()
    await expect(drawer.getByText('Not supported in this interface', { exact: true })).toBeVisible()
    expect(installed).toBe(1)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})
}

test('extension environments keep App identity at every width and reject Web-only routes', async ({ page }) => {
  const target = await backend()
  target.extensions.extensions.push(environmentState('app-feature', ['app']), environmentState('web-feature', ['web-desktop', 'web-mobile']))
  try {
    await environmentSandbox(page)
    await nativeWorkspace(page, target)
    await page.goto('http://127.0.0.1:4174/#/extensions/app-feature/editor')
    await expect(page.frameLocator('iframe').locator('output')).toHaveText('app')
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.frameLocator('iframe').locator('output')).toHaveText('app')
    await page.goto('http://127.0.0.1:4174/#/extensions/web-feature/editor')
    await expect(page.getByText('Not supported in this interface', { exact: true })).toBeVisible()
    await expect(page.locator('iframe')).toHaveCount(0)
  } finally { await page.unrouteAll({ behavior: 'wait' }).catch(() => {}); await target.close() }
})
