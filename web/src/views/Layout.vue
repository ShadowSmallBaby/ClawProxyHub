<template>
  <t-layout class="layout">
    <t-aside :width="collapsed ? '64px' : '200px'" class="aside">
      <div class="logo" @click="router.push('/dashboard')">
        <img class="logo-badge" src="/logo.png" alt="ClawProxyHub" />
        <span v-if="!collapsed" class="logo-text">Claw<span>ProxyHub</span></span>
      </div>
      <t-menu
        :value="route.path"
        :collapsed="collapsed"
        :width="collapsed ? '64px' : '200px'"
        class="aside-menu"
        @change="(v: string) => router.push(v)"
      >
        <t-menu-item v-for="item in visibleMenuItems" :key="item.value" :value="item.value">
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
          <t-tooltip v-if="version" :content="updateAvailable ? $t('common.hasUpdate') : $t('common.checkUpdate')">
            <div class="ver-chip" :class="{ 'has-update': updateAvailable, checking }" @click="checkVersion(true)">
              <t-loading v-if="checking" size="12px" />
              <span class="ver-text">v{{ version }}</span>
              <span v-if="updateAvailable && !checking" class="ver-dot" />
            </div>
          </t-tooltip>
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

    <!-- 更新日志弹窗：点版本徽标有更新时展示，右下按钮跳发布页 -->
    <t-dialog v-model:visible="changelogVisible" :header="changelogTitle" :footer="false" width="480px">
      <div class="changelog-ver">v{{ version }} → <b>v{{ latest }}</b></div>
      <ul v-if="changelog?.items?.length" class="changelog-list">
        <li v-for="(it, i) in changelog.items" :key="i">{{ clText(it) }}</li>
      </ul>
      <div class="changelog-foot">
        <t-button theme="primary" @click="gotoRelease">{{ $t('common.goRelease') }}</t-button>
      </div>
    </t-dialog>
  </t-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DashboardIcon, AppIcon, UserIcon, FolderIcon, InternetIcon,
  LockOnIcon, TimeIcon, FileIcon, RootListIcon, SettingIcon,
  ChevronLeftIcon, ChevronRightIcon, TranslateIcon, MoonIcon, SunnyIcon,
  PoweroffIcon, CheckIcon, LogoGithubIcon, CertificateIcon,
} from 'tdesign-icons-vue-next'
import { MessagePlugin } from 'tdesign-vue-next'
import { api, clearToken } from '../api/client'
import i18n, { setLocale as applyLocale, type Locale } from '../i18n'

const route = useRoute()
const router = useRouter()
const dark = ref(localStorage.getItem('cph-theme') === 'dark')
const collapsed = ref(localStorage.getItem('cph-sidebar') === 'collapsed')
// 用户名由 /admin/me 下发（token 已是 JWT，不能再从中拆用户名）
const username = ref('')
// 角色展示名 + 可见菜单键（后端 /admin/me 按角色下发，RBAC 前端守卫）
const role = ref('')
const allowedMenus = ref<string[] | null>(null) // null = 未加载（先全显，避免闪烁）
const roleLabel = computed(() =>
  i18n.global.t(role.value === 'guest' ? 'common.guest' : 'common.admin'),
)
api.get<{ username: string; role: string; menus: string[] }>('/admin/me')
  .then((r) => { username.value = r.username; role.value = r.role; allowedMenus.value = r.menus ?? null })
  .catch(() => {})

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

// ---------- 版本信息（头部徽标；点击重查，首次进页自动查一次） ----------
const version = ref('')
const latest = ref('')
const updateAvailable = ref(false)
const checking = ref(false)
const releaseUrl = ref('https://github.com/ShadowSmallBaby/ClawProxyHub/releases')
// 更新日志（远端 version.json.changelog，中英双语）+ 弹窗开关
interface Changelog { title?: Record<string, string>; items?: Record<string, string>[] }
const changelog = ref<Changelog | null>(null)
const changelogVisible = ref(false)

// checkVersion 拉取本机/远端版本对比；manual=true 时弹结果提示（手动点击）。
async function checkVersion(manual = false) {
  if (checking.value) return
  checking.value = true
  try {
    const r = await api.get<{ version: string; latest?: string; update_available?: boolean; release_url?: string; changelog?: Changelog }>('/admin/version')
    version.value = r.version
    latest.value = r.latest ?? ''
    updateAvailable.value = !!r.update_available
    if (r.release_url) releaseUrl.value = r.release_url
    changelog.value = r.changelog ?? null
    if (manual) {
      if (updateAvailable.value) {
        // 有更新：先弹更新日志弹窗（弹窗内按钮再跳发布页）
        changelogVisible.value = true
      } else if (latest.value) {
        MessagePlugin.success(i18n.global.t('common.upToDate'))
      } else {
        MessagePlugin.warning(i18n.global.t('common.checkFailed'))
      }
    }
  } catch {
    if (manual) MessagePlugin.warning(i18n.global.t('common.checkFailed'))
  } finally {
    checking.value = false
  }
}
// clText 按当前语言取更新日志文案（缺当前语言回退另一语言）
function clText(m: Record<string, string> | undefined): string {
  if (!m) return ''
  return m[locale.value] || m.zh || m.en || ''
}
// 更新日志弹窗标题（远端未给则用通用文案）
const changelogTitle = computed(() => clText(changelog.value?.title) || i18n.global.t('common.hasUpdate'))
// gotoRelease 跳发布页并关弹窗
function gotoRelease() {
  window.open(releaseUrl.value, '_blank')
  changelogVisible.value = false
}

checkVersion() // 首次进页自动查一次

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
  { value: '/oauth', label: 'menu.oauth', desc: 'menuDesc.oauth', icon: CertificateIcon },
  { value: '/tasks', label: 'menu.tasks', desc: 'menuDesc.tasks', icon: TimeIcon },
  { value: '/logs', label: 'menu.logs', desc: 'menuDesc.logs', icon: FileIcon },
  { value: '/settings', label: 'menu.settings', desc: 'menuDesc.settings', icon: SettingIcon },
]

// 按角色过滤后的可见菜单（allowedMenus 未加载时全显，避免刷新闪烁）
const visibleMenuItems = computed(() =>
  allowedMenus.value === null
    ? menuItems
    : menuItems.filter((m) => allowedMenus.value!.includes(m.value.slice(1))),
)

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
/* 版本 chip：默认灰字，有更新时描边高亮 + 红点，可点跳发布页 */
.ver-chip {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding: 0 10px;
  border-radius: 13px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--td-text-color-placeholder);
  cursor: pointer;
  transition: all 0.15s ease;
}
.ver-chip:hover {
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-secondarycontainer);
}
.ver-chip.checking {
  cursor: progress;
}
.ver-chip.has-update {
  color: var(--td-warning-color);
  background: var(--td-warning-color-1);
  cursor: pointer;
}
.ver-chip.has-update:hover {
  background: var(--td-warning-color-2);
}
.ver-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--td-warning-color);
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

/* 更新日志弹窗 */
.changelog-ver {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin-bottom: 12px;
  font-variant-numeric: tabular-nums;
}
.changelog-ver b {
  color: var(--td-brand-color);
}
.changelog-list {
  margin: 0;
  padding-left: 20px;
  line-height: 1.9;
  font-size: 14px;
  color: var(--td-text-color-primary);
}
.changelog-foot {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
}
</style>
