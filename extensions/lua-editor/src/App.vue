<template>
  <div class="editor-surface">
    <p v-if="fatal" class="editor-error" role="alert">{{ fatal }}</p>
    <code-editor v-if="initialized" v-model="content" height="100%" />
    <div v-if="initialized" class="editor-feedback" role="status" aria-live="polite">
      <div class="editor-summary">
        <span v-if="checking">{{ text(labels.checking) }}</span>
        <span v-else-if="analysisSource === content && analysis?.valid">{{ text(labels.validSyntax) }}</span>
        <span v-if="draftState === 'saving'">{{ text(labels.savingDraft) }}</span>
        <span v-else-if="dirty && draftState === 'saved'">{{ text(labels.savedDraft) }}</span>
      </div>
      <p v-for="diagnostic in analysisSource === content ? analysis?.diagnostics : []" :key="diagnostic.line" class="editor-problem">
        {{ text(labels.line) }} {{ diagnostic.line }}:{{ diagnostic.column }} · {{ diagnostic.message }}
      </p>
      <p v-if="analysisError" class="editor-problem">{{ analysisError }}</p>
      <p v-if="draftError" class="editor-problem">{{ text(labels.draftFailed) }} {{ draftError }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
// 编辑区留在沙箱；宿主渲染操作，Go 后端提供分析和可恢复草稿。
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import CodeEditor from './CodeEditor.vue'
import { ready, draft, appearance, ui, onUIEvent, activate } from './bridge'
import { editorApi } from './api'
import type { Analysis } from './api'
import type { HostContext, LocalizedText, UIDrawer, UIEvent, UIValue } from '../../../sdk/extension/ui'

const labels = {
  new: { zh: '新建 Lua 插件', en: 'New Lua plugin' },
  save: { zh: '保存', en: 'Save' },
  run: { zh: '运行', en: 'Run' },
  runHint: { zh: '运行已保存的源码', en: 'Run the saved source' },
  create: { zh: '新建插件', en: 'New plugin' },
  name: { zh: '插件名', en: 'Plugin name' },
  label: { zh: '显示名', en: 'Display name' },
  icon: { zh: '图标', en: 'Icon' },
  nameHint: { zh: '从源码中的 PLUGIN_NAME 提取。', en: 'Extracted from PLUGIN_NAME in the source.' },
  labelHint: { zh: '从源码中的 PLUGIN_LABEL_ZH / PLUGIN_LABEL_EN 提取。', en: 'Extracted from PLUGIN_LABEL_ZH / PLUGIN_LABEL_EN in the source.' },
  invalidName: { zh: '请在源码中填写有效的 PLUGIN_NAME（1–32 位字母、数字、连字符或下划线）。', en: 'Set a valid PLUGIN_NAME in the source (1–32 letters, digits, hyphens or underscores).' },
  saved: { zh: '已保存', en: 'Saved' },
  running: { zh: '已运行', en: 'Running' },
  checking: { zh: '检查语法…', en: 'Checking syntax…' },
  validSyntax: { zh: '语法检查通过', en: 'Syntax valid' },
  line: { zh: '行', en: 'Line' },
  savingDraft: { zh: '正在保存草稿…', en: 'Saving draft…' },
  savedDraft: { zh: '草稿已保存', en: 'Draft saved' },
  draftFailed: { zh: '草稿未保存：', en: 'Draft was not saved:' },
  recovered: { zh: '已恢复未保存的草稿。', en: 'Restored your unsaved draft.' },
  sourceChanged: { zh: '已恢复草稿，但已保存的源码也有更新；保存前请检查内容。', en: 'Draft restored. The saved source has also changed; review your draft before saving.' },
  unsupported: { zh: '当前宿主不支持此扩展需要的 UI 协议，请更新宿主。', en: 'Update the host to support this extension UI protocol.' },
}
const content = ref(''), name = ref(''), saved = ref(''), fatal = ref(''), initialized = ref(false)
const operation = ref(''), createSource = ref<string>(), createAnalysis = ref<Analysis>()
const analysis = ref<Analysis>(), analysisSource = ref(''), checking = ref(false), analysisError = ref('')
const draftState = ref(''), draftError = ref('')
const dirty = computed(() => content.value !== saved.value), busy = computed(() => operation.value !== '')
let context: HostContext | undefined
let baseHash = '', draftRevision = '', persistedContent: string | undefined, draftLoaded = false, disposed = false
let analysisTimer: ReturnType<typeof setTimeout> | undefined, draftTimer: ReturnType<typeof setTimeout> | undefined
let analysisCall: AbortController | undefined, draftQueue = Promise.resolve()
const validName = (value: string) => /^[A-Za-z0-9_-]{1,32}$/.test(value)
const message = (error: unknown) => error instanceof Error ? error.message : String(error)
const text = (value: { zh: string; en: string }) => appearance.value.locale.startsWith('en') ? value.en : value.zh

function displayName(value: Analysis) {
  const { zh, en } = value.labels
  return (appearance.value.locale.startsWith('en') ? en : zh) || zh || en || value.name
}
function failure(error: unknown) { if (!disposed) fatal.value = message(error) }
function describePage() {
  if (!context?.ui || disposed) return
  void ui({ kind: 'page', page: {
    title: name.value ? { zh: '编辑 · ' + name.value, en: 'Edit · ' + name.value } : labels.new,
    subtitle: 'main.lua',
    actions: [
      { id: 'save', label: labels.save, intent: 'primary', loading: operation.value === 'save', disabled: busy.value || !initialized.value },
      { id: 'run', label: labels.run, hint: labels.runHint, loading: operation.value === 'run', disabled: busy.value || !name.value || dirty.value },
    ],
  } }).catch(failure)
}
function describeDrawer() {
  if (!context?.ui || createSource.value === undefined || !createAnalysis.value || disposed) return
  const metadata = createAnalysis.value
  const drawer: UIDrawer = {
    id: 'create', title: labels.create,
    submit: { label: labels.save, intent: 'primary', loading: operation.value === 'save', disabled: busy.value },
    fields: [
      { id: 'name', kind: 'text', label: labels.name, hint: labels.nameHint, value: metadata.name, required: true, readonly: true },
      { id: 'label', kind: 'text', label: labels.label, hint: labels.labelHint, value: displayName(metadata), readonly: true },
      { id: 'icon', kind: 'image', label: labels.icon },
    ],
  }
  void ui({ kind: 'drawer', drawer }).catch(failure)
}
async function notify(value: LocalizedText, level: 'success' | 'error' | 'warning' = 'success') {
  if (!disposed) await ui({ kind: 'notice', level, message: value }).catch(failure)
}
async function perform(kind: string, action: () => Promise<void>) {
  if (busy.value || disposed) return
  clearTimeout(draftTimer)
  operation.value = kind
  try { await draftQueue; await action() }
  catch (error) { await notify(message(error), 'error') }
  finally { operation.value = '' }
}

function scheduleAnalysis() {
  clearTimeout(analysisTimer)
  analysisCall?.abort()
  if (!initialized.value || disposed) return
  const snapshot = content.value
  if (analysis.value && analysisSource.value === snapshot) { checking.value = false; return }
  checking.value = true
  analysisError.value = ''
  analysisTimer = setTimeout(async () => {
    const call = new AbortController()
    analysisCall = call
    try {
      const result = await editorApi.analyze(snapshot, call.signal)
      if (!call.signal.aborted && content.value === snapshot && !disposed) {
        analysis.value = result; analysisSource.value = snapshot
      }
    } catch (error) {
      if (!call.signal.aborted && !disposed) analysisError.value = message(error)
    } finally {
      if (analysisCall === call && !disposed) checking.value = false
    }
  }, 400)
}
async function analyzeSnapshot(snapshot: string) {
  return analysis.value && analysisSource.value === snapshot ? analysis.value : editorApi.analyze(snapshot)
}
function scheduleDraft() {
  clearTimeout(draftTimer)
  if (!initialized.value || !draftLoaded || busy.value || disposed) return
  draft(content.value, name.value, dirty.value)
  draftState.value = content.value === persistedContent ? 'saved' : ''
  draftTimer = setTimeout(() => {
    const snapshot = { name: name.value, content: content.value, saved: saved.value, base_sha256: baseHash }
    draftQueue = draftQueue.then(async () => {
      if (disposed || snapshot.name !== name.value) return
      if (snapshot.content === persistedContent && snapshot.content !== snapshot.saved) return
      draftState.value = 'saving'
      try {
        if (snapshot.content === snapshot.saved) {
          if (draftRevision) await editorApi.deleteDraft(snapshot.name, draftRevision)
          draftRevision = ''; persistedContent = undefined
        } else {
          const result = await editorApi.saveDraft({ name: snapshot.name, content: snapshot.content, base_sha256: snapshot.base_sha256, revision: draftRevision })
          draftRevision = result.revision; persistedContent = snapshot.content
        }
        draftError.value = ''
        draftState.value = content.value === snapshot.content ? 'saved' : ''
      } catch (error) {
        draftError.value = message(error); draftState.value = ''
      }
    })
  }, 750)
}
async function clearDraft(targetName: string) {
  try {
    if (draftRevision) await editorApi.deleteDraft(targetName, draftRevision)
    draftError.value = ''
  } catch (error) {
    draftError.value = message(error)
    if (targetName === name.value) return
  }
  draftRevision = ''; persistedContent = undefined; draftState.value = ''
}
async function savedSource(snapshot: string, metadata: Analysis, previousName = name.value) {
  saved.value = snapshot; baseHash = metadata.sha256
  await clearDraft(previousName)
  draft(content.value, name.value, dirty.value)
  await notify(labels.saved)
}
function save() {
  if (busy.value || !initialized.value) return
  const snapshot = content.value
  void perform('save', async () => {
    const metadata = await analyzeSnapshot(snapshot)
    if (!name.value) {
      if (!metadata.valid_name) { await notify(labels.invalidName, 'warning'); return }
      createAnalysis.value = metadata; createSource.value = snapshot
      return
    }
    await editorApi.save(name.value, snapshot)
    await savedSource(snapshot, metadata)
  })
}
function create(values: Record<string, UIValue>) {
  if (busy.value || createSource.value === undefined || !createAnalysis.value) return
  const snapshot = createSource.value, metadata = createAnalysis.value
  const icon = values.icon && typeof values.icon === 'object' ? values.icon.data : ''
  void perform('save', async () => {
    const previousName = name.value
    const result = await editorApi.create({ name: metadata.name, label: displayName(metadata), content: snapshot, icon })
    name.value = result.name; createSource.value = undefined; createAnalysis.value = undefined
    await ui({ kind: 'drawer', drawer: null })
    await savedSource(snapshot, metadata, previousName)
  })
}
function execute() {
  if (busy.value || !name.value || dirty.value) return
  void perform('run', async () => { await editorApi.run(name.value); await notify(labels.running) })
}
function handleEvent(event: UIEvent) {
  if (event.kind === 'activate') {
    if (initialized.value || busy.value) return
    void perform('load', async () => {
      const restored = context?.draft
      const targetName = event.context.name || (restored?.dirty && validName(restored.name) ? restored.name : '')
      name.value = targetName
      saved.value = targetName ? (await editorApi.read(targetName)).content : (await editorApi.scaffold()).lua
      const metadata = await editorApi.analyze(saved.value)
      baseHash = metadata.sha256
      let recovery: Awaited<ReturnType<typeof editorApi.readDraft>> | undefined
      try { recovery = await editorApi.readDraft(targetName); draftLoaded = true }
      catch (error) { draftError.value = message(error) }
      draftRevision = recovery?.draft?.revision || ''
      persistedContent = recovery?.draft?.content
      const local = restored?.dirty && restored.name === targetName ? restored : undefined
      content.value = local?.content ?? recovery?.draft?.content ?? saved.value
      analysis.value = metadata; analysisSource.value = saved.value
      initialized.value = true
      if (dirty.value && recovery?.draft) {
        await notify(recovery.draft.base_sha256 !== baseHash ? labels.sourceChanged : labels.recovered, 'warning')
      }
    })
  } else if (event.kind === 'action') {
    if (event.id === 'save') save()
    else if (event.id === 'run') execute()
  } else if (event.kind === 'submit' && event.id === 'create') create(event.values)
  else if (event.kind === 'close' && event.id === 'create') { createSource.value = undefined; createAnalysis.value = undefined }
}

watch(appearance, value => {
  document.documentElement.lang = value.locale
  document.documentElement.setAttribute('theme-mode', value.dark ? 'dark' : 'light')
  describeDrawer()
}, { immediate: true })
watch([name, operation, dirty, initialized], describePage)
watch([createSource, operation], describeDrawer)
watch([content, initialized], scheduleAnalysis)
watch([content, name, saved, operation, initialized], () => {
  if (initialized.value) draft(content.value, name.value, dirty.value)
  scheduleDraft()
})
const unsubscribe = onUIEvent(handleEvent)
onBeforeUnmount(() => {
  disposed = true; unsubscribe(); clearTimeout(analysisTimer); clearTimeout(draftTimer); analysisCall?.abort()
})
void ready.then(value => {
  context = value
  if (value.ui?.version !== 1 || !(['page', 'drawer', 'notice'] as const).every(surface => value.ui!.surfaces.includes(surface)) ||
    !(['text', 'image'] as const).every(field => value.ui!.fields.includes(field))) {
    fatal.value = text(labels.unsupported)
    return
  }
  describePage()
  activate()
}).catch(failure)
</script>

<style>
html, body, #app { height: 100%; margin: 0; overflow: hidden; }
body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; }
.editor-surface { height: 100%; display: flex; flex-direction: column; min-height: 0; }
.editor-surface > .code-editor { flex: 1; min-height: 0; border: 0; border-radius: 0; }
.editor-error { margin: 0; padding: 16px; color: #9e3232; background: #fff; }
.editor-feedback { padding: 8px 12px; font-size: 12px; color: #53627e; background: #fff; border-top: 1px solid rgba(31, 61, 156, .12); overflow: auto; max-height: 30%; }
.editor-summary { display: flex; gap: 16px; flex-wrap: wrap; }
.editor-problem { margin: 4px 0 0; color: #9e3232; overflow-wrap: anywhere; }
[theme-mode="dark"] .editor-error, [theme-mode="dark"] .editor-problem { color: #fca5a5; }
[theme-mode="dark"] .editor-error, [theme-mode="dark"] .editor-feedback { background: #1a2440; }
[theme-mode="dark"] .editor-feedback { color: #acb6cb; border-top-color: rgba(255, 255, 255, .1); }
</style>
