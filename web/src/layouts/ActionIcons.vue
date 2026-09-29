<!-- ActionIcons — 功能 icon 组：版本 / GitHub / 通知 / 语言 / 主题。 -->
<template>
  <div class="action-icons" :class="{ compact }">
    <version-chip />
    <github-icon-btn />
    <notif-bell />
    <lang-switch />
    <theme-switch />
  </div>
</template>

<script setup lang="ts">
import { GithubIconBtn, VersionChip, NotifBell, LangSwitch, ThemeSwitch } from './header'

defineProps<{
  compact?: boolean
}>()
</script>

<style scoped>
.action-icons {
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
/* 抽屉内：空间不足隐藏语言/主题，单行不换行 */
.action-icons.compact {
  gap: 4px;
}
</style>
