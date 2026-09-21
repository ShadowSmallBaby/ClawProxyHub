<!-- EllipsisCell — 表格省略单元格：有内容时悬浮展示全文，空值显示 -。 -->
<template>
  <t-tooltip v-if="content" :content="content" placement="top-left">
    <span class="ellipsis-cell">{{ content }}</span>
  </t-tooltip>
  <span v-else class="ellipsis-dim">-</span>
</template>

<script setup lang="ts">
defineProps<{ content?: string | null }>()
</script>

<style scoped>
.ellipsis-cell {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
  cursor: default;
}
.ellipsis-dim {
  color: var(--td-text-color-placeholder);
}
</style>
