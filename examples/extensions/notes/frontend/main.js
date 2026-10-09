import { ready, activate, invoke, ui, onUIEvent, onContext } from './cph.js'

const text = (zh, en) => ({ zh, en })
onContext(context => { document.body.dataset.dark = String(context.dark) })
async function refresh() {
  const { notes } = await invoke('list', {})
  const root = document.getElementById('notes')
  root.replaceChildren()
  for (const note of notes || []) {
    const item = document.createElement('article'); item.textContent = note.content; root.append(item)
  }
}
onUIEvent(async event => {
  try {
    if (event.kind === 'activate' || event.kind === 'action' && event.id === 'refresh') await refresh()
    if (event.kind === 'action' && event.id === 'new') await ui({ kind: 'drawer', drawer: {
      id: 'create', title: text('新笔记', 'New note'), fields: [
        { id: 'content', kind: 'textarea', label: text('内容', 'Content'), required: true, max_length: 4096 },
        { id: 'label', kind: 'text', label: text('标签', 'Label'), required: true, max_length: 80 },
      ], submit: { label: text('保存', 'Save'), intent: 'primary' },
    } })
    if (event.kind === 'submit' && event.id === 'create') {
      await invoke('create', event.values)
      await ui({ kind: 'drawer', drawer: null }); await refresh()
    }
  } catch (error) { await ui({ kind: 'notice', level: 'error', message: error.message }) }
})
void ready.then(async () => {
  await ui({ kind: 'page', page: { title: text('笔记', 'Notes'), actions: [
    { id: 'new', label: text('新建', 'New'), intent: 'primary' }, { id: 'refresh', label: text('刷新', 'Refresh') },
  ] } })
  activate()
})
