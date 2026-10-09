<template>
  <page-layout :scroll="false" class="extension-ui">
    <template #header>
      <div class="extension-topbar">
        <t-button variant="text" shape="square" :aria-label="t('extensions.ui.back')" @click="emit('back')">
          <template #icon><arrow-left-icon /></template>
        </t-button>
        <span class="extension-title">{{ text(page?.title) || title }}</span>
        <span v-if="page?.subtitle" class="extension-subtitle">{{ text(page.subtitle) }}</span>
        <t-space class="extension-toolbar">
          <t-tooltip v-for="action in page?.actions || []" :key="action.id" :content="text(action.hint)" :disabled="!action.hint">
            <t-button :theme="action.intent || 'default'" :variant="action.intent === 'primary' ? 'base' : 'outline'"
              :loading="action.loading" :disabled="action.disabled" @click="activate(action)">
              {{ text(action.label) }}
            </t-button>
          </t-tooltip>
        </t-space>
      </div>
    </template>
    <slot />
    <template #overlays>
      <c-drawer :visible="!!drawer" :header="text(drawer?.title)" class="extension-form"
        :confirm-btn="{ content: text(drawer?.submit.label), theme: drawer?.submit.intent || 'primary', loading: busy, disabled: drawer?.submit.disabled }"
        :cancel-btn="{ content: t('common.cancel'), loading: drawer?.submit.loading }"
        @update:visible="value => { if (!value) close() }" @cancel="close" @confirm="submit">
        <extension-fields v-if="drawer" :key="drawer.id" ref="form" v-model="values" :fields="drawer.fields" :busy="busy" />
      </c-drawer>
    </template>
  </page-layout>
</template>

<script setup lang="ts">
// 标准控件由宿主渲染；用户操作仅回传事件，业务动作仍须经过扩展授权桥。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowLeftIcon } from 'tdesign-icons-vue-next'
import { PageLayout, CDrawer, ExtensionFields } from '@/components'
import { uiText } from '@/features/extensionUI'
import type { LocalizedText, UIAction, UIDrawer, UIEvent, UIPage, UIValue } from '../../../sdk/extension/ui'

const props = defineProps<{ title: string; page?: UIPage; drawer?: UIDrawer }>()
const emit = defineEmits<{ event: [event: UIEvent]; back: [] }>()
const { t, locale } = useI18n()
const text = (value?: LocalizedText) => uiText(value, locale.value)
const values = ref<Record<string, UIValue>>({}), submitted = ref(false)
const form = ref<InstanceType<typeof ExtensionFields>>()
const busy = computed(() => submitted.value || !!props.drawer?.submit.loading)

watch(() => props.drawer, (next, previous) => {
  submitted.value = false
  if (next?.id !== previous?.id) {
    values.value = {}
  }
  for (const field of next?.fields || []) {
    if (field.kind !== 'image' && (field.readonly || values.value[field.id] === undefined)) {
      values.value[field.id] = field.value ?? (field.kind === 'toggle' ? false : '')
    }
  }
}, { immediate: true })

function activate(action: UIAction) {
  if (!action.disabled && !action.loading) emit('event', { kind: 'action', id: action.id })
}
function close() {
  if (props.drawer && !props.drawer.submit.loading) emit('event', { kind: 'close', id: props.drawer.id })
}
function submit() {
  const drawer = props.drawer
  if (!drawer || busy.value || drawer.submit.disabled) return
  const result = form.value?.validate()
  if (!result) return
  submitted.value = true
  emit('event', { kind: 'submit', id: drawer.id, values: result })
}
</script>

<style scoped>
.extension-ui { height: 100%; padding: 0; }
.extension-topbar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding: 10px 16px; border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-container); }
.extension-title { font-weight: 600; overflow-wrap: anywhere; }
.extension-subtitle { font-size: 12px; color: var(--td-text-color-secondary); font-family: ui-monospace, monospace; }
.extension-toolbar { margin-left: auto; }
@media (max-width: 767px) {
  .extension-topbar { padding: 10px 12px; gap: 8px; }
  .extension-title { flex: 1; min-width: 0; }
  .extension-subtitle { display: none; }
}
</style>
