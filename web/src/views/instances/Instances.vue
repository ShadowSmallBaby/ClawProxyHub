<template>
  <div class="page">
    <page-header>
      <t-select v-model="filterPlugin" clearable :placeholder="$t('instances.filterPlugin')" style="width: 200px" @change="load">
        <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
      </t-select>
      <t-button theme="primary" :disabled="!multiPlugins.length" @click="openCreate">{{ $t('instances.add') }}</t-button>
    </page-header>

    <c-table row-key="id" :data="list" :columns="columns" :loading="loading">
      <template #base_url="{ row }">
        <span class="mono">{{ row.base_url || '-' }}</span>
      </template>
      <template #op="{ row }">
        <t-space size="small">
          <t-link v-if="editable(row)" theme="primary" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
          <t-link theme="danger" @click="askRemove(row)">{{ $t('common.delete') }}</t-link>
        </t-space>
      </template>
    </c-table>

    <instance-form-dialog v-model:visible="dialogVisible" :plugin="dialogPlugin" :plugins="multiPlugins" :instance="editing" @saved="load" />
    <delete-impact-dialog
      v-model:visible="removeVisible"
      :header="$t('common.delete') + ' · ' + (removing?.name ?? '')"
      :message="$t('instances.confirmDelete')"
      :impact-url="`/admin/instances/${removing?.id ?? 0}/impact`"
      :delete-url="`/admin/instances/${removing?.id ?? 0}`"
      @deleted="load"
    />
  </div>
</template>

<script setup lang="ts">
import { CCard, CTable } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import { useAsync } from '../../composables'
import { pluginLabelOf } from '../../utils/lookup'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { pluginApi, instanceApi } from '../../api/entities'
import InstanceFormDialog from '../../components/InstanceFormDialog.vue'
import DeleteImpactDialog from '../../components/DeleteImpactDialog.vue'
import type { InstanceInfo, PluginInfo } from '../../api/types'

const { t } = useI18n()

const plugins = ref<PluginInfo[]>([])
const list = ref<InstanceInfo[]>([])
const filterPlugin = ref<number | undefined>(undefined)
const createPluginId = ref<number | undefined>(undefined)

const dialogVisible = ref(false)
const dialogPlugin = ref<PluginInfo | null>(null)
const editing = ref<InstanceInfo | null>(null)

const columns = computed(() => [
  { colKey: 'name', title: t('instances.colName'), width: 160, ellipsis: true },
  { colKey: 'plugin', title: t('instances.colPlugin'), width: 130, cell: (_h: any, { row }: any) => pluginLabel(row.plugin_id), align: 'center' },
  { colKey: 'base_url', title: t('instances.colBaseUrl'), ellipsis: true },
  { colKey: 'account_count', title: t('instances.colAccounts'), width: 90, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 120, align: 'center' },
])

const pluginLabel = (id: number) => pluginLabelOf(plugins.value, id)

// 可新建实例的插件：声明了 instances 能力（单例插件只有默认实例，可编辑不可新增）
const multiPlugins = computed(() => plugins.value.filter((p) => p.multi_instance))

const { loading, run } = useAsync()

async function load() {
  await run(async () => {
    const [p, i] = await Promise.all([
      pluginApi.list(),
      instanceApi.list(filterPlugin.value),
    ])
    plugins.value = p.plugins ?? []
    list.value = i.instances ?? []
  })
}

function openCreate() {
  dialogPlugin.value = null // 弹窗内选插件
  editing.value = null
  dialogVisible.value = true
}

// 默认实例（单实例插件）名称/地址系统固定；无附加设置字段则无可编辑项，隐藏编辑入口
function editable(row: InstanceInfo): boolean {
  const p = plugins.value.find((x) => x.id === row.plugin_id)
  if (!p) return true
  return !!p.multi_instance || !!p.instance_schema
}

function openEdit(row: InstanceInfo) {
  dialogPlugin.value = plugins.value.find((p) => p.id === row.plugin_id) ?? null
  editing.value = row
  dialogVisible.value = true
}

const removeVisible = ref(false)
const removing = ref<InstanceInfo | null>(null)

function askRemove(row: InstanceInfo) {
  removing.value = row
  removeVisible.value = true
}

onMounted(load)
</script>

<style scoped>
.mono {
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
</style>
