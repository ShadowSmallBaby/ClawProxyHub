<!-- CTabs — 统一二次封装页签：激活底色与过渡集中管理。 -->
<template>
  <t-tabs v-bind="$attrs" :class="{ 'c-tabs-fill': fill }">
    <template v-for="(_, name) in $slots" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
  </t-tabs>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })
defineProps<{ fill?: boolean }>()
</script>

<style>
/* 统一页签过渡：只针对颜色 */
.t-tabs .t-tabs__nav-item {
  transition: color 0.2s ease-out, background-color 0.2s ease-out;
}
.c-tabs-fill {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.c-tabs-fill > .t-tabs__header {
  flex: none;
}
.c-tabs-fill > .t-tabs__content {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.c-tabs-fill > .t-tabs__content > .t-tab-panel {
  height: 100%;
  min-height: 0;
}
</style>
