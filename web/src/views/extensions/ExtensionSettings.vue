<template>
  <c-drawer v-model:visible="visible" :header="title" class="extension-settings" :footer="editable ? undefined : false"
    :confirm-btn="{ content: t('common.save'), loading }" @confirm="save">
    <div class="settings-fields">
      <extension-fields v-if="config" ref="form" v-model="values" :fields="fields" :busy="loading" />
    </div>
  </c-drawer>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { CDrawer, ExtensionFields } from '@/components'
import { useAsync } from '@/composables/useAsync'
import { useDialog } from '@/composables/useDialogVisible'
import { useLocalizedText } from '@/composables/useLocale'
import { extensionApi, type ExtensionSettings, type ExtensionState } from '@/api/extensions'
import { onConnectionReset } from '@/api/connections'
import type { SettingsValues, UIField, UIValue } from '../../../../sdk/extension/ui'

const { t } = useI18n(), localized = useLocalizedText(), { run, loading } = useAsync(), { visible } = useDialog()
const state = ref<ExtensionState>(), config = ref<ExtensionSettings>(), values = ref<Record<string, UIValue>>({})
const form = ref<InstanceType<typeof ExtensionFields>>()
const title = computed(() => `${localized(state.value?.manifest.label) || state.value?.manifest.name || ''} · ${t('extensions.settings')}`)
const editable = computed(() => config.value?.fields.some(field => !field.readonly))
const fields = computed(() => (config.value?.fields || []).map(({ default: initial, ...field }) => ({
  ...field, value: values.value[field.id] ?? initial ?? (field.kind === 'toggle' ? false : ''),
})) as UIField[])
let generation = 0
const reset = onConnectionReset(() => { generation++; visible.value = false; config.value = undefined })

async function open(extension: ExtensionState) {
  const current = ++generation
  visible.value = false; config.value = undefined
  await run(async () => {
    const result = await extensionApi.settings(extension.manifest.id)
    if (current !== generation) return
    state.value = extension; config.value = result; values.value = { ...result.values }; visible.value = true
  })
}
async function save() {
  if (!state.value || !config.value || loading.value || !editable.value) return
  const result = form.value?.validate()
  if (!result) return
  const current = generation, id = state.value.manifest.id, hash = config.value.hash
  await run(async () => {
    await extensionApi.saveSettings(id, hash, result as SettingsValues)
    if (current !== generation) return
    visible.value = false; MessagePlugin.success(t('common.saved'))
  })
}
onBeforeUnmount(() => { generation++; reset() })
defineExpose({ open })
</script>

<style scoped>
.settings-fields { padding-top: 20px; }
</style>
