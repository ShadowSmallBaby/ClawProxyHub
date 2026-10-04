<!-- PageLayout — 页头和页脚固定，主体占满剩余空间；表格可接管内部滚动。 -->
<template>
  <section class="page page-layout">
    <div v-if="$slots.header" class="page-layout-header"><slot name="header" /></div>
    <div :key="bodyKey" class="page-layout-body" :class="{ 'is-scroll': scroll }"><slot /></div>
    <div v-if="$slots.footer" class="page-layout-footer"><slot name="footer" /></div>
    <slot name="overlays" />
  </section>
</template>

<script setup lang="ts">
// 翻页时重建滚动区域，避免新一页仍停在上一页底部。
withDefaults(defineProps<{ scroll?: boolean; bodyKey?: string | number }>(), { scroll: true })
</script>

<style scoped>
.page-layout {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
.page-layout-header,
.page-layout-footer {
  flex: none;
  min-width: 0;
}
.page-layout-body {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
.page-layout-body.is-scroll {
  overflow: auto;
}
</style>
