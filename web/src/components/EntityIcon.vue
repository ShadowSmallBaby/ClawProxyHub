<!-- EntityIcon — 实体图标：有 icon 显示图片，否则兜底首字母（插件卡片/市场/客户端选择复用）。 -->
<template>
  <div class="entity-icon" :style="{ width: size, height: size, fontSize }">
    <img v-if="source && !failed" :src="source" :alt="name" @error="failed = true" />
    <span v-else>{{ (name || '?').slice(0, 1) }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { resourceURL } from '@/api/client'

const props = withDefaults(defineProps<{
  icon?: string // 包内相对路径或 URL（空 = 兜底首字母）
  name: string // 实体名（取首字母兜底 + alt）
  size?: string
}>(), { size: '44px' })

const fontSize = computed(() => {
  const n = parseInt(props.size, 10)
  return `${Math.round(n * 0.45)}px`
})
const source = computed(() => resourceURL(props.icon || ''))
const failed = ref(false)
watch(source, () => { failed.value = false })
</script>

<style scoped>
.entity-icon {
  border-radius: 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  overflow: hidden;
  flex-shrink: 0;
}
.entity-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
