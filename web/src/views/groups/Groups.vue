<template>
  <div class="page">
    <page-header>
      
      <t-button theme="primary" @click="createVisible = true">{{ $t('groups.create') }}</t-button>
    </page-header>
    <c-table row-key="id" :data="groups" :columns="columns" :loading="loading">
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
          <t-link theme="primary" @click="openBind(row)">{{ $t('groups.bind') }}</t-link>
          <t-popconfirm :content="$t('groups.confirmDelete')" @confirm="remove(row.id)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </c-table>

    <c-dialog v-model:visible="createVisible" :header="$t('groups.create')" :confirm-btn="{ loading: creating }" @confirm="create">
      <t-form label-width="90px">
        <t-form-item :label="$t('groups.name')" mark>
          <t-input v-model="newName" :placeholder="$t('groups.namePh')" />
        </t-form-item>
        <t-form-item :label="$t('groups.plugin')" mark>
          <t-select v-model="newPlugin" :placeholder="$t('groups.pickPluginPh')" @change="onPluginChange">
            <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
          </t-select>
        </t-form-item>
        <t-form-item :label="$t('accounts.instance')" mark>
          <t-select v-model="newInstance" :options="instanceOptions(newPlugin)" :disabled="!selectedPlugin?.multi_instance" :placeholder="$t('groups.pickInstancePh')" />
        </t-form-item>
        <t-alert v-if="selectedPlugin?.multi_instance && !instanceOptions(newPlugin).length" theme="warning" :message="$t('accounts.noInstance')" />
        <t-alert v-else theme="info" :message="$t('groups.hintCreate')" />
      </t-form>
    </c-dialog>

    <!-- 编辑：改名；多实例插件且分组为空时可换实例 -->
    <c-dialog v-model:visible="editVisible" :header="$t('groups.editTitle')" :confirm-btn="{ loading: editing }" @confirm="submitEdit">
      <t-form v-if="editRow" label-width="90px">
        <t-form-item :label="$t('groups.name')" mark>
          <t-input v-model="editName" />
        </t-form-item>
        <t-form-item :label="$t('accounts.instance')">
          <t-select v-model="editInstance" :options="instanceOptions(editRow.plugin_id)" :disabled="!pluginOf(editRow.plugin_id)?.multi_instance || editRow.accounts > 0" />
        </t-form-item>
        <t-alert v-if="editRow.accounts > 0" theme="info" :message="$t('groups.hintEditLocked')" />
      </t-form>
    </c-dialog>

    <c-dialog v-model:visible="bindVisible" :header="$t('groups.bindHeader', { name: bindGroup?.name })" @confirm="bind">
      <bind-select v-model="bindProxyIds" :options="proxyOptions" :placeholder="$t('groups.bindPh')" />
      <t-alert style="margin-top: 12px" theme="info" :message="$t('groups.hintBind')" />
    </c-dialog>
  </div>
</template>

<script setup lang="ts">
import { CDialog } from '../../components/base'
import { CCard, CTable } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import { useAsync } from '../../composables'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { groupApi, instanceApi, pluginApi, proxyApi, type Proxy } from '../../api/entities'
import { instanceOptionsOf, instanceNameOf, proxyOptionsOf } from '../../utils/lookup'
import BindSelect from '../../components/BindSelect.vue'
import type { GroupInfo, InstanceInfo, PluginInfo } from '../../api/types'

const { t } = useI18n()

const groups = ref<GroupInfo[]>([])
const plugins = ref<PluginInfo[]>([])
const instances = ref<InstanceInfo[]>([])
const createVisible = ref(false)
const creating = ref(false)
const newName = ref('')
// t-select 绑定值用 undefined 作空值（0/null 会被当成真实选项显示）
const newPlugin = ref<number | undefined>(undefined)
const newInstance = ref<number | undefined>(undefined)

// 编辑弹窗
const editVisible = ref(false)
const editing = ref(false)
const editRow = ref<GroupInfo | null>(null)
const editName = ref('')
const editInstance = ref<number | undefined>(undefined)

const selectedPlugin = computed(() => plugins.value.find((p) => p.id === newPlugin.value))
function pluginOf(id: number) {
  return plugins.value.find((p) => p.id === id)
}
const instanceOptions = (pluginID: number | undefined) => instanceOptionsOf(instances.value, pluginID)
const instanceName = (id: number) => instanceNameOf(instances.value, id)
// 切换插件：按插件拉实例（保证默认实例存在），预选第一个；单实例插件即默认实例（不可改）
async function onPluginChange() {
  if (!newPlugin.value) return
  const resp = await instanceApi.list(newPlugin.value).catch(() => ({ instances: [] }))
  const list = resp.instances ?? []
  instances.value = [...instances.value.filter((i) => i.plugin_id !== newPlugin.value), ...list]
  newInstance.value = list[0]?.id
}

function openEdit(row: GroupInfo) {
  editRow.value = row
  editName.value = row.name
  editInstance.value = row.instance_id || undefined
  editVisible.value = true
}

async function submitEdit() {
  if (!editRow.value) return
  if (!editName.value.trim()) {
    MessagePlugin.warning(t('groups.errForm'))
    return
  }
  editing.value = true
  try {
    await groupApi.update(editRow.value.id, { name: editName.value.trim(), instance_id: editInstance.value ?? 0 })
    MessagePlugin.success(t('common.updated'))
    editVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    editing.value = false
  }
}

const proxies = ref<Proxy[]>([])
const bindVisible = ref(false)
const bindGroup = ref<GroupInfo | null>(null)
const bindProxyIds = ref<number[]>([])

const columns = computed(() => [
  { colKey: 'id', title: t('common.colId'), width: 70 },
  { colKey: 'name', title: t('common.colName'), align: 'center' },
  { colKey: 'plugin_label', title: t('groups.plugin'), align: 'center' },
  { colKey: 'instance', title: t('accounts.instance'), align: 'center', cell: (_h: any, { row }: any) => instanceName(row.instance_id) },
  { colKey: 'accounts', title: t('groups.accounts'), width: 100, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 200, align: 'center' },
])

const proxyOptions = computed(() => proxyOptionsOf(proxies.value))

const { loading, run } = useAsync()

// 全量基础数据（分组/插件/代理/实例）
async function load() {
  await run(async () => {
    const [g, p, px, ins] = await Promise.all([
      groupApi.list(), pluginApi.list(), proxyApi.list(), instanceApi.list(),
    ])
    groups.value = g.groups ?? []
    plugins.value = p.plugins ?? []
    proxies.value = px.proxies ?? []
    instances.value = ins.instances ?? []
  })
}

async function openBind(row: GroupInfo) {
  bindGroup.value = row
  const resp = await groupApi.proxies(row.id)
  bindProxyIds.value = resp.proxy_ids ?? []
  bindVisible.value = true
}

async function bind() {
  if (!bindGroup.value) return
  await groupApi.saveProxies(bindGroup.value.id, bindProxyIds.value)
  MessagePlugin.success(t('common.updated'))
  bindVisible.value = false
}

async function create() {
  if (!newName.value || !newPlugin.value || !newInstance.value) {
    MessagePlugin.warning(t('groups.errForm'))
    return
  }
  creating.value = true
  try {
    await groupApi.create({ name: newName.value, plugin_id: newPlugin.value, instance_id: newInstance.value ?? 0 })
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
  await groupApi.remove(id)
  await load()
}

onMounted(load)
</script>
