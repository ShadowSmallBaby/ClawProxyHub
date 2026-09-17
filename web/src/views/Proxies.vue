<template>
  <div class="page">
    <div class="page-header">
      
      <t-button theme="primary" @click="createVisible = true">{{ $t('proxies.create') }}</t-button>
    </div>
    <t-table row-key="ID" :data="proxies" :columns="columns">
      <template #op="{ row }">
        <t-popconfirm :content="$t('proxies.confirmDelete')" @confirm="remove(row.ID)">
          <t-link theme="danger">{{ $t('common.delete') }}</t-link>
        </t-popconfirm>
      </template>
    </t-table>

    <t-dialog v-model:visible="createVisible" :header="$t('proxies.create')" :confirm-btn="{ loading: creating }" @confirm="create">
      <t-form label-width="80px">
        <t-form-item :label="$t('proxies.name')">
          <t-input v-model="form.name" :placeholder="$t('common.optional')" />
        </t-form-item>
        <t-form-item :label="$t('proxies.scheme')" mark>
          <t-radio-group v-model="form.scheme" variant="default-filled">
            <t-radio-button value="http">HTTP</t-radio-button>
            <t-radio-button value="https">HTTPS</t-radio-button>
            <t-radio-button value="socks5">SOCKS5</t-radio-button>
          </t-radio-group>
        </t-form-item>
        <t-form-item :label="$t('proxies.host')" mark>
          <t-input v-model="form.host" :placeholder="$t('proxies.hostPh')" />
        </t-form-item>
        <t-form-item :label="$t('proxies.port')" mark>
          <t-input-number v-model="form.port" :min="1" :max="65535" theme="column" style="width: 160px" />
        </t-form-item>
        <t-form-item :label="$t('proxies.username')">
          <t-input v-model="form.username" :placeholder="$t('common.optional')" />
        </t-form-item>
        <t-form-item :label="$t('proxies.password')">
          <t-input v-model="form.password" type="password" :placeholder="$t('common.optional')" />
        </t-form-item>
        <t-alert theme="info" :message="$t('proxies.hint')" />
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api } from '../api/client'

const { t } = useI18n()

interface Proxy {
  ID: number
  Name: string
  Scheme: string
  Host: string
  Port: number
  Username: string
}

const proxies = ref<Proxy[]>([])
const createVisible = ref(false)
const creating = ref(false)
const form = reactive({ name: '', scheme: 'http', host: '', port: 7890, username: '', password: '' })

const columns = computed(() => [
  { colKey: 'ID', title: t('common.colId'), width: 70 },
  { colKey: 'Name', title: t('common.colName'), align: 'center' },
  { colKey: 'Scheme', title: t('proxies.colScheme'), width: 90, align: 'center' },
  { colKey: 'Host', title: t('proxies.colHost'), align: 'center' },
  { colKey: 'Port', title: t('proxies.colPort'), width: 90, align: 'center' },
  { colKey: 'Username', title: t('proxies.colUser'), width: 120, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 100, align: 'center' },
])

async function load() {
  const resp = await api.get<{ proxies: Proxy[] }>('/admin/proxies')
  proxies.value = resp.proxies ?? []
}

async function create() {
  if (!form.host || !form.port) {
    MessagePlugin.warning(t('proxies.errForm'))
    return
  }
  creating.value = true
  try {
    await api.post('/admin/proxies', { ...form })
    MessagePlugin.success(t('common.created'))
    createVisible.value = false
    form.name = ''; form.host = ''; form.username = ''; form.password = ''
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    creating.value = false
  }
}

async function remove(id: number) {
  await api.del(`/admin/proxies/${id}`)
  await load()
}

onMounted(load)
</script>
