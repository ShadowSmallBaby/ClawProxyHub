<!-- AppLayout — 组装层：侧栏 + 头部 + 内容区。状态拆在子组件内（主题 useTheme、用户信息 /admin/me、菜单配置）。 -->
<template>
  <t-layout class="layout">
    <app-sidebar v-if="!isAndroidApp" :items="visibleMenuItems" />
    <!-- 移动端：侧栏改为抽屉（品牌区 + 菜单 + 功能区 footer） -->
    <mobile-nav-drawer v-if="isMobile || isAndroidApp" v-model:visible="drawerVisible" :items="visibleMenuItems" />
    <t-layout class="inner">
      <app-header  :username="username" :role="role" :current-page="currentPage" :is-mobile="isMobile || isAndroidApp" @toggle-menu="drawerVisible = true" />
      <!-- 内容区域：内部滚动，头部与侧栏固定 -->
      <t-content class="content">
        <t-alert v-if="backendAccess?.legacy" theme="warning" :message="$t('connections.legacyBackend')" />
        <router-view v-slot="{ Component }"><component :is="Component" :key="$route.path.startsWith('/extensions/') ? $route.fullPath : undefined" /></router-view>
      </t-content>
    </t-layout>
  </t-layout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { features } from '@profile'
import { isAndroidApp } from '@/api/application'
import { backendAccess } from '@/features/access'
import { permits } from '@/features/policy'
import { ensureBranding } from '@/utils/branding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import MobileNavDrawer from './MobileNavDrawer.vue'
import { useTheme, useIsMobile } from '@/composables'
import type { MenuItem } from './types'

useTheme()
ensureBranding()

const route = useRoute()
const { isMobile } = useIsMobile()
const drawerVisible = ref(false)

const username = computed(() => backendAccess.value?.username ?? '')
const role = computed(() => backendAccess.value?.role ?? 'guest')
const menuItems: MenuItem[] = features.map(f => ({ value: `/${f.path}`, label: f.label, desc: f.desc, icon: f.icon }))
const visibleMenuItems = computed(() => menuItems.filter((_, i) => features[i].sidebar && permits(backendAccess.value, features[i])))
const currentPage = computed(() => menuItems.find(m => m.value === route.path) ?? menuItems.find(m => route.path.startsWith(m.value)) ?? menuItems[0])
</script>

<style scoped>
.layout {
  height: 100%;
}
/* 内层布局：压住 flex 默认 min-width:auto，不被超宽内容撑出视口（header/侧栏始终固定可见） */
.inner {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}
/* 页面自行管理主体滚动，避免外层滚动带走页头和分页。 */
.content {
  flex: 1;
  height: 0;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
</style>
