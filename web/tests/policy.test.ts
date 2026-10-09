import { test } from 'node:test'
import assert from 'node:assert/strict'
import { availableCapabilities, permits, type Access } from '../src/features/policy.ts'

test('only installed, enabled and available modules contribute capabilities', () => {
  const state = { id: 'test', installed: true, enabled: true, available: true, capabilities: ['tasks'] }
  for (const field of ['installed', 'enabled', 'available']) {
    assert.deepEqual(availableCapabilities({ profile: 'full', modules: [{ ...state, [field]: false }] }), [])
  }
  assert.deepEqual(availableCapabilities({ profile: 'full', modules: [state, state] }), ['tasks'])
  assert.throws(() => availableCapabilities({} as any))
})

test('capabilities and permissions intersect; missing data cannot grant access', () => {
  const admin: Access = { username: 'admin', role: 'admin', menus: ['tasks', 'keys'], capabilities: ['tasks'], legacy: false }
  assert.equal(permits(admin, { permission: 'tasks', capability: 'tasks' }), true)
  assert.equal(permits(admin, { permission: 'keys', capability: 'keys' }), false)
  assert.equal(permits(admin, { permission: 'plugins', capability: 'tasks' }), false)
  assert.equal(permits(null, {}), false)
  assert.equal(permits(admin, {}), true)
  assert.equal(permits({ ...admin, role: 'guest' }, { permission: 'tasks', capability: 'tasks' }), false)
  assert.equal(permits({ ...admin, role: 'guest', menus: ['logs'], capabilities: ['logs'] }, { permission: 'logs', capability: 'logs' }), true)
})
