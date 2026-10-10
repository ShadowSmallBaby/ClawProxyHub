import { test, expect } from '@playwright/test'
import { spawn, execFileSync } from 'node:child_process'
import { mkdtemp, mkdir, copyFile, rm } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { tmpdir } from 'node:os'
import { once } from 'node:events'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'

// 真实核心与已签名扩展验证包导入、沙箱编辑、停用和卸载收据。
test('mounted Lua editor saves offline and respects disable and uninstall', async ({ page, request }) => {
  test.setTimeout(180000)
  page.setDefaultTimeout(15000)
  const repo = resolve('..'), dir = await mkdtemp(join(tmpdir(), 'cph-editor-test-'))
  const python = process.env.CPH_PYTHON || (process.platform === 'win32' ? 'python' : 'python3')
  const editorVersion = execFileSync(python, ['scripts/project_config.py', 'get', 'components.lua-editor.version'], { cwd: repo, encoding: 'utf8', windowsHide: true }).trim()
  const binary = join(dir, process.platform === 'win32' ? 'cph.exe' : 'cph')
  const sources = join(dir, 'packages'), trust = join(dir, 'trust.json')
  const updates = createServer((_, response) => { response.writeHead(404); response.end() })
  let core: ReturnType<typeof spawn> | undefined
  const start = async () => {
    core = spawn(binary, [], { cwd: dir, windowsHide: true, env: { ...process.env, CPH_INSTALL_PACKAGES: 'true', CPH_DATA_DIR: dir, CPH_DATABASE_DSN: join(dir, 'cph.db'), CPH_PLUGIN_DIR: join(dir, 'plugins'), CPH_PACKAGE_DIRS: sources, CPH_EXTENSION_TRUST: trust, CPH_ADDR: '127.0.0.1:0', CPH_PROFILE: 'full', CPH_ADMIN_USERNAME: 'admin', CPH_ADMIN_PASSWORD: 'editor-test-password' } })
    let log = ''
    core.stderr!.on('data', b => { log += b })
    return new Promise<string>((resolve, reject) => {
      core!.stdout!.on('data', b => { log += b; const match = /listening on (127\.0\.0\.1:\d+)/.exec(log); if (match) resolve('http://' + match[1]) })
      core!.once('exit', () => reject(new Error(log)))
    })
  }
  const stop = async () => {
    if (core && core.exitCode === null) { const exited = once(core, 'exit'); core.kill(); await exited.catch(() => {}) }
  }
  try {
    // 离线编辑测试仍走真实版本 API，更新源固定为本地，避免等待外部网络。
    updates.listen(0, '127.0.0.1')
    await once(updates, 'listening')
    const updateURL = `http://127.0.0.1:${(updates.address() as AddressInfo).port}`
    await mkdir(sources)
    await copyFile(join(repo, `build/packages/lua-editor-${editorVersion}.cphext`), join(sources, 'lua-editor.cphext'))
    await copyFile(join(repo, 'build/packages/trust.json'), trust)
    execFileSync('go', ['build', '-ldflags', `-X github.com/ShadowSmallBaby/ClawProxyHub/internal/version.UpdateRepository=${updateURL}`, '-o', binary, './cmd/cph'], { cwd: repo, windowsHide: true })
    let base = await start()
    const login = await request.post(base + '/admin/login', { data: { username: 'admin', password: 'editor-test-password' } })
    expect(login.ok()).toBeTruthy()
    const { token } = await login.json(), headers = { Authorization: `Bearer ${token}` }
    expect((await request.put(base + '/admin/settings', { headers, data: { plugin_lua_enabled: false } })).ok()).toBeTruthy()
    await page.addInitScript(token => {
      if (window.top !== window) {
        const messages: unknown[] = []
        ;(window as any).__bridgeMessages = messages
        window.addEventListener('message', event => { if (event.source === window.parent) messages.push(event.data) })
        return
      }
      localStorage.setItem('cph-token:same-origin', token); localStorage.setItem('cph-locale', 'en'); localStorage.setItem('cph-theme', 'light')
    }, token)
    const errors: string[] = []
    const actions: string[] = []
    page.on('pageerror', e => errors.push(e.message))
    page.on('request', request => { const path = new URL(request.url()).pathname; if (path.startsWith('/admin/actions/')) actions.push(path) })
    await page.goto(base + '/extensions')
    const settingsCard = page.locator('[data-extension="lua-editor"]')
    const settingsDrawer = page.locator('.extension-settings.t-drawer--open')
    await settingsCard.getByText('Settings', { exact: true }).click()
    await expect(settingsDrawer.locator('.t-select input')).toHaveValue('Dark')
    await settingsDrawer.locator('.t-select').click()
    await page.locator('.t-select-option').filter({ hasText: /^Light$/ }).click()
    await settingsDrawer.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(settingsDrawer).toHaveCount(0)
    await settingsCard.getByText('Settings', { exact: true }).click()
    await expect(settingsDrawer.locator('.t-select input')).toHaveValue('Light')
    await settingsDrawer.locator('.t-select').click()
    await page.locator('.t-select-option').filter({ hasText: /^Follow system$/ }).click()
    await settingsDrawer.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(settingsDrawer).toHaveCount(0)
    await page.goto(base + '/plugins')
    await expect(page.locator('.ver-text')).toHaveText(/^v\d+\.\d+\.\d+$/)
    await page.getByRole('button', { name: /New Lua plugin/ }).click()
    const editor = page.frameLocator('iframe')
    await expect(editor.locator('.cm-content')).toBeVisible()
    await expect(editor.locator('html')).toHaveAttribute('theme-mode', 'light')
    const editorBackground = () => editor.locator('.code-editor').evaluate(node => getComputedStyle(node).backgroundColor)
    const lightBackground = await editorBackground()
    await expect(editor.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0)
    await expect(page.locator('iframe')).toHaveAttribute('sandbox', 'allow-scripts')
    const messages = () => editor.locator('body').evaluate(() => (window as any).__bridgeMessages)
    await expect.poll(async () => (await messages()).find((message: any) => message.type === 'cph:ui-event')?.event.contribution)
      .toEqual({ id: 'new', location: 'plugins.toolbar' })
    const isolated = await editor.locator('body').evaluate(() => {
      let storage = false, host = false
      try { localStorage.getItem('cph-token:same-origin') } catch { storage = true }
      try { window.parent.document.querySelector('body') } catch { host = true }
      return { storage, host }
    })
    expect(isolated).toEqual({ storage: true, host: true })
    await editor.locator('body').evaluate(() => {
      const nonce = (window as any).__bridgeMessages.find((message: any) => message.type === 'cph:init').nonce
      window.parent.postMessage({ type: 'cph:ui', nonce, id: 99999, command: { kind: 'drawer', drawer: { id: 'injected', title: 'Injected component', fields: [{ id: 'x', kind: 'component', component: 'CDrawer', label: 'X' }], submit: { label: 'OK' } } } }, '*')
    })
    await expect.poll(async () => (await messages()).find((message: any) => message.id === 99999)?.error).toBeTruthy()
    await expect(page.getByText('Injected component', { exact: true })).toHaveCount(0)
    const source = '--[[\nlocal PLUGIN_NAME = "commented-name"\n]]\nlocal PLUGIN_NAME = "offline-test"\nlocal PLUGIN_LABEL_ZH = "Offline test"\nreturn {}'
    await editor.locator('.cm-content').fill(source + '\nfunction unfinished(')
    await expect(editor.locator('.editor-problem')).toContainText('Line')
    await editor.locator('.cm-content').fill(source)
    await expect(editor.getByText('Syntax valid', { exact: true })).toBeVisible()
    const save = page.locator('.extension-topbar').getByRole('button', { name: 'Save', exact: true })
    await save.click()
    const form = page.locator('.extension-form.t-drawer--open')
    const name = form.locator('.s-form-item').filter({ hasText: 'Plugin name' }).locator('input')
    await expect(name).toHaveValue('offline-test')
    await expect(name).toHaveJSProperty('readOnly', true)
    await form.locator('input[type=file]').setInputFiles({ name: 'icon.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jv8sAAAAASUVORK5CYII=', 'base64') })
    await expect(form.locator('.extension-image img')).toBeVisible()
    await page.screenshot({ path: '../build/verification/lua-editor-create.png', fullPage: true, animations: 'disabled' })
    await form.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(page.locator('.extension-status')).toHaveText('Saved')
    await expect(form).toHaveCount(0)
    const saved = await request.get(base + '/admin/plugins/offline-test/source?file=main.lua', { headers })
    expect(saved.ok()).toBeTruthy()
    expect((await saved.json()).content.replaceAll('\r\n', '\n')).toBe(source)
    const edited = source + '\n-- saved without runtime'
    await editor.locator('.cm-content').fill(edited)
    // 切换宿主语言和主题不重建沙箱，也不覆盖编辑中的源码。
    await page.locator('button:has(.t-icon-translate)').click()
    await page.locator('.lang-menu-item').filter({ hasText: '中文' }).click()
    await expect(page.locator('.extension-topbar').getByRole('button', { name: '保存', exact: true })).toBeVisible()
    await expect(editor.locator('html')).toHaveAttribute('lang', 'zh')
    await page.locator('button:has(.t-icon-moon)').click()
    await expect(editor.locator('html')).toHaveAttribute('theme-mode', 'dark')
    await expect.poll(editorBackground).not.toBe(lightBackground)
    await expect(editor.locator('.cm-content')).toHaveText(edited.replaceAll('\n', ''))
    await page.screenshot({ path: '../build/verification/lua-editor-dark.png', fullPage: true, animations: 'disabled' })
    const settingsURL = base + '/admin/extensions/lua-editor/settings'
    const config = await (await request.get(settingsURL, { headers })).json()
    const theme = async (value: string) => {
      const response = await request.put(settingsURL, { headers, data: { hash: config.hash, values: { theme: value } } })
      expect(response.ok()).toBeTruthy()
    }
    await theme('light')
    await expect(editor.locator('html')).toHaveAttribute('theme-mode', 'light')
    await expect(page.locator('html')).toHaveAttribute('theme-mode', 'dark')
    await theme('dark')
    await page.locator('button:has(.t-icon-sunny)').click()
    await expect(page.locator('html')).toHaveAttribute('theme-mode', 'light')
    await expect(editor.locator('html')).toHaveAttribute('theme-mode', 'dark')
    await theme('system')
    await expect(editor.locator('html')).toHaveAttribute('theme-mode', 'light')
    await expect(editor.locator('.cm-content')).toHaveText(edited.replaceAll('\n', ''))
    await page.locator('button:has(.t-icon-translate)').click()
    await page.locator('.lang-menu-item').filter({ hasText: 'English' }).click()
    await save.click()
    await expect.poll(async () => (await (await request.get(base + '/admin/plugins/offline-test/source?file=main.lua', { headers })).json()).content.replaceAll('\r\n', '\n')).toBe(edited)
    expect(actions.some(path => path === '/admin/actions/lua-editor.reload')).toBe(false)
    const unsaved = edited + '\n-- retained draft'
    await editor.locator('.cm-content').fill(unsaved)
    await expect(editor.getByText('Draft saved', { exact: true })).toBeVisible()
    const persistentDraft = await request.post(base + '/admin/actions/lua-editor.draft-read', { headers, data: { name: 'offline-test' } })
    expect(persistentDraft.ok()).toBeTruthy()
    expect((await persistentDraft.json()).draft.content).toBe(unsaved)
    await page.locator('.extension-topbar').getByRole('button', { name: 'Back', exact: true }).click()
    const leave = page.locator('.c-drawer.t-drawer--open').filter({ hasText: 'Leave with unsaved changes?' })
    await leave.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(leave).toHaveCount(0)
    await expect(editor.locator('.cm-content')).toHaveText(unsaved.replaceAll('\n', ''))
    await page.locator('.extension-topbar').getByRole('button', { name: 'Back', exact: true }).click()
    await leave.getByRole('button', { name: 'Leave', exact: true }).click()
    await page.getByRole('button', { name: /New Lua plugin/ }).click()
    await expect(editor.locator('.cm-content')).toHaveText(unsaved.replaceAll('\n', ''))
    await expect(page.locator('.extension-topbar').getByRole('button', { name: 'Run', exact: true })).toBeDisabled()
    // 清空会话缓存后从明确的插件入口恢复，确保草稿来自 Go 后端存储。
    await page.evaluate(() => sessionStorage.clear())
    await page.goto(base + '/extensions/lua-editor/editor?name=offline-test&contribution=edit&from=plugins')
    await expect(editor.locator('.cm-content')).toHaveText(unsaved.replaceAll('\n', ''))
    await save.click()
    await expect.poll(async () => (await (await request.get(base + '/admin/plugins/offline-test/source?file=main.lua', { headers })).json()).content.replaceAll('\r\n', '\n')).toBe(unsaved)
    await expect(page.locator('.extension-topbar').getByRole('button', { name: 'Run', exact: true })).toBeEnabled()
    const clearedDraft = await request.post(base + '/admin/actions/lua-editor.draft-read', { headers, data: { name: 'offline-test' } })
    expect((await clearedDraft.json()).found).toBe(false)
    await page.locator('.extension-topbar').getByRole('button', { name: 'Back', exact: true }).click()
    await page.locator('button:has(.t-icon-translate)').click()
    await page.locator('.lang-menu-item').filter({ hasText: '中文' }).click()
    const plugin = page.locator('.plugin-grid .c-card').filter({ hasText: 'Offline test' })
    await plugin.locator('.plugin-ops .t-link').filter({ hasText: /^编辑$/ }).click()
    await expect(editor.locator('.cm-content')).toHaveText(unsaved.replaceAll('\n', ''))
    await expect.poll(async () => (await messages()).find((message: any) => message.type === 'cph:ui-event')?.event)
      .toEqual({ kind: 'activate', page: 'editor', context: { name: 'offline-test' }, contribution: { id: 'edit', location: 'plugins.item.actions' } })
    expect((await request.post(base + '/admin/actions/core.workspace.reload', { headers, data: { name: 'offline-test' } })).ok()).toBeFalsy()
    const catalog = await (await request.get(base + '/admin/extensions/catalog', { headers })).json()
    expect(catalog.packages[0].status).toBe('installed')
    expect(catalog.packages[0].manifest.version).toBe(editorVersion)
    expect((await request.put(base + '/admin/extensions/lua-editor/enabled', { headers, data: { enabled: false } })).ok()).toBeTruthy()
    await expect(page.locator('iframe')).toHaveCount(0)
    await page.goto(base + '/plugins')
    await expect(page.getByRole('button', { name: /New Lua plugin/ })).toHaveCount(0)
    expect((await request.delete(base + '/admin/extensions/lua-editor', { headers })).ok()).toBeTruthy()
    await stop()
    base = await start()
    const after = await (await request.get(base + '/admin/extensions', { headers })).json()
    expect(after.extensions).toEqual([])
    expect((await (await request.get(base + '/admin/extensions/catalog', { headers })).json()).packages[0].status).toBe('removed')
    await page.goto(base + '/extensions')
    const card = page.locator('[data-extension="lua-editor"]')
    await expect(card.getByText('Built-in', { exact: true })).toBeVisible()
    await expect(card.getByText('Not installed', { exact: true })).toBeVisible()
    await expect(card.locator('.t-link').filter({ hasText: /^Install$/ })).toBeVisible()
    expect(errors).toEqual([])
  } finally {
    await stop()
    await new Promise<void>(resolve => updates.close(() => resolve()))
    await rm(dir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
  }
})
