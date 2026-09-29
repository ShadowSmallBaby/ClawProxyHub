<!-- AppHeader — 头部：左页面标题/描述 + 右功能区（ActionIcons + UserMenu）。 -->
<template>
  <t-header class="header">
    <div class="header-left">
      <t-button v-if="isMobile" variant="text" shape="square" class="menu-btn" @click="$emit('toggle-menu')">
        <template #icon><menu-icon /></template>
      </t-button>
      <div class="header-text">
        <div class="header-title">{{ $t(currentPage.label) }}</div>
        <div v-if="!isMobile" class="header-desc">{{ $t(currentPage.desc) }}</div>
      </div>
    </div>
    <div class="header-right">
      <action-icons v-if="!isMobile" />
      <user-menu :username="isMobile ? '' : username" :role="role" />
    </div>
  </t-header>
</template>

<script setup lang="ts">
import { MenuIcon } from 'tdesign-icons-vue-next'
import ActionIcons from './ActionIcons.vue'
import { UserMenu } from './header'
import type { MenuItem } from './types'

defineProps<{
  username: string
  role: string
  currentPage: MenuItem
  isMobile: boolean
}>()

defineEmits<{ 'toggle-menu': [] }>()
</script>

<style scoped>
/* 状态头：左右结构，固定不滚动 */
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 64px;
  padding: 0 24px;
  padding-left: max(24px, env(safe-area-inset-left));
  padding-right: max(24px, env(safe-area-inset-right));
  flex-shrink: 0;
}
.header-left {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
}
.menu-btn {
  flex-shrink: 0;
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
  gap: 8px;
  flex-shrink: 0;
}
@media (max-width: 768px) {
  .header {
    height: 52px;
    padding: 0 8px;
    padding-left: max(8px, env(safe-area-inset-left));
  }
  /* 手机端：actions 收进抽屉，仅头像在 header */
  .header-right {
    gap: 4px;
  }
}
</style>
