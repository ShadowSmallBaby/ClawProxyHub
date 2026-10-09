<template>
  <form-item v-for="field in fields" :key="field.id" :label="text(field.label)" :tip="text(field.hint)" :mark="field.required">
    <t-input v-if="field.kind === 'text'" :value="textValue(field.id)" :aria-label="text(field.label)"
      :readonly="field.readonly" :disabled="busy" :maxlength="field.max_length || 4096"
      @update:value="(value: unknown) => setValue(field.id, value)" />
    <t-textarea v-else-if="field.kind === 'textarea'" :value="textValue(field.id)" :aria-label="text(field.label)"
      :readonly="field.readonly" :disabled="busy" :maxlength="field.max_length || 4096" :autosize="{ minRows: 3, maxRows: 12 }"
      @update:value="(value: unknown) => setValue(field.id, value)" />
    <t-select v-else-if="field.kind === 'select'" :value="textValue(field.id) || undefined" :aria-label="text(field.label)"
      :disabled="busy || field.readonly" :options="field.options.map(option => ({ value: option.value, label: text(option.label) }))"
      @update:value="(value: unknown) => setValue(field.id, value)" />
    <t-switch v-else-if="field.kind === 'toggle'" :value="modelValue[field.id] === true" :aria-label="text(field.label)"
      :disabled="busy || field.readonly" @update:value="(value: unknown) => setValue(field.id, value)" />
    <template v-else-if="field.kind === 'image'">
      <button type="button" class="extension-image" :disabled="busy || field.readonly" :aria-label="text(field.label)"
        @click="imageInputs[field.id]?.click()" @dragover.prevent @drop.prevent="event => pickImage(field.id, event.dataTransfer?.files?.[0])">
        <img v-if="imageValue(field.id)" :src="imageURL(field.id)" alt="" />
        <image-add-icon v-else />
        <span>{{ imageValue(field.id)?.name || t('extensions.ui.upload') }}</span>
      </button>
      <input :ref="element => setImageInput(field.id, element)" type="file" accept="image/png,image/jpeg,image/webp" hidden
        :aria-label="text(field.label)" :disabled="busy || field.readonly" @change="event => imageChanged(field.id, event)">
    </template>
    <div v-if="errors[field.id]" class="extension-field-error" role="alert">{{ errors[field.id] }}</div>
  </form-item>
</template>

<script setup lang="ts">
// 扩展抽屉与持久化设置共用协议控件，扩展无需引用宿主组件。
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ImageAddIcon } from 'tdesign-icons-vue-next'
import { FormItem } from '@/components/base'
import { extensionUICapabilities, uiText } from '@/features/extensionUI'
import type { LocalizedText, UIField, UIImage, UIValue } from '../../../sdk/extension/ui'

const props = defineProps<{ fields: UIField[]; modelValue: Record<string, UIValue>; busy?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [values: Record<string, UIValue>] }>()
const { t, locale } = useI18n()
const text = (value?: LocalizedText) => uiText(value, locale.value)
const errors = ref<Record<string, string>>({})
const imageInputs: Record<string, HTMLInputElement> = Object.create(null)
const imageReads = new Map<string, symbol>()

function textValue(id: string) { return typeof props.modelValue[id] === 'string' ? props.modelValue[id] as string : '' }
function imageValue(id: string) { const value = props.modelValue[id]; return value && typeof value === 'object' ? value : undefined }
function imageURL(id: string) { const value = imageValue(id); return value ? `data:${value.mime};base64,${value.data}` : '' }
function setValue(id: string, value: unknown) {
  const field = props.fields.find(field => field.id === id)
  if (!field || field.readonly || props.busy) return
  if (typeof value === 'string' || typeof value === 'boolean') {
    emit('update:modelValue', { ...props.modelValue, [id]: value }); delete errors.value[id]
  }
}
function setImageInput(id: string, element: unknown) {
  if (element instanceof HTMLInputElement) imageInputs[id] = element
  else delete imageInputs[id]
}
function imageChanged(id: string, event: Event) {
  const input = event.target as HTMLInputElement
  pickImage(id, input.files?.[0]); input.value = ''
}
function pickImage(id: string, file?: File) {
  const field = props.fields.find(field => field.id === id)
  if (!file || props.busy || field?.kind !== 'image' || field.readonly) return
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > extensionUICapabilities.image_max_bytes) {
    errors.value[id] = t('extensions.ui.imageRule'); return
  }
  const ticket = Symbol(), reader = new FileReader()
  imageReads.set(id, ticket)
  reader.onload = () => {
    if (imageReads.get(id) !== ticket || props.busy || !props.fields.some(field => field.id === id && field.kind === 'image' && !field.readonly)) return
    const value = { name: file.name, mime: file.type as UIImage['mime'], data: String(reader.result).split(',')[1] || '' }
    emit('update:modelValue', { ...props.modelValue, [id]: value }); delete errors.value[id]
  }
  reader.onerror = () => { if (imageReads.get(id) === ticket) errors.value[id] = t('extensions.ui.imageRule') }
  reader.readAsDataURL(file)
}
function validate() {
  errors.value = {}
  const result: Record<string, UIValue> = {}
  for (const field of props.fields) {
    const value = field.readonly && field.kind !== 'image' ? field.value : props.modelValue[field.id]
    if (field.required && (value === undefined || typeof value === 'string' && !value.trim())) errors.value[field.id] = t('extensions.ui.required')
    if (value !== undefined) result[field.id] = typeof value === 'object' ? { ...value } : value
  }
  return Object.keys(errors.value).length ? undefined : result
}
onBeforeUnmount(() => imageReads.clear())
defineExpose({ validate })
</script>

<style scoped>
.extension-field-error { color: var(--td-error-color); font-size: 12px; margin-top: 6px; }
.extension-image { display: flex; align-items: center; gap: 10px; padding: 0 14px; width: 100%; height: 60px; border: 1px dashed var(--td-component-stroke); border-radius: 10px; cursor: pointer; color: var(--td-text-color-placeholder); background: transparent; font: inherit; text-align: left; transition: color 0.2s, border-color 0.2s; }
.extension-image:hover, .extension-image:focus-visible { border-color: var(--td-brand-color); color: var(--td-brand-color); }
.extension-image:disabled { cursor: default; }
.extension-image img { width: 40px; height: 40px; flex: none; object-fit: cover; border-radius: 8px; }
.extension-image .t-icon { font-size: 24px; flex: none; }
.extension-image span { font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
