import { test } from 'node:test'
import assert from 'node:assert/strict'
import { extensionCards } from '../src/features/extensionCatalog.ts'
import { extractTrustCode, parseTrustJSON } from '../src/features/extensionTrust.ts'
import type { ExtensionManifest, ExtensionState, PackageEntry } from '../src/api/extensions.ts'

const manifest: ExtensionManifest = { id: 'example', name: 'Example', version: '0.1.0', kind: 'data', target: 'backend', activation: 'hot', permissions: [] }
const bundled: PackageEntry = { path: 'data/packages/example.cphext', manifest, sha256: 'a'.repeat(64), status: 'available', enabled: false, available: false }
const installed: ExtensionState = { manifest, hash: 'a'.repeat(64), signer: 'publisher-key', publisher: 'Publisher', status: 'disabled', enabled: false, available: false, bytes: 42 }
const config = { public_key: Buffer.alloc(32, 1).toString('base64'), publisher: 'Publisher', ids: ['example'], permissions: [] }

test('bundled cards remain visible before installation and after removal, without duplicating installed extensions', () => {
  const before = extensionCards([], [bundled])
  assert.equal(before.length, 1)
  assert.equal(before[0]?.state, undefined)
  const active = extensionCards([installed], [{ ...bundled, status: 'installed' }])
  assert.equal(active.length, 1)
  assert.equal(active[0]?.state, installed)
  assert.ok(active[0]?.bundled)
  const removed = extensionCards([], [{ ...bundled, status: 'removed' }])
  assert.equal(removed[0]?.id, 'example')
  assert.equal(removed[0]?.state, undefined)
  assert.ok(removed[0]?.bundled)
})

test('catalog selects a compatible update while preserving the installed version and showing packages with errors', () => {
  const update = { ...bundled, manifest: { ...manifest, version: '0.2.0' }, status: 'update' }
  const unsupported = { ...bundled, manifest: { ...manifest, version: '0.3.0' }, status: 'incompatible' }
  const cards = extensionCards([installed], [unsupported, bundled, update])
  assert.equal(cards.length, 1)
  assert.equal(cards[0]?.version, '0.1.0')
  assert.equal(cards[0]?.bundled?.manifest?.version, '0.2.0')
  assert.equal(extensionCards([], [{ ...bundled, manifest: undefined, status: 'invalid' }]).length, 1)
})

test('signing JSON extracts its single identity and accepts explicit empty permission grants', () => {
  assert.deepEqual(parseTrustJSON(JSON.stringify({ key_id: 'publisher-key', ...config })), { code: 'publisher-key', config })
  assert.deepEqual(parseTrustJSON(JSON.stringify({ 'publisher-key': config })), { code: 'publisher-key', config })
  assert.deepEqual(parseTrustJSON(JSON.stringify({ key_id: config })), { code: 'key_id', config })
  assert.equal(extractTrustCode(JSON.stringify({ key_id: 'publisher-key', public_key: '' })), 'publisher-key')
  assert.equal(extractTrustCode(JSON.stringify({ 'publisher-key': config })), 'publisher-key')
})

test('signing JSON rejects malformed identities, missing fields and invalid key or grant types', () => {
  for (const value of ['{', 'null', '[]', '{}', JSON.stringify({ one: config, two: config }), JSON.stringify(config)]) {
    assert.throws(() => parseTrustJSON(value))
  }
  for (const key_id of ['', 'Invalid ID', 1, null]) {
    assert.throws(() => parseTrustJSON(JSON.stringify({ key_id, ...config })))
  }
  for (const invalid of [
    { ...config, publisher: '' }, { ...config, public_key: 'not-a-key' },
    { ...config, public_key: undefined }, { ...config, certificate: 'YWJj' },
    { ...config, ids: [] }, { ...config, ids: ['example', 'example'] },
    { ...config, permissions: undefined }, { ...config, permissions: 'workspace.read' },
    { ...config, permissions: ['*'] }, { ...config, native: 'true' }, { ...config, private_key: 'unexpected' },
  ]) assert.throws(() => parseTrustJSON(JSON.stringify({ 'publisher-key': invalid })))
})
