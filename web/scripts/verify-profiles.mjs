// Web 与 App 共享后端能力，仅 Web 发行预配编辑器。
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import assert from 'node:assert/strict'
const config = JSON.parse(await readFile(new URL('../../build-config.json', import.meta.url), 'utf8'))
for (const [profile, preset] of Object.entries(config.frontends)) {
  const dir = resolve('..', preset.out)
  const report = JSON.parse(await readFile(resolve(dir, 'profile.json'), 'utf8'))
  assert.equal(report.profile, profile)
  assert(!report.modules.some(id => /AndroidShell|ApplicationSettings|ConnectionPicker/.test(id)))
  const pluginPage = report.modules.some(id => /views\/plugins\/Plugins\.vue$/.test(id))
  assert.equal(pluginPage, preset.surface === 'web')
  assert.deepEqual(report.packages, preset.packages)
  for (const page of ['accounts/Accounts', 'tasks/Tasks', 'routes/Routes', 'keys/Keys', 'groups/Groups', 'logs/RunLogs']) {
    assert(report.modules.some(id => id.includes(`views/${page}.vue`)), `${profile}: missing ${page}`)
  }
  for (const editor of ['views/plugins/PluginEditor.vue', 'components/CodeEditor.vue']) {
    assert.equal(report.modules.some(id => id.endsWith(editor)), false)
  }
  console.log(`${profile}: ${report.modules.length} modules verified`)
}
