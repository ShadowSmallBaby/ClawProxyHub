import { test, expect } from '@playwright/test'
import { execFileSync, spawn } from 'node:child_process'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { tmpdir } from 'node:os'
import { once } from 'node:events'

// 真实签名包验证非 Vue 页面、Go RPC、系统备份入口及旧表清理。
test('plain JavaScript extension persists through Go and cleans obsolete tables', async ({ page, request }) => {
  test.setTimeout(180000)
  page.setDefaultTimeout(15000)
  const repo = resolve('..'), dir = await mkdtemp(join(tmpdir(), 'cph-go-extension-'))
  const packages = join(dir, 'packages'), source = join(dir, 'upgrade')
  const binary = join(dir, process.platform === 'win32' ? 'cph.exe' : 'cph')
  const python = process.env.CPH_PYTHON || (process.platform === 'win32' ? 'python' : 'python3')
  let core: ReturnType<typeof spawn> | undefined
  const stop = async () => {
    if (core && core.exitCode === null) {
      const exited = once(core, 'exit')
      core.kill()
      await exited.catch(() => {})
    }
  }
  const start = async () => {
    core = spawn(binary, [], { cwd: dir, windowsHide: true, env: {
      ...process.env, CPH_DATA_DIR: dir, CPH_DATABASE_DSN: join(dir, 'cph.db'), CPH_PLUGIN_DIR: join(dir, 'plugins'),
      CPH_PACKAGE_DIRS: packages, CPH_EXTENSION_TRUST: join(packages, 'trust.json'), CPH_INSTALL_PACKAGES: 'true',
      CPH_ADDR: '127.0.0.1:0', CPH_PROFILE: 'full', CPH_ADMIN_USERNAME: 'admin', CPH_ADMIN_PASSWORD: 'extension-test-password',
      CPH_SECRET_KEY: '', CPH_SEED_API_KEY: '',
    } })
    let log = ''
    return new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('Core startup timed out: ' + log)), 30000)
      const output = (data: Buffer) => {
        log += data
        const match = /listening on (127\.0\.0\.1:\d+)/.exec(log)
        if (match) { clearTimeout(timer); resolve('http://' + match[1]) }
      }
      core!.stdout!.on('data', output)
      core!.stderr!.on('data', output)
      core!.once('error', error => { clearTimeout(timer); reject(error) })
      core!.once('exit', () => { clearTimeout(timer); reject(new Error(log)) })
    })
  }
  try {
    execFileSync(python, ['scripts/build-extension.py', '--project', 'examples/extensions/notes/build.json', '--out', packages, '--development-key'], { cwd: repo, windowsHide: true })
    execFileSync('go', ['build', '-o', binary, './cmd/cph'], { cwd: repo, windowsHide: true })
    let base = await start()
    const login = await request.post(base + '/admin/login', { data: { username: 'admin', password: 'extension-test-password' } })
    expect(login.ok()).toBeTruthy()
    const { token } = await login.json(), headers = { Authorization: `Bearer ${token}` }
    await page.addInitScript(token => {
      if (window.top !== window) return
      localStorage.setItem('cph-token:same-origin', token)
      localStorage.setItem('cph-locale', 'en')
      localStorage.setItem('cph-theme', 'light')
    }, token)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(base + '/plugins')
    await page.getByRole('button', { name: 'Notes', exact: true }).click()
    const frame = page.frameLocator('iframe')
    await expect(page.locator('iframe')).toHaveAttribute('sandbox', 'allow-scripts')
    await page.locator('.extension-topbar').getByRole('button', { name: 'New', exact: true }).click()
    const form = page.locator('.extension-form.t-drawer--open')
    await form.locator('.s-form-item').filter({ hasText: 'Content' }).locator('textarea').fill('Plain JavaScript talks to Go')
    await form.locator('.s-form-item').filter({ hasText: 'Label' }).locator('input').fill('integration')
    await form.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(form).toHaveCount(0)
    await expect(frame.locator('#notes article')).toHaveText('Plain JavaScript talks to Go')
    const isolation = await frame.locator('body').evaluate(() => {
      let storage = false, parent = false
      try { localStorage.getItem('cph-token:same-origin') } catch { storage = true }
      try { window.parent.document.body } catch { parent = true }
      return { storage, parent }
    })
    expect(isolation).toEqual({ storage: true, parent: true })
    await page.locator('button:has(.t-icon-moon)').click()
    await expect(frame.locator('body')).toHaveAttribute('data-dark', 'true')
    await mkdir(join(repo, 'build/verification'), { recursive: true })
    await page.screenshot({ path: join(repo, 'build/verification/extension-notes-dark.png'), animations: 'disabled' })
    await page.reload()
    await expect(frame.locator('#notes article')).toHaveText('Plain JavaScript talks to Go')
    const installed = (await (await request.get(base + '/admin/extensions', { headers })).json()).extensions[0]
    const platform = `${process.platform === 'win32' ? 'windows' : process.platform}/${process.arch === 'x64' ? 'amd64' : process.arch}`
    expect((await request.get(`${base}/extension-assets/sample-notes/${installed.hash}/${installed.manifest.backend.entries[platform]}`)).status()).toBe(404)

    await page.goto(base + '/settings')
    await page.locator('.settings-tabs .t-tabs__nav-item').filter({ hasText: /^System$/ }).click()
    const download = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Export Backup', exact: true }).click()
    expect(await (await download).failure()).toBeNull()

    // 仅变更测试清单，验证省略旧表后的宿主保留与清理；不调用旧的 create 处理器。
    execFileSync(python, ['-c', `import json, pathlib, sys, zipfile
source = pathlib.Path(sys.argv[2])
with zipfile.ZipFile(sys.argv[1]) as package:
    for name in package.namelist():
        if name != 'signature.json':
            package.extract(name, source)
path = source / 'manifest.json'
manifest = json.loads(path.read_text(encoding='utf-8'))
manifest['version'] = '0.2.0'
manifest['storage']['schema_version'] = 2
manifest['storage']['tables'] = [table for table in manifest['storage']['tables'] if table['name'] == 'notes']
path.write_text(json.dumps(manifest), encoding='utf-8')`, join(packages, 'sample-notes-0.1.0.cphext'), source], { cwd: repo, windowsHide: true })
    const upgrade = join(dir, 'upgrade.cphext')
    execFileSync('go', ['run', './cmd/cphext', 'pack', '--dir', source, '--out', upgrade, '--key-id', 'local-development', '--key', join(repo, '.cache/development-signing/extension.key')], { cwd: repo, windowsHide: true })
    const update = await request.post(base + '/admin/extensions/install', { headers, multipart: {
      package: { name: 'upgrade.cphext', mimeType: 'application/zip', buffer: await readFile(upgrade) },
      grants: JSON.stringify(['service.execute', 'storage.read', 'storage.write']),
    } })
    expect(update.ok(), await update.text()).toBeTruthy()
    await page.goto(base + '/extensions')
    await page.locator('[data-extension="sample-notes"]').getByText('Clean up', { exact: true }).click()
    const cleanup = page.locator('.c-drawer.t-drawer--open')
    const obsolete = cleanup.getByRole('checkbox', { name: 'Obsolete tables', exact: true })
    await expect(obsolete).toBeEnabled()
    await expect(cleanup.getByText('labels', { exact: true })).toBeVisible()
    await cleanup.getByText('Obsolete tables', { exact: true }).click()
    await expect(obsolete).toBeChecked()
    await expect(cleanup.getByText(/permanently deleted/)).toBeVisible()
    await page.screenshot({ path: join(repo, 'build/verification/extension-obsolete-tables.png'), animations: 'disabled' })
    await cleanup.getByRole('button', { name: 'Clean up', exact: true }).click()
    await expect(cleanup).toHaveCount(0)
    const remaining = await request.post(base + '/admin/actions/core.extensions.obsolete', { headers, data: { id: 'sample-notes' } })
    expect((await remaining.json()).tables).toEqual([])
    await stop()
    base = await start()
    await page.goto(base + '/plugins')
    await page.getByRole('button', { name: 'Notes', exact: true }).click()
    await expect(frame.locator('#notes article')).toHaveText('Plain JavaScript talks to Go')
    expect(errors).toEqual([])
  } finally {
    await stop()
    await rm(dir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
  }
})
