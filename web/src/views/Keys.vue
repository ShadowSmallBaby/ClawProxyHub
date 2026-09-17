<template>
  <div class="page">
    <div class="page-header">
      
      <t-button theme="primary" @click="createVisible = true">{{ $t('keys.create') }}</t-button>
    </div>
    <t-table row-key="id" :data="keys" :columns="columns">
      <template #key="{ row }">
        <span class="key-mask">
          {{ row.key_mask }}
          <t-tooltip :content="$t('keys.copyPlain')">
            <file-copy-icon class="copy-icon" @click="copyKey(row)" />
          </t-tooltip>
        </span>
      </template>
      <template #enabled="{ row }">
        <t-switch :value="row.enabled" @change="() => toggle(row)" />
      </template>
      <template #routes="{ row }">
        <template v-if="row.route_ids?.length">
          <t-tag v-for="rid in row.route_ids" :key="rid" size="small" variant="light">
            {{ routeName(rid) }}
          </t-tag>
        </template>
        <t-tag v-else size="small" theme="primary" variant="light">{{ $t('keys.allRoutes') }}</t-tag>
      </template>
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="bindVisible = row.id">{{ $t('keys.bindRoutes') }}</t-link>
          <t-popconfirm :content="$t('keys.confirmDelete')" @confirm="remove(row.id)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </t-table>

    <!-- 创建密钥 -->
    <t-dialog v-model:visible="createVisible" :header="$t('keys.create')" :confirm-btn="{ loading: creating }" @confirm="submitCreate">
      <t-form label-width="90px">
        <t-form-item :label="$t('keys.name')">
          <t-input v-model="createName" :placeholder="$t('keys.namePh')" clearable @enter="submitCreate" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 明文只在创建时展示一次 -->
    <t-dialog v-model:visible="newKeyVisible" :header="$t('keys.createdTitle')" :footer="false">
      <div class="new-key">{{ newKey }}</div>
      <t-button block variant="outline" @click="copy">{{ $t('keys.copy') }}</t-button>
    </t-dialog>

    <t-dialog v-model:visible="bindVisibleBool" :header="$t('keys.bindTitle')" @confirm="bind">
      <t-select v-model="bindRoutes" multiple clearable :placeholder="$t('keys.bindPh')">
        <t-option v-for="r in routes" :key="r.ID" :value="r.ID" :label="r.Name" />
      </t-select>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { FileCopyIcon } from 'tdesign-icons-vue-next'
import { api } from '../api/client'
import type { KeyInfo, RouteInfo } from '../api/types'

const { t } = useI18n()

const keys = ref<KeyInfo[]>([])
const routes = ref<RouteInfo[]>([])
const newKeyVisible = ref(false)
const newKey = ref('')
const createVisible = ref(false)
const createName = ref('')
const creating = ref(false)
const bindVisible = ref<number | null>(null)
const bindRoutes = ref<number[]>([])

// 行内明文缓存（keyID → 明文，复制用）
const plainKeys = ref<Record<number, string>>({})

// 复制小图标 → 调 reveal（带缓存）→ 复制到粘贴板
async function copyKey(row: KeyInfo) {
  try {
    if (!plainKeys.value[row.id]) {
      const resp = await api.get<{ key: string }>(`/admin/keys/${row.id}/reveal`)
      plainKeys.value[row.id] = resp.key
    }
    await navigator.clipboard.writeText(plainKeys.value[row.id])
    MessagePlugin.success(t('keys.copiedPlain'))
  } catch (e: any) {
    MessagePlugin.error(e.message || t('keys.copyFailed'))
  }
}

const bindVisibleBool = computed({
  get: () => bindVisible.value !== null,
  set: (v: boolean) => { if (!v) bindVisible.value = null },
})

const columns = computed(() => [
  { colKey: 'id', title: t('common.colId'), width: 70 },
  { colKey: 'name', title: t('keys.name'), width: 150, ellipsis: true, align: 'center' },
  { colKey: 'key', title: t('keys.colKey'), width: 300, align: 'center' },
  { colKey: 'enabled', title: t('keys.enabled'), width: 90, align: 'center' },
  { colKey: 'routes', title: t('keys.colRoutes'), align: 'center' },
  { colKey: 'last_used_at', title: t('keys.colLastUsed'), width: 110, cell: (_h: any, { row }: any) => row.last_used_at ? timeAgo(row.last_used_at) : t('keys.never'), align: 'center' },
  { colKey: 'created_at', title: t('keys.colCreatedAt'), width: 180, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 150, align: 'center' },
])

// 相对时间：如 5分钟前 / 1天前 / 3个月前
function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts.replace(' ', 'T')).getTime()
  const min = Math.floor(diff / 60000)
  if (min < 1) return t('common.justNow')
  if (min < 60) return t('common.minutesAgo', { n: min })
  const h = Math.floor(min / 60)
  if (h < 24) return t('common.hoursAgo', { n: h })
  const d = Math.floor(h / 24)
  if (d < 30) return t('common.daysAgo', { n: d })
  const mo = Math.floor(d / 30)
  if (mo < 12) return t('common.monthsAgo', { n: mo })
  return t('common.yearsAgo', { n: Math.floor(mo / 12) })
}

function routeName(id: number): string {
  return routes.value.find((r) => r.ID === id)?.Name ?? `#${id}`
}

async function load() {
  const [k, r] = await Promise.all([
    api.get<{ keys: KeyInfo[] }>('/admin/keys'),
    api.get<{ routes: RouteInfo[] }>('/admin/routes'),
  ])
  keys.value = k.keys ?? []
  routes.value = r.routes ?? []
}

async function submitCreate() {
  creating.value = true
  try {
    const resp = await api.post<{ key: string }>('/admin/keys', { name: createName.value })
    newKey.value = resp.key
    createVisible.value = false
    createName.value = ''
    newKeyVisible.value = true
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    creating.value = false
  }
}

async function toggle(row: KeyInfo) {
  await api.post(`/admin/keys/${row.id}/toggle`)
  await load()
}

async function remove(id: number) {
  await api.del(`/admin/keys/${id}`)
  await load()
}

async function bind() {
  if (bindVisible.value === null) return
  await api.put(`/admin/keys/${bindVisible.value}/routes`, { route_ids: bindRoutes.value })
  MessagePlugin.success(t('common.updated'))
  bindVisible.value = null
  await load()
}

function copy() {
  navigator.clipboard.writeText(newKey.value)
  MessagePlugin.success(t('keys.copied'))
}

onMounted(load)
</script>

<style scoped>
.new-key {
  margin: 16px 0;
  padding: 12px;
  font-family: monospace;
  word-break: break-all;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
}
.key-mask {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-family: monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  white-space: nowrap; /* 密钥一行展示 */
}
.key-plain {
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}
.copy-icon {
  cursor: pointer;
  color: var(--td-text-color-placeholder);
  transition: color 0.15s ease;
  flex-shrink: 0;
  width: 12px;
  height: 12px;
  font-size: 12px;
}
.copy-icon:hover {
  color: var(--td-brand-color);
}
</style>
