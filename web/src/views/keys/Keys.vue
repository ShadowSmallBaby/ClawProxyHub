<template>
  <div class="page">
    <page-header>
      
      <t-button theme="primary" @click="createVisible = true">{{ $t('keys.create') }}</t-button>
    </page-header>
    <c-table row-key="id" :data="keys" :columns="columns" :loading="loading">
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
          <t-link theme="primary" @click="openRename(row)">{{ $t('common.edit') }}</t-link>
          <t-link theme="primary" @click="bindVisible = row.id">{{ $t('keys.bindRoutes') }}</t-link>
          <t-popconfirm :content="$t('keys.confirmDelete')" @confirm="remove(row.id)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </c-table>

    <!-- 创建密钥 -->
    <c-dialog v-model:visible="createVisible" :header="$t('keys.create')" :confirm-btn="{ loading: creating }" @confirm="submitCreate">
      <t-form label-width="90px">
        <t-form-item :label="$t('keys.name')">
          <t-input v-model="createName" :placeholder="$t('keys.namePh')" clearable @enter="submitCreate" />
        </t-form-item>
      </t-form>
    </c-dialog>

    <!-- 明文只在创建时展示一次 -->
    <c-dialog v-model:visible="newKeyVisible" :header="$t('keys.createdTitle')" :footer="false">
      <div class="new-key">{{ newKey }}</div>
      <t-button block variant="outline" @click="copy">{{ $t('keys.copy') }}</t-button>
    </c-dialog>

    <c-dialog v-model:visible="bindVisibleBool" :header="$t('keys.bindTitle')" @confirm="bind">
      <bind-select v-model="bindRoutes" :options="routeOptions" :placeholder="$t('keys.bindPh')" />
    </c-dialog>

    <!-- 改名 -->
    <c-dialog v-model:visible="renameVisibleBool" :header="$t('keys.rename')" @confirm="submitRename">
      <t-form label-width="90px">
        <t-form-item :label="$t('keys.name')">
          <t-input v-model="renameName" :placeholder="$t('keys.namePh')" clearable @enter="submitRename" />
        </t-form-item>
      </t-form>
    </c-dialog>
  </div>
</template>

<script setup lang="ts">
import { CDialog } from '../../components/base'
import { CCard, CTable } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import { useAsync } from '../../composables'
import { useDialogVisible } from '../../composables'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { FileCopyIcon } from 'tdesign-icons-vue-next'
import { keyApi, routeApi } from '../../api/entities'
import { timeAgo } from '../../utils/format'
import BindSelect from '../../components/BindSelect.vue'
import type { KeyInfo, RouteInfo } from '../../api/types'

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
const renameVisible = ref<number | null>(null)
const renameName = ref('')

// 行内明文缓存（keyID → 明文，复制用）
const plainKeys = ref<Record<number, string>>({})

// 复制小图标 → 调 reveal（带缓存）→ 复制到粘贴板
async function copyKey(row: KeyInfo) {
  try {
    if (!plainKeys.value[row.id]) {
      const resp = await keyApi.reveal(row.id)
      plainKeys.value[row.id] = resp.key
    }
    await navigator.clipboard.writeText(plainKeys.value[row.id])
    MessagePlugin.success(t('keys.copiedPlain'))
  } catch (e: any) {
    MessagePlugin.error(e.message || t('keys.copyFailed'))
  }
}

const bindVisibleBool = useDialogVisible(bindVisible)

const renameVisibleBool = useDialogVisible(renameVisible)

function openRename(row: KeyInfo) {
  renameVisible.value = row.id
  renameName.value = row.name
}

async function submitRename() {
  if (renameVisible.value === null) return
  await keyApi.update(renameVisible.value, { name: renameName.value })
  MessagePlugin.success(t('common.updated'))
  renameVisible.value = null
  await load()
}

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

function routeName(id: number): string {
  return routes.value.find((r) => r.ID === id)?.Name ?? `#${id}`
}

const routeOptions = computed(() => routes.value.map((r) => ({ value: r.ID, label: r.Name })))

const { loading, run } = useAsync()

async function load() {
  await run(async () => {
    const [k, r] = await Promise.all([keyApi.list(), routeApi.list()])
    keys.value = k.keys ?? []
    routes.value = r.routes ?? []
  })
}

async function submitCreate() {
  creating.value = true
  try {
    const resp = await keyApi.create(createName.value)
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
  await keyApi.toggle(row.id)
  await load()
}

async function remove(id: number) {
  await keyApi.remove(id)
  await load()
}

async function bind() {
  if (bindVisible.value === null) return
  await keyApi.routes(bindVisible.value, bindRoutes.value)
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
