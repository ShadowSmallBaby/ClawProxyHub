<!-- AppLayout — 组装层：侧栏 + 头部 + 内容区。状态拆在子组件内（主题 useTheme、用户信息 /admin/me、菜单配置）。 -->
<template>
  <t-layout class="layout">
    <app-sidebar :items="visibleMenuItems" />
    <t-layout>
      <app-header :username="username" :role="role" :current-page="currentPage" />
      <!-- 内容区域：内部滚动，头部与侧栏固定 -->
      <t-content class="content">
        <router-view />
      </t-content>
    </t-layout>
  </t-layout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { DashboardIcon, AppIcon, UserIcon, FolderIcon, InternetIcon, LockOnIcon, TimeIcon, FileIcon, RootListIcon, SettingIcon, CertificateIcon, ServerIcon } from 'tdesign-icons-vue-next'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import { authApi } from '../api/auth'
import { useTheme } from '../composables'
import type { MenuItem } from './types'

useTheme()

const route = useRoute()

// 用户名由 /admin/me 下发（token 已是 JWT，不能再从中拆用户名）
const username = ref('')
// 角色展示名 + 可见菜单键（后端 /admin/me 按角色下发，RBAC 前端守卫）
const role = ref('')
const allowedMenus = ref<string[] | null>(null) // null = 未加载（先全显，避免闪烁）
authApi.me()
  .then((r) => { username.value = r.username; role.value = r.role; allowedMenus.value = r.menus ?? null })
  .catch(() => {})

const menuItems: MenuItem[] = [
  { value: '/dashboard', label: 'menu.dashboard', desc: 'menuDesc.dashboard', icon: DashboardIcon },
  { value: '/plugins', label: 'menu.plugins', desc: 'menuDesc.plugins', icon: AppIcon },
  { value: '/instances', label: 'menu.instances', desc: 'menuDesc.instances', icon: ServerIcon },
  { value: '/accounts', label: 'menu.accounts', desc: 'menuDesc.accounts', icon: UserIcon },
  { value: '/groups', label: 'menu.groups', desc: 'menuDesc.groups', icon: FolderIcon },
  { value: '/proxies', label: 'menu.proxies', desc: 'menuDesc.proxies', icon: RootListIcon },
  { value: '/routes', label: 'menu.routes', desc: 'menuDesc.routes', icon: InternetIcon },
  { value: '/keys', label: 'menu.keys', desc: 'menuDesc.keys', icon: LockOnIcon },
  { value: '/oauth', label: 'menu.oauth', desc: 'menuDesc.oauth', icon: CertificateIcon },
  { value: '/tasks', label: 'menu.tasks', desc: 'menuDesc.tasks', icon: TimeIcon },
  { value: '/logs', label: 'menu.logs', desc: 'menuDesc.logs', icon: FileIcon },
]

// 头像下拉里的页面（不在侧栏）：用于顶部标题/描述匹配
const extraPages: MenuItem[] = [
  { value: '/profile', label: 'common.profile', desc: 'menuDesc.profile', icon: UserIcon },
  { value: '/settings', label: 'menu.settings', desc: 'menuDesc.settings', icon: SettingIcon },
]

// 按角色过滤后的可见菜单（allowedMenus 未加载时全显，避免刷新闪烁）
const visibleMenuItems = computed(() =>
  allowedMenus.value === null
    ? menuItems
    : menuItems.filter((m) => allowedMenus.value!.includes(m.value.slice(1))),
)

// 当前页面（含子路径前缀匹配；头像下拉页也参与匹配）
const currentPage = computed(
  () => [...menuItems, ...extraPages].find((m) => route.path.startsWith(m.value)) ?? menuItems[0],
)
</script>

<style scoped>
.layout {
  height: 100%;
}
/* 内容区域：占满剩余高度，内部滚动（头部/侧栏固定） */
.content {
  flex: 1;
  overflow-y: auto;
  height: 0;
}
</style>
