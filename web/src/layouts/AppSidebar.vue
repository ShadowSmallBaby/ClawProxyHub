<!-- AppSidebar — 侧栏：logo + 菜单 + 收起脚。 -->
<template>
  <t-aside :width="collapsed ? '64px' : '200px'" class="aside">
    <div class="logo" @click="router.push('/dashboard')">
      <img class="logo-badge" :src="brandLogo" :alt="branding.name" />
      <span v-if="!collapsed" class="logo-text" :class="{ custom: brandCustom }" :title="branding.name">
        <template v-if="brandCustom">{{ branding.name }}</template>
        <template v-else>Claw<span>ProxyHub</span></template>
      </span>
    </div>
    <t-menu
      :value="route.path"
      :collapsed="collapsed"
      :width="collapsed ? '64px' : '200px'"
      class="aside-menu"
      @change="(v: string) => router.push(v)"
    >
      <t-menu-item v-for="item in items" :key="item.value" :value="item.value">
        <template #icon><component :is="item.icon" /></template>{{ $t(item.label) }}
      </t-menu-item>
    </t-menu>
    <div class="aside-footer" @click="collapsed = !collapsed">
      <template v-if="!collapsed">
        <chevron-left-icon />
        <span class="aside-footer-text">{{ $t('common.collapse') }}</span>
      </template>
      <chevron-right-icon v-else />
    </div>
  </t-aside>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeftIcon, ChevronRightIcon } from 'tdesign-icons-vue-next'
import { branding, brandLogo, brandCustom } from '../utils/branding'
import type { MenuItem } from './types'

defineProps<{ items: MenuItem[] }>()

const route = useRoute()
const router = useRouter()

// 收起状态持久化
const collapsed = ref(localStorage.getItem('cph-sidebar') === 'collapsed')
</script>

<style scoped>
.aside {
  flex-shrink: 0; /* TDesign sider 默认参与收缩，会把 220px 挤没 */
  display: flex;
  flex-direction: column;
  transition: width 0.25s;
  overflow: hidden;
}
.logo {
  height: 64px;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  cursor: pointer;
  color: var(--td-brand-color);
  font-size: 18px;
  font-weight: 700;
  flex-shrink: 0;
}
.logo-badge {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--td-brand-color-3); /* 品牌色描边 */
  flex-shrink: 0;
  object-fit: cover;
}
.logo-text {
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
}
/* 自定义品牌名：品牌色高亮字（不用渐变） */
.logo-text.custom {
  font-size: 16px;
  color: var(--td-brand-color);
}
.logo-text span {
  font-weight: 300;
  opacity: 0.8;
  margin-left: 2px;
}
.aside-menu {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}
.aside-footer {
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  cursor: pointer;
  color: var(--td-text-color-placeholder);
  border-top: 1px solid var(--td-component-border);
  flex-shrink: 0;
}
.aside-footer:hover {
  color: var(--td-brand-color);
}
.aside-footer-text {
  font-size: 12px;
  white-space: nowrap;
}
</style>
