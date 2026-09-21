<!-- NotifBell — 头部通知铃铛：未读红点；点击列表，条目点击标记已读并弹详情；60s 轮询。 -->
<template>
  <t-popup trigger="click" placement="bottom-right" @visible-change="(v: boolean) => v && load()">
    <t-badge :count="unread" :offset="[4, 4]" size="small">
      <t-button variant="text" shape="square" theme="default">
        <notification-icon />
      </t-button>
    </t-badge>
    <template #content>
      <div class="notif-menu">
        <div class="notif-head">
          <span>{{ $t('common.notifications') }}<span v-if="unread" class="notif-unread"> · {{ $t('common.unreadN', { n: unread }) }}</span></span>
          <span class="notif-actions">
            <t-link v-if="unread" size="small" theme="primary" @click="readAll">{{ $t('common.markAllRead') }}</t-link>
            <t-link v-if="notifications.some((n) => n.read)" size="small" theme="default" @click="clearRead">{{ $t('common.clearRead') }}</t-link>
          </span>
        </div>
        <div v-if="!notifications.length" class="notif-empty">{{ $t('common.noNotifications') }}</div>
        <div v-else class="notif-list">
          <div v-for="n in notifications" :key="n.id" class="notif-item" :class="{ unread: !n.read, [n.level]: true }" @click="open(n)">
            <span class="notif-dot" />
            <div class="notif-body">
              <div class="notif-title">{{ n.title }}</div>
              <div class="notif-time">{{ n.created_at.replace('T', ' ').slice(0, 16) }}</div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </t-popup>

  <!-- 通知详情弹窗 -->
  <t-dialog v-model:visible="detailVisible" :header="current?.title" :footer="false" width="480px">
    <div class="notif-content">{{ current?.content }}</div>
    <div v-if="current" class="notif-meta">{{ current.created_at.replace('T', ' ').slice(0, 19) }}</div>
  </t-dialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { NotificationIcon } from 'tdesign-icons-vue-next'
import { notificationApi, type Notification } from '../../api/auth'

const notifications = ref<Notification[]>([])
const unread = ref(0)
const current = ref<Notification | null>(null)
const detailVisible = ref(false)

async function load() {
  try {
    const r = await notificationApi.list()
    notifications.value = r.notifications ?? []
    unread.value = r.unread ?? 0
  } catch { /* 静默 */ }
}

async function open(n: Notification) {
  current.value = n
  detailVisible.value = true
  if (!n.read) {
    n.read = true
    unread.value = Math.max(0, unread.value - 1)
    notificationApi.read(n.id).catch(() => {})
  }
}

async function readAll() {
  await notificationApi.readAll().catch(() => {})
  load()
}

async function clearRead() {
  await notificationApi.clear().catch(() => {})
  load()
}

load()
const timer = window.setInterval(load, 60_000)
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<style scoped>
/* 通知列表 */
.notif-menu {
  width: 320px;
}
.notif-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px 10px;
  font-size: 13px;
  font-weight: 600;
  border-bottom: 1px solid var(--td-component-border);
}
.notif-unread {
  font-weight: 400;
  color: var(--td-text-color-placeholder);
}
.notif-actions {
  display: flex;
  gap: 10px;
  font-weight: 400;
}
.notif-empty {
  padding: 28px 0;
  text-align: center;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}
.notif-list {
  max-height: 360px;
  overflow-y: auto;
  padding-top: 4px;
}
.notif-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.2s ease-out;
}
.notif-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.notif-dot {
  width: 6px;
  height: 6px;
  margin-top: 6px;
  border-radius: 50%;
  background: transparent;
  flex-shrink: 0;
}
.notif-item.unread .notif-dot {
  background: var(--td-brand-color);
}
.notif-item.unread.warning .notif-dot {
  background: var(--td-warning-color);
}
.notif-item.unread.error .notif-dot {
  background: var(--td-error-color);
}
.notif-body {
  min-width: 0;
  flex: 1;
}
.notif-title {
  font-size: 13px;
  color: var(--td-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.notif-item:not(.unread) .notif-title {
  color: var(--td-text-color-secondary);
}
.notif-time {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  margin-top: 2px;
}
.notif-content {
  font-size: 14px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-all;
}
.notif-meta {
  margin-top: 12px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
