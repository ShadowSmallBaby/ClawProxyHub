<template>
  <div class="page">
    <div class="page-header">
      
      <t-button theme="primary" @click="createVisible = true">{{ $t('groups.create') }}</t-button>
    </div>
    <t-table row-key="id" :data="groups" :columns="columns">
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="openBind(row)">{{ $t('groups.bind') }}</t-link>
          <t-popconfirm :content="$t('groups.confirmDelete')" @confirm="remove(row.id)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </t-table>

    <t-dialog v-model:visible="createVisible" :header="$t('groups.create')" :confirm-btn="{ loading: creating }" @confirm="create">
      <t-form label-width="90px">
        <t-form-item :label="$t('groups.name')" mark>
          <t-input v-model="newName" :placeholder="$t('groups.namePh')" />
        </t-form-item>
        <t-form-item :label="$t('groups.plugin')" mark>
          <t-select v-model="newPlugin">
            <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
          </t-select>
        </t-form-item>
        <t-alert theme="info" :message="$t('groups.hintCreate')" />
      </t-form>
    </t-dialog>

    <t-dialog v-model:visible="bindVisible" :header="$t('groups.bindHeader', { name: bindGroup?.name })" @confirm="bind">
      <t-select v-model="bindProxyIds" multiple clearable :placeholder="$t('groups.bindPh')">
        <t-option v-for="px in proxies" :key="px.ID" :value="px.ID" :label="`${px.Scheme}://${px.Host}:${px.Port}`" />
      </t-select>
      <t-alert style="margin-top: 12px" theme="info" :message="$t('groups.hintBind')" />
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api } from '../api/client'
import type { GroupInfo } from '../api/types'

const { t } = useI18n()

const groups = ref<GroupInfo[]>([])
const plugins = ref<{ id: number; name: string; label?: string }[]>([])
const createVisible = ref(false)
const creating = ref(false)
const newName = ref('')
const newPlugin = ref<number | null>(null)

const proxies = ref<{ ID: number; Scheme: string; Host: string; Port: number }[]>([])
const bindVisible = ref(false)
const bindGroup = ref<GroupInfo | null>(null)
const bindProxyIds = ref<number[]>([])

const columns = computed(() => [
  { colKey: 'id', title: t('common.colId'), width: 70 },
  { colKey: 'name', title: t('common.colName'), align: 'center' },
  { colKey: 'plugin_label', title: t('groups.plugin'), align: 'center' },
  { colKey: 'accounts', title: t('groups.accounts'), width: 100, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 160, align: 'center' },
])

async function load() {
  const [g, p, px] = await Promise.all([
    api.get<{ groups: GroupInfo[] }>('/admin/groups'),
    api.get<{ plugins: { id: number; name: string; label?: string }[] }>('/admin/plugins'),
    api.get<{ proxies: typeof proxies.value }>('/admin/proxies'),
  ])
  groups.value = g.groups ?? []
  plugins.value = p.plugins ?? []
  proxies.value = px.proxies ?? []
}

async function openBind(row: GroupInfo) {
  bindGroup.value = row
  const resp = await api.get<{ proxy_ids: number[] }>(`/admin/groups/${row.id}/proxies`)
  bindProxyIds.value = resp.proxy_ids ?? []
  bindVisible.value = true
}

async function bind() {
  if (!bindGroup.value) return
  await api.put(`/admin/groups/${bindGroup.value.id}/proxies`, { proxy_ids: bindProxyIds.value })
  MessagePlugin.success(t('common.updated'))
  bindVisible.value = false
}

async function create() {
  if (!newName.value || !newPlugin.value) {
    MessagePlugin.warning(t('groups.errForm'))
    return
  }
  creating.value = true
  try {
    await api.post('/admin/groups', { name: newName.value, plugin_id: newPlugin.value })
    MessagePlugin.success(t('common.created'))
    createVisible.value = false
    newName.value = ''
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    creating.value = false
  }
}

async function remove(id: number) {
  await api.del(`/admin/groups/${id}`)
  await load()
}

onMounted(load)
</script>
