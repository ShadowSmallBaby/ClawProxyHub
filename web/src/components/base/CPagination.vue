<!-- CPagination — 分页二次封装：手机端自动精简（去页大小/跳页器，simple 主题），桌面保持完整。 -->
<template>
  <t-pagination
    v-if="total > 0"
    class="c-pagination"
    v-bind="$attrs"
    :total="total"
    :page-size-options="isPhone ? [] : pageSizeOptions"
    :show-jumper="!isPhone && showJumper"
    :theme="isPhone ? 'simple' : theme"
    :max-page-btn="5"
    :folded-max-page-btn="3"
  />
</template>

<script setup lang="ts">
import { useIsMobile } from '@/composables'

defineOptions({ inheritAttrs: false })

withDefaults(defineProps<{
  pageSizeOptions?: number[]
  showJumper?: boolean
  theme?: 'default' | 'simple'
  total?: number
}>(), {
  pageSizeOptions: () => [10, 30, 50, 100, 200],
  showJumper: true,
  theme: 'default',
  total: 0,
})

const { isPhone } = useIsMobile()
</script>

<style>
.c-pagination {
  margin-top: 12px;
  flex-shrink: 0;
}
</style>
