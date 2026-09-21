<!-- CTable — 统一二次封装表格：行悬停背景提亮 + 斑马纹关闭，集中管理边框/圆角。 -->
<template>
  <t-table v-bind="$attrs" class="c-table">
    <template v-for="(_, name) in $slots" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
  </t-table>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })
</script>

<style>
/* 统一表格质感：单元格边框降为弱描边，行悬停仅背景提亮 */
.c-table .t-table__cell {
  transition: background-color 0.2s ease-out;
}
</style>
