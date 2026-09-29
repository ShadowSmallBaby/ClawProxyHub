<!-- CDialog — 统一二次封装弹窗：圆角/头部字重集中管理。 -->
<template>
  <t-dialog v-bind="$attrs" class="c-dialog">
    <template v-for="(_, name) in $slots" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
  </t-dialog>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })
</script>

<style>
/* 统一弹窗质感：中圆角 + 头部 600 字重 */
.c-dialog .t-dialog {
  border-radius: 12px;
}
.c-dialog .t-dialog__header {
  font-weight: 600;
}
</style>
