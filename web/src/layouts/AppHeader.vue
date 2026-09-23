<!-- AppHeader — 头部：左页面标题/描述 + 右功能区（版本/通知/语言/主题/用户）。 -->
<template>
  <t-header class="header">
    <div class="header-left">
      <div class="header-title">{{ $t(currentPage.label) }}</div>
      <div class="header-desc">{{ $t(currentPage.desc) }}</div>
    </div>
    <div class="header-right">
      <version-chip />
      <github-icon-btn />
      <notif-bell />
      <lang-switch />
      <theme-switch />
      <user-menu :username="username" :role="role" />
    </div>
  </t-header>
</template>

<script setup lang="ts">
import { GithubIconBtn, NotifBell, LangSwitch, ThemeSwitch, UserMenu, VersionChip } from './header'
import type { MenuItem } from './types'

defineProps<{
  username: string
  role: string
  currentPage: MenuItem
}>()
</script>

<style scoped>
/* 状态头：左右结构，固定不滚动 */
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 64px;
  padding: 0 24px;
  flex-shrink: 0;
}
.header-left {
  min-width: 0;
}
.header-title {
  font-size: 16px;
  font-weight: 700;
  line-height: 1.3;
}
.header-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
</style>
