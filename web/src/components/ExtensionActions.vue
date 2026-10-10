<template>
  <template v-for="item in items" :key="item.key">
    <t-button v-if="location === 'plugins.toolbar'" variant="outline" :shape="isPhone ? 'circle' : undefined"
      :size="isPhone ? 'large' : 'medium'" :aria-label="title(item)" :title="isPhone ? title(item) : undefined" @click="open(item)">
      <template v-if="isPhone" #icon><add-icon /></template>
      <span v-if="!isPhone">{{ title(item) }}</span>
    </t-button>
    <t-link v-else theme="primary" @click="open(item)">{{ title(item) }}</t-link>
  </template>
</template>

<script setup lang="ts">
// 贡献入口复用所在工具栏或卡片的控件，文字由扩展提供并跟随当前语言。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { AddIcon } from 'tdesign-icons-vue-next'
import { useIsMobile } from '@/composables/useIsMobile'
import { useLocalizedText } from '@/composables/useLocale'
import { useExtensionEnvironment } from '@/composables/useExtensionEnvironment'
import { extensionStates } from '@/features/extensions'

const props = defineProps<{ location: 'plugins.toolbar' | 'plugins.item.actions'; name?: string; editable?: boolean }>()
const router = useRouter(), { isPhone } = useIsMobile(), localized = useLocalizedText()
const { acceptsUI } = useExtensionEnvironment()
const items = computed(() => extensionStates.value.filter(state => state.available && acceptsUI(state.manifest)).flatMap(state =>
  (state.manifest.contributions || []).filter(item => item.location === props.location && (!item.when || item.when === 'editable' && props.editable))
    .map(item => ({ ...item, key: state.manifest.id + '.' + item.id, extension: state.manifest.id }))
).sort((a, b) => (a.order || 0) - (b.order || 0)))
const title = (item: typeof items.value[number]) => localized(item.labels) || item.label

function open(item: typeof items.value[number]) {
  if (!items.value.some(current => current.key === item.key)) return
  router.push({ path: `/extensions/${item.extension}/${item.page}`, query: { from: 'plugins', contribution: item.id, ...(props.name ? { name: props.name } : {}) } })
}
</script>
