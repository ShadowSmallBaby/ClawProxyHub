<template>
  <c-drawer v-model:visible="visible" :header="t('extensions.trust')" :footer="false" width="760px">
    <t-loading :loading="loading" style="width: 100%">
      <div class="trust-grid">
        <button type="button" class="trust-add" :disabled="loading" @click="edit()">
          <span class="trust-add-icon" aria-hidden="true">＋</span>
          {{ t('extensions.addTrust') }}
        </button>
        <c-card v-for="identity in identities" :key="identity.code" class="trust-card" role="button" tabindex="0"
          :aria-label="identity.config.publisher" :aria-disabled="loading" @click="edit(identity)"
          @keydown.enter.prevent="edit(identity)" @keydown.space.prevent="edit(identity)">
          <div class="trust-name">{{ identity.config.publisher }}</div>
          <t-tag size="small" variant="light" :theme="identity.read_only ? 'primary' : 'default'">
            {{ t(identity.read_only ? 'extensions.fixedTrust' : 'extensions.managedTrust') }}
          </t-tag>
        </c-card>
      </div>
    </t-loading>
  </c-drawer>
  <c-drawer v-model:visible="formVisible" :header="formTitle" width="600px" :footer="draft?.readOnly ? false : undefined"
    :confirm-btn="{ loading, disabled: !!validationError, content: t('common.save') }" @confirm="save">
    <template v-if="draft">
      <form-item :label="t('extensions.trustConfig')" :mark="!draft.readOnly" stacked>
        <t-textarea v-model="draft.json" name="signing-json" :aria-label="t('extensions.trustConfig')" :readonly="draft.readOnly"
          :placeholder="example" :status="validationError ? 'error' : undefined"
          :autosize="{ minRows: 14, maxRows: 24 }" class="trust-config" />
      </form-item>
      <form-item :label="t('extensions.trustCode')" :tip="t('extensions.trustCodeHint')">
        <t-input :value="extractedCode" :aria-label="t('extensions.trustCode')" readonly :placeholder="t('extensions.trustCodeAuto')" />
      </form-item>
      <t-alert v-if="validationError" theme="error" :message="validationError" role="alert" />
      <t-button v-if="draft.code && !draft.readOnly" class="trust-remove" variant="outline" theme="danger" :disabled="loading"
        @click="removal = identities.find(identity => identity.code === draft?.code) || null">{{ t('common.delete') }}</t-button>
    </template>
  </c-drawer>
  <c-drawer v-model:visible="removalVisible" :header="t('common.delete')" width="480px"
    :confirm-btn="{ loading, theme: 'danger', content: t('common.delete') }" @confirm="remove">
    <p class="trust-name">{{ removal?.config.publisher }}</p>
    <t-alert theme="warning" :message="t('extensions.removeTrustHint')" />
  </c-drawer>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CCard, CDrawer, FormItem } from '@/components'
import { useAsync } from '@/composables/useAsync'
import { useDialog, useDialogVisible } from '@/composables/useDialogVisible'
import { extensionApi, type ExtensionTrust } from '@/api/extensions'
import { extractTrustCode, parseTrustJSON } from '@/features/extensionTrust'

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n(), { run, loading } = useAsync()
const { visible } = useDialog()
const identities = ref<ExtensionTrust[]>([])
const draft = ref<{ code: string; json: string; readOnly: boolean } | null>(null)
const formVisible = useDialogVisible(draft)
const removal = ref<ExtensionTrust | null>(null), removalVisible = useDialogVisible(removal)
const submitted = ref(false)
const example = JSON.stringify({ key_id: 'publisher-key', public_key: '<Base64 Ed25519>', publisher: 'Publisher', ids: ['example'], permissions: [] }, null, 2)
const formTitle = computed(() => t(draft.value?.code ? 'extensions.trustDetails' : 'extensions.addTrust'))
const parsed = computed(() => {
  try { return { entry: parseTrustJSON(draft.value?.json || ''), error: '' } }
  catch (error) { return { entry: undefined, error: t(error instanceof Error ? error.message : 'extensions.trustValidation.json') } }
})
const extractedCode = computed(() => extractTrustCode(draft.value?.json || ''))
const validationError = computed(() => {
  if (!draft.value || draft.value.readOnly || (!submitted.value && !draft.value.json.trim())) return ''
  if (parsed.value.error) return parsed.value.error
  const code = parsed.value.entry!.code
  if (draft.value.code && code !== draft.value.code) return t('extensions.trustValidation.rename')
  if (!draft.value.code && identities.value.some(identity => identity.code === code)) return t('extensions.trustValidation.duplicate')
  return ''
})

async function reload() { identities.value = (await extensionApi.trust()).identities || [] }
function open() { visible.value = true; void run(reload) }
function edit(entry?: ExtensionTrust) {
  if (loading.value) return
  submitted.value = false
  draft.value = { code: entry?.code || '', readOnly: entry?.read_only || false,
    json: entry ? JSON.stringify({ key_id: entry.code, ...entry.config }, null, 2) : '' }
}
async function save() {
  submitted.value = true
  if (!draft.value || draft.value.readOnly || validationError.value || !parsed.value.entry) return
  const { code, config } = parsed.value.entry
  await run(async () => {
    await extensionApi.saveTrust(code, config)
    draft.value = null
    await reload()
    emit('changed')
  })
}
async function remove() {
  const entry = removal.value
  if (!entry || entry.read_only) return
  await run(async () => { await extensionApi.deleteTrust(entry.code); removal.value = null; draft.value = null; await reload(); emit('changed') })
}
defineExpose({ open })
</script>

<style scoped>
.trust-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(210px, 100%), 1fr)); gap: 12px; }
.trust-card { min-height: 112px; cursor: pointer; }
.trust-card:focus-visible { outline: 1px solid var(--td-brand-color); outline-offset: 2px; }
.trust-card :deep(.t-card__body) { display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: 8px; height: 100%; box-sizing: border-box; }
.trust-name { font-weight: 600; overflow-wrap: anywhere; }
.trust-config :deep(textarea) { font-family: ui-monospace, monospace; }
.trust-remove { margin-top: 16px; }
.trust-add { min-height: 112px; border: 1px dashed var(--td-component-stroke); border-radius: 12px; background: transparent; color: var(--td-text-color-secondary); font: inherit; cursor: pointer; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; transition: color 0.2s, border-color 0.2s; }
.trust-add:hover, .trust-add:focus-visible { color: var(--td-brand-color); border-color: var(--td-brand-color); }
.trust-add-icon { font-size: 28px; }
</style>
