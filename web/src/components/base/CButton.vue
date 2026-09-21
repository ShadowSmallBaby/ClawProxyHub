<!-- CButton — 统一二次封装按钮：主色/描边/危险三种语义，集中管理圆角与过渡。 -->
<template>
  <t-button v-bind="$attrs" :theme="theme" :variant="variant">
    <template v-for="(_, name) in $slots" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
  </t-button>
</template>

<script setup lang="ts">
// theme/variant 仅设默认值，调用方可覆盖
withDefaults(defineProps<{ theme?: string; variant?: string }>(), {
  theme: 'default',
  variant: 'base',
})
</script>

<style>
/* 统一按钮质感：小圆角 + 颜色过渡 */
.t-button {
  border-radius: 8px;
  transition: color 0.2s ease-out, background-color 0.2s ease-out, border-color 0.2s ease-out;
}
</style>
