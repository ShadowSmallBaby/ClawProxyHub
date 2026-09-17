<template>
  <t-layout class="layout">
    <t-aside :width="collapsed ? '64px' : '220px'" class="aside">
      <div class="logo" @click="router.push('/dashboard')">
        <img class="logo-badge" src="/logo.png" alt="ClawProxyHub" />
        <span v-if="!collapsed" class="logo-text">Claw<span>ProxyHub</span></span>
      </div>
      <t-menu
        :value="route.path"
        :collapsed="collapsed"
        :width="collapsed ? '64px' : '220px'"
        class="aside-menu"
        @change="(v: string) => router.push(v)"
      >
        <t-menu-item v-for="item in menuItems" :key="item.value" :value="item.value">
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
    <t-layout>
      <!-- 状态头：左菜单信息 + 右操作区，固定不滚动 -->
      <t-header class="header">
        <div class="header-left">
          <div class="header-title">{{ $t(currentPage.label) }}</div>
          <div class="header-desc">{{ $t(currentPage.desc) }}</div>
        </div>
        <div class="header-right">
          <t-tooltip :content="$t('common.github')">
            <t-button variant="text" shape="square" theme="default" @click="openGithub">
              <logo-github-icon />
            </t-button>
          </t-tooltip>
          <t-popup trigger="click">
            <t-button variant="text" shape="square" theme="default">
              <translate-icon />
            </t-button>
            <template #content>
              <div class="lang-menu">
                <div
                  v-for="l in langs"
                  :key="l"
                  class="lang-menu-item"
                  :class="{ active: locale === l }"
                  @click="setLocale(l)"
                >
                  {{ $t('common.' + l) }}
                  <check-icon v-if="locale === l" />
                </div>
              </div>
            </template>
          </t-popup>
          <t-tooltip :content="dark ? $t('common.light') : $t('common.dark')">
            <t-button variant="text" shape="square" theme="default" @click="dark = !dark">
              <moon-icon v-if="!dark" />
              <sunny-icon v-else />
            </t-button>
          </t-tooltip>
          <t-popup trigger="click">
            <div class="user-chip">
              <t-avatar size="26px" theme="light" class="user-avatar">
                <template #icon><user-icon /></template>
              </t-avatar>
              <span v-if="username" class="user-name">{{ username }}</span>
            </div>
            <template #content>
              <div class="user-menu">
                <div class="user-menu-head">
                  <t-avatar size="34px" theme="light">
                    <template #icon><user-icon /></template>
                  </t-avatar>
                  <div>
                    <div class="user-menu-name">{{ username || $t('common.admin') }}</div>
                    <div class="user-menu-sub">{{ roleLabel }}</div>
                  </div>
                </div>
                <div class="user-menu-item disabled">
                  <user-icon /> {{ $t('common.profile') }}
                </div>
                <div class="user-menu-item" @click="logout">
                  <poweroff-icon /> {{ $t('common.logout') }}
                </div>
              </div>
            </template>
          </t-popup>
        </div>
      </t-header>
      <!-- 内容区域：内部滚动，头部与侧栏固定 -->
      <t-content class="content">
        <router-view />
      </t-content>
    </t-layout>
  </t-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DashboardIcon, AppIcon, UserIcon, FolderIcon, InternetIcon,
  LockOnIcon, TimeIcon, FileIcon, RootListIcon, SettingIcon,
  ChevronLeftIcon, ChevronRightIcon, TranslateIcon, MoonIcon, SunnyIcon,
  PoweroffIcon, CheckIcon, LogoGithubIcon,
} from 'tdesign-icons-vue-next'
import { api, clearToken, getToken } from '../api/client'
import i18n, { setLocale as applyLocale, type Locale } from '../i18n'

const route = useRoute()
const router = useRouter()
const dark = ref(localStorage.getItem('cph-theme') === 'dark')
const collapsed = ref(localStorage.getItem('cph-sidebar') === 'collapsed')
// token 形如 "user:password"，展示用户名部分
const username = computed(() => getToken().split(':')[0])
// 角色展示名（管理员 / 访客，后端 /admin/me 提供）
const role = ref('')
const roleLabel = computed(() =>
  i18n.global.t(role.value === 'guest' ? 'common.guest' : 'common.admin'),
)
api.get<{ role: string }>('/admin/me').then((r) => (role.value = r.role)).catch(() => {})

// ---------- 多语言（zh / en，vue-i18n 全局响应式） ----------
const locale = computed<Locale>(() => i18n.global.locale.value as Locale)
const langs: Locale[] = ['zh', 'en']
function setLocale(l: Locale) {
  applyLocale(l)
}

// 项目主页
function openGithub() {
  window.open('https://github.com/ShadowSmallBaby/ClawProxyHub', '_blank')
}

interface MenuItem {
  value: string
  label: string // i18n key（menu.xxx）
  desc: string // i18n key（menuDesc.xxx）
  icon: Component
}

const menuItems: MenuItem[] = [
  { value: '/dashboard', label: 'menu.dashboard', desc: 'menuDesc.dashboard', icon: DashboardIcon },
  { value: '/plugins', label: 'menu.plugins', desc: 'menuDesc.plugins', icon: AppIcon },
  { value: '/accounts', label: 'menu.accounts', desc: 'menuDesc.accounts', icon: UserIcon },
  { value: '/groups', label: 'menu.groups', desc: 'menuDesc.groups', icon: FolderIcon },
  { value: '/proxies', label: 'menu.proxies', desc: 'menuDesc.proxies', icon: RootListIcon },
  { value: '/routes', label: 'menu.routes', desc: 'menuDesc.routes', icon: InternetIcon },
  { value: '/keys', label: 'menu.keys', desc: 'menuDesc.keys', icon: LockOnIcon },
  { value: '/tasks', label: 'menu.tasks', desc: 'menuDesc.tasks', icon: TimeIcon },
  { value: '/logs', label: 'menu.logs', desc: 'menuDesc.logs', icon: FileIcon },
  { value: '/settings', label: 'menu.settings', desc: 'menuDesc.settings', icon: SettingIcon },
]

// 当前菜单（含子路径前缀匹配）
const currentPage = computed(
  () => menuItems.find((m) => route.path.startsWith(m.value)) ?? menuItems[0],
)

// 主题切换与收起状态持久化
watch(dark, (v) => {
  const mode = v ? 'dark' : 'light'
  document.documentElement.setAttribute('theme-mode', mode)
  localStorage.setItem('cph-theme', mode)
})
watch(collapsed, (v) => {
  localStorage.setItem('cph-sidebar', v ? 'collapsed' : 'expanded')
})

// 初始主题
document.documentElement.setAttribute('theme-mode', dark.value ? 'dark' : 'light')

function logout() {
  clearToken()
  router.push('/login')
}
</script>

<style scoped>
.layout {
  height: 100%;
}
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
/* 语言菜单 */
.lang-menu {
  min-width: 140px;
}
.lang-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  white-space: nowrap;
  border-radius: 6px;
  transition: background-color 0.15s ease;
}
.lang-menu-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.lang-menu-item.active {
  color: var(--td-brand-color);
}
/* 用户胶囊：头像 + 用户名 */
.user-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 18px;
  transition: background-color 0.15s ease;
}
.user-chip:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.user-avatar {
  color: var(--td-brand-color);
}
.user-name {
  font-size: 13px;
  color: var(--td-text-color-primary);
  max-width: 120px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* 下拉功能块 */
.user-menu {
  min-width: 180px;
}
.user-menu-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px 12px;
  border-bottom: 1px solid var(--td-component-border);
  margin-bottom: 4px;
}
.user-menu-head .t-avatar {
  color: var(--td-brand-color);
}
.user-menu-name {
  font-size: 14px;
  font-weight: 600;
}
.user-menu-sub {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.user-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  transition: background-color 0.15s ease;
}
.user-menu-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.user-menu-item.disabled {
  color: var(--td-text-color-disabled);
  cursor: not-allowed;
}
.user-menu-item.disabled:hover {
  background: none;
}

/* 内容区域：占满剩余高度，内部滚动（头部/侧栏固定） */
.content {
  flex: 1;
  overflow-y: auto;
  height: 0;
}
</style>
