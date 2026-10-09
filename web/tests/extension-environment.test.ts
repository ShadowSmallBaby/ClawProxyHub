import { test } from 'node:test'
import assert from 'node:assert/strict'
import { extensionEnvironment, supportsExtensionInstallation, supportsExtensionUI } from '../src/features/extensionEnvironment.ts'
import type { ExtensionEnvironment } from '../../sdk/extension/ui'

const environments: ExtensionEnvironment[] = ['app', 'web-desktop', 'web-mobile']

test('App identity is independent of viewport size', () => {
  assert.equal(extensionEnvironment('app-full', false), 'app')
  assert.equal(extensionEnvironment('app-full', true), 'app')
  assert.equal(extensionEnvironment('web-full', false), 'web-desktop')
  assert.equal(extensionEnvironment('web-full', true), 'web-mobile')
})

test('Web extensions can be managed from both viewports while their UI stays scoped', () => {
  for (const supported of ['web-desktop', 'web-mobile'] as const) {
    const manifest = { environments: [supported] }
    for (const current of environments) {
      assert.equal(supportsExtensionInstallation(manifest, current), current !== 'app')
      assert.equal(supportsExtensionUI(manifest, current), current === supported)
    }
  }
})

test('App-only extensions cannot be installed or displayed in Web', () => {
  const manifest = { environments: ['app'] as const }
  for (const current of environments) {
    assert.equal(supportsExtensionInstallation(manifest, current), current === 'app')
    assert.equal(supportsExtensionUI(manifest, current), current === 'app')
  }
})

test('Universal packages support every interface; missing manifests grant no access', () => {
  for (const current of environments) {
    for (const manifest of [{}, { environments }]) {
      assert.equal(supportsExtensionInstallation(manifest, current), true)
      assert.equal(supportsExtensionUI(manifest, current), true)
    }
    assert.equal(supportsExtensionInstallation(undefined, current), false)
    assert.equal(supportsExtensionUI(undefined, current), false)
    assert.equal(supportsExtensionInstallation({ environments: [] }, current), false)
    assert.equal(supportsExtensionUI({ environments: [] }, current), false)
  }
})
