import { test } from 'node:test'
import assert from 'node:assert/strict'
import { parseUICommand, uiText } from '../src/features/extensionUI.ts'

const drawer = {
  kind: 'drawer', drawer: { id: 'create', title: { zh: '新建插件', en: 'New plugin' }, submit: { label: 'Save' }, fields: [
    { id: 'name', kind: 'text', label: 'Name', value: 'example', readonly: true, required: true },
    { id: 'notes', kind: 'textarea', label: 'Notes', max_length: 2000 },
    { id: 'scope', kind: 'select', label: 'Scope', options: [{ value: 'local', label: 'Local' }] },
    { id: 'enabled', kind: 'toggle', label: 'Enabled', value: false },
    { id: 'icon', kind: 'image', label: 'Icon' },
  ] },
}

test('UI protocol accepts declarative controls and resolves locale fallbacks', () => {
  assert.deepEqual(parseUICommand(drawer), drawer)
  assert.deepEqual(parseUICommand({ kind: 'drawer', drawer: null }), { kind: 'drawer', drawer: null })
  assert.equal(uiText(drawer.drawer.title, 'en-US'), 'New plugin')
  assert.equal(uiText({ fr: 'Nom' }, 'en'), 'Nom')
})

test('UI protocol rejects component injection, unsafe IDs and remote image sources', () => {
  const invalidFields = [
    { id: 'x', kind: 'component', component: 'CDrawer', label: 'Injected' },
    { id: 'x', kind: 'text', label: 'Injected', html: '<script />' },
    { id: '__proto__', kind: 'text', label: 'Injected' },
    { id: 'constructor', kind: 'text', label: 'Injected' },
    { id: 'x', kind: 'image', label: 'Injected', value: 'https://example.com/image' },
    { id: 'x', kind: 'text', label: { component: 'CDrawer' } },
    { id: 'x', kind: 'text', label: 'Injected', readonly: 'false' },
    { id: 'x', kind: 'select', label: 'Injected', options: [{ value: 'a', label: 'A' }], value: 'not-an-option' },
  ]
  for (const field of invalidFields) assert.throws(() => parseUICommand({ ...drawer, drawer: { ...drawer.drawer, fields: [field] } }))
  assert.throws(() => parseUICommand({ ...drawer, component: 'CDrawer' }))
  assert.throws(() => parseUICommand({ kind: 'modal', html: '<script />' }))
})

test('UI protocol bounds payloads and requires unique action and field identifiers', () => {
  assert.throws(() => parseUICommand({ kind: 'page', page: { title: 'x', actions: Array(2).fill({ id: 'save', label: 'Save' }) } }))
  assert.throws(() => parseUICommand({ ...drawer, drawer: { ...drawer.drawer, fields: Array(2).fill(drawer.drawer.fields[0]) } }))
  assert.throws(() => parseUICommand({ kind: 'notice', level: 'info', message: 'x'.repeat(65537) }))
  assert.throws(() => parseUICommand({ kind: 'notice', level: 'html', message: 'Unsafe' }))
  for (const value of [null, [], false, { kind: 'page', page: { title: [], actions: [] } }]) assert.throws(() => parseUICommand(value))
})
