<!-- CCard — 统一二次封装卡片：1px 描边 + 中圆角，默认无边框高亮悬浮（仅背景提亮）。 -->
<template>
  <t-card v-bind="$attrs" :bordered="false" class="c-card">
    <template v-for="(_, name) in $slots" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
  </t-card>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })
</script>

<style>
/* 统一卡片质感：1px 描边 + 中圆角 + 悬浮仅背景提亮（不上浮不发光） */
.c-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  transition: background-color 0.2s ease-out;
}
.c-card:hover {
  background-color: var(--td-bg-color-container-hover);
}
.c-card .t-card__header {
  font-weight: 600;
}
</style>
