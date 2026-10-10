<template>
  <ExtensionUI :title="localized(state?.manifest.label) || state?.manifest.name || ''" :page="uiPage" :drawer="uiDrawer" @event="uiEvent" @back="back">
    <div class="extension-content">
      <t-alert v-if="error" theme="error" :message="error" />
      <iframe v-if="state && entry" ref="frame" :key="state.hash" :title="localized(state.manifest.label) || state.manifest.name"
        :src="extensionApi.asset(state, entry)" sandbox="allow-scripts" referrerpolicy="no-referrer" @load="initialize" />
      <span class="extension-status" role="status">{{ uiText(notice, locale) }}</span>
    </div>
  </ExtensionUI>
  <c-drawer v-model:visible="leaveVisible" :header="t('extensions.leave')" :confirm-btn="{ content: t('extensions.leave') }"
    :cancel-btn="{ content: t('common.cancel') }" @confirm="finishLeave(true)" @cancel="finishLeave(false)">
    <t-alert theme="warning" :message="t('extensions.unsaved')" />
  </c-drawer>
</template>

<script setup lang="ts">
// 每个沙箱独立会话；UI 描述与业务动作分别校验，不向扩展传入凭据或宿主对象。
import { ref, shallowRef, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { useTheme } from '@/composables/useTheme'
import { useLocalizedText } from '@/composables/useLocale'
import { useExtensionEnvironment } from '@/composables/useExtensionEnvironment'
import { CDrawer, ExtensionUI } from '@/components'
import { useDialog } from '@/composables/useDialogVisible'
import { extensionApi, type ExtensionState } from '@/api/extensions'
import { currentConnection, onConnectionReset } from '@/api/connections'
import { extensionUICapabilities, parseUICommand, uiText } from '@/features/extensionUI'
import type { ContributionLocation, LocalizedText, SettingsValues, UIDrawer, UIEvent, UIPage } from '../../../../sdk/extension/ui'

const route = useRoute(), router = useRouter(), { t, locale } = useI18n(), { dark } = useTheme(), localized = useLocalizedText()
const { environment, acceptsUI } = useExtensionEnvironment()
const frame = ref<HTMLIFrameElement>(), state = ref<ExtensionState>(), entry = ref(''), error = ref('')
const uiPage = ref<UIPage>(), uiDrawer = ref<UIDrawer>(), notice = ref<LocalizedText>('')
const settings = shallowRef<SettingsValues>({})
const { visible: leaveVisible } = useDialog()
const calls = new Map<number, AbortController>()
const name = typeof route.query.name === 'string' && /^[A-Za-z0-9_-]{1,32}$/.test(route.query.name) ? route.query.name : ''
let nonce = crypto.randomUUID(), dirty = false, stopped = false, activated = false, checking = false, generation = 0
let timer: ReturnType<typeof setInterval> | undefined
let leaveDecision: Promise<boolean> | undefined, resolveLeave: ((allow: boolean) => void) | undefined
let draftKey = ''

function post(message: Record<string, unknown>) { frame.value?.contentWindow?.postMessage({ ...message, nonce }, '*') }
function clearUI() { uiPage.value = undefined; uiDrawer.value = undefined; notice.value = ''; activated = false }
function revoke() {
  generation++; nonce = crypto.randomUUID()
  for (const controller of calls.values()) controller.abort()
  calls.clear(); clearUI(); state.value = undefined; entry.value = ''; settings.value = {}
}
const reset = onConnectionReset(revoke)

async function check() {
  if (checking) return
  checking = true
  const pendingGeneration = generation
  try {
    const response = await extensionApi.list()
    if (stopped || pendingGeneration !== generation) return
    const next = response.extensions.find(item => item.manifest.id === route.params.extensionId && item.available)
    const page = next?.manifest.pages?.find(item => item.id === route.params.pageId)
    if (!next || !page) throw new Error(t('extensions.unavailable'))
    if (!acceptsUI(next.manifest)) throw new Error(t('extensions.unsupportedEnvironment'))
    const config = next.manifest.settings?.length ? await extensionApi.settings(next.manifest.id) : undefined
    if (stopped || pendingGeneration !== generation) return
    if (!acceptsUI(next.manifest)) throw new Error(t('extensions.unsupportedEnvironment'))
    if (config && config.hash !== next.hash) return
    if (state.value && state.value.hash !== next.hash) revoke()
    if (JSON.stringify(settings.value) !== JSON.stringify(config?.values || {})) settings.value = config?.values || {}
    state.value = next; entry.value = page.entry; error.value = ''
  } catch (failure) {
    if (!stopped && pendingGeneration === generation) {
      revoke(); error.value = failure instanceof Error ? failure.message : t('extensions.unavailable')
    }
  } finally { checking = false }
}
function initialize() {
  if (!state.value || !acceptsUI(state.value.manifest)) return
  nonce = crypto.randomUUID(); clearUI()
  for (const controller of calls.values()) controller.abort()
  calls.clear()
  draftKey = ['cph-extension-draft', currentConnection().id, route.params.extensionId, route.params.pageId, name].join(':')
  let saved: unknown
  try { saved = JSON.parse(sessionStorage.getItem(draftKey) || 'null') } catch { /* 不恢复损坏的草稿。 */ }
  post({ type: 'cph:init', context: { name, locale: locale.value, dark: dark.value, environment: environment.value, settings: settings.value, draft: saved, ui: extensionUICapabilities } })
}
function activate() {
  if (activated || !state.value) return
  activated = true
  const contribution = state.value.manifest.contributions?.find(item =>
    item.id === route.query.contribution && item.page === route.params.pageId &&
    extensionUICapabilities.locations.includes(item.location as ContributionLocation))
  const event: UIEvent = { kind: 'activate', page: String(route.params.pageId), context: name ? { name } : {} }
  if (contribution) event.contribution = { id: contribution.id, location: contribution.location as ContributionLocation }
  post({ type: 'cph:ui-event', event })
}
function uiEvent(event: UIEvent) {
  if (!state.value || !activated) return
  if (event.kind === 'action' && !uiPage.value?.actions.some(action => action.id === event.id && !action.disabled && !action.loading)) return
  if ((event.kind === 'submit' || event.kind === 'close') && uiDrawer.value?.id !== event.id) return
  if (event.kind === 'close') uiDrawer.value = undefined
  post({ type: 'cph:ui-event', event })
}
function back() { void router.push(route.query.from === 'plugins' ? '/plugins' : '/extensions') }
watch(environment, () => {
  if (state.value && !acceptsUI(state.value.manifest)) {
    revoke(); error.value = t('extensions.unsupportedEnvironment')
  } else if (!state.value) void check()
}, { flush: 'sync' })
watch([locale, dark, settings, environment], () => {
  if (state.value) post({ type: 'cph:context', context: { locale: locale.value, dark: dark.value, environment: environment.value, settings: settings.value } })
})

async function message(event: MessageEvent) {
  if (!state.value || !acceptsUI(state.value.manifest) || event.source !== frame.value?.contentWindow || event.origin !== 'null') return
  const data = event.data
  if (!data || typeof data !== 'object' || data.nonce !== nonce) return
  if (data.type === 'cph:ready') { activate(); return }
  if (data.type === 'cph:back') { back(); return }
  if (data.type === 'cph:cancel' && Number.isSafeInteger(data.id)) { calls.get(data.id)?.abort(); return }
  if (data.type === 'cph:draft') {
    if (typeof data.content !== 'string' || data.content.length > 2 * 1024 * 1024 || typeof data.dirty !== 'boolean' ||
      typeof data.name !== 'string' || data.name.length > 32) return
    dirty = data.dirty
    try {
      if (dirty) sessionStorage.setItem(draftKey, JSON.stringify({ content: data.content, name: data.name, dirty }))
      else sessionStorage.removeItem(draftKey)
    } catch { /* 存储受限时仍保留当前页面内容和离开确认。 */ }
    return
  }
  if (!Number.isSafeInteger(data.id) || data.id < 1 || calls.has(data.id)) return
  if (data.type === 'cph:ui') {
    try {
      const command = parseUICommand(data.command)
      if (command.kind === 'page') uiPage.value = command.page
      else if (command.kind === 'drawer') uiDrawer.value = command.drawer || undefined
      else {
        notice.value = command.message
        void MessagePlugin[command.level](uiText(command.message, locale.value))
      }
      post({ type: 'cph:result', id: data.id, result: null })
    } catch (failure) {
      post({ type: 'cph:result', id: data.id, error: failure instanceof Error ? failure.message : 'Invalid extension UI descriptor' })
    }
    return
  }
  if (data.type !== 'cph:invoke' || calls.size >= 8 || typeof data.action !== 'string') return
  if (!state.value.manifest.actions?.some(action => action.id === data.action)) return
  try { if (JSON.stringify(data.input)?.length > 2 * 1024 * 1024) return } catch { return }
  const session = nonce, target = frame.value?.contentWindow, controller = new AbortController()
  calls.set(data.id, controller)
  let result: unknown, failure: string | undefined
  try { result = await extensionApi.invoke(state.value.manifest.id + '.' + data.action, data.input, controller.signal) }
  catch (error) { failure = error instanceof Error ? error.message : String(error) }
  finally { if (calls.get(data.id) === controller) calls.delete(data.id) }
  if (session === nonce) target?.postMessage({ type: 'cph:result', nonce: session, id: data.id, result, error: failure }, '*')
}
function finishLeave(allow: boolean) {
  leaveVisible.value = false
  const resolve = resolveLeave; resolveLeave = undefined; leaveDecision = undefined; resolve?.(allow)
}
watch(leaveVisible, visible => { if (!visible && resolveLeave) finishLeave(false) })
function beforeUnload(event: BeforeUnloadEvent) { if (dirty) { event.preventDefault(); event.returnValue = '' } }
function canLeave() {
  if (!dirty) return true
  if (!leaveDecision) { leaveDecision = new Promise(resolve => { resolveLeave = resolve }); leaveVisible.value = true }
  return leaveDecision
}
onBeforeRouteLeave(canLeave)
onBeforeRouteUpdate(canLeave)
onMounted(() => {
  window.addEventListener('message', message); window.addEventListener('beforeunload', beforeUnload)
  void check(); timer = setInterval(check, 3000)
})
onBeforeUnmount(() => {
  finishLeave(false); stopped = true; reset(); revoke(); clearInterval(timer)
  window.removeEventListener('message', message); window.removeEventListener('beforeunload', beforeUnload)
})
</script>

<style scoped>
.extension-content { height: 100%; min-height: 0; display: flex; flex-direction: column; overflow: hidden; }
.extension-content iframe { width: 100%; height: 100%; flex: 1; min-height: 0; border: 0; }
.extension-status { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
</style>
