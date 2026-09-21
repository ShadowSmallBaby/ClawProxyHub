<!-- ThemeSwitch — 头部明暗切换（状态在 useTheme 内持久化）。 -->
<template>
  <t-tooltip :content="dark ? $t('common.light') : $t('common.dark')">
    <t-button variant="text" shape="square" theme="default" @click="dark = !dark">
      <moon-icon v-if="!dark" />
      <sunny-icon v-else />
    </t-button>
  </t-tooltip>
</template>

<script setup lang="ts">
import { MoonIcon, SunnyIcon } from 'tdesign-icons-vue-next'
import { useTheme } from '../../composables'

const { dark } = useTheme()
</script>
