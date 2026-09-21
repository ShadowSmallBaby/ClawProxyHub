<!-- UserMenu — 个人头像 + 下拉（用户信息 / 个人中心 / 设置 / 退出）。 -->
<template>
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
        <div class="user-menu-item" @click="go('/profile')">
          <user-icon /> {{ $t('common.profile') }}
        </div>
        <div v-if="role !== 'guest'" class="user-menu-item" @click="go('/settings')">
          <setting-icon /> {{ $t('menu.settings') }}
        </div>
        <div class="user-menu-item" @click="logout">
          <poweroff-icon /> {{ $t('common.logout') }}
        </div>
      </div>
    </template>
  </t-popup>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { PoweroffIcon, SettingIcon, UserIcon } from 'tdesign-icons-vue-next'
import i18n from '../../i18n'
import { clearToken } from '../../api/client'
import { useAsync } from '../../composables'

const props = defineProps<{
  username: string
  role: string
}>()

const { t } = useI18n()
const router = useRouter()
const { run } = useAsync()

const roleLabel = computed(() =>
  i18n.global.t(props.role === 'guest' ? 'common.guest' : 'common.admin'),
)

function go(path: string) {
  router.push(path)
}

function logout() {
  clearToken()
  router.push('/login')
}
</script>

<style scoped>
/* 用户胶囊：头像 + 用户名 */
.user-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 18px;
  transition: background-color 0.2s ease-out;
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
  transition: background-color 0.2s ease-out;
}
.user-menu-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
</style>
