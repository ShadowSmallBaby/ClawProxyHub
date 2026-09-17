<template>
  <!-- TDesign 组件内置文案（分页/空状态/加载等）跟随全局语言切换 -->
  <t-config-provider :global-config="tdConfig">
    <router-view />
  </t-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import zhCN from 'tdesign-vue-next/es/locale/zh_CN'
import enUS from 'tdesign-vue-next/es/locale/en_US'
import i18n from './i18n'

const tdConfig = computed(() => (i18n.global.locale.value === 'en' ? enUS : zhCN))
</script>
