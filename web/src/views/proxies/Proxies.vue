<template>
  <div class="page">
    <page-header>
      
      <t-button theme="primary" @click="openCreate">{{ $t('proxies.create') }}</t-button>
    </page-header>
    <c-table row-key="ID" :data="proxies" :columns="columns" :loading="loading">
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" :loading="testingId === row.ID" @click="test(row)">{{ $t('proxies.test') }}</t-link>
          <t-link theme="default" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
          <t-popconfirm :content="$t('proxies.confirmDelete')" @confirm="remove(row.ID)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </c-table>

    <c-dialog v-model:visible="createVisible" :header="editingId ? $t('proxies.edit') : $t('proxies.create')" :confirm-btn="{ loading: creating }" @confirm="submit">
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
          <t-input v-model="form.password" type="password" :placeholder="editingId ? $t('proxies.pwdKeep') : $t('common.optional')" />
        </t-form-item>
        <t-alert theme="info" :message="$t('proxies.hint')" />
      </t-form>
    </c-dialog>
  </div>
</template>

<script setup lang="ts">
import { CDialog } from '../../components/base'
import { CCard, CTable } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import { useAsync } from '../../composables'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { proxyApi, type Proxy } from '../../api/entities'

const { t } = useI18n()

const proxies = ref<Proxy[]>([])
const createVisible = ref(false)
const creating = ref(false)
const editingId = ref(0) // 0 = 新建，>0 = 编辑该代理
const testingId = ref(0) // 正在测试的代理 ID（行内 loading）
const form = reactive({ name: '', scheme: 'http', host: '', port: 7890, username: '', password: '' })

function resetForm() {
  form.name = ''; form.scheme = 'http'; form.host = ''; form.port = 7890; form.username = ''; form.password = ''
}

const columns = computed(() => [
  { colKey: 'ID', title: t('common.colId'), width: 70 },
  { colKey: 'Name', title: t('common.colName'), align: 'center' },
  { colKey: 'Scheme', title: t('proxies.colScheme'), width: 90, align: 'center' },
  { colKey: 'Host', title: t('proxies.colHost'), align: 'center' },
  { colKey: 'Port', title: t('proxies.colPort'), width: 90, align: 'center' },
  { colKey: 'Username', title: t('proxies.colUser'), width: 120, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 180, align: 'center' },
])

const { loading, run } = useAsync()

async function load() {
  await run(async () => {
    const resp = await proxyApi.list()
    proxies.value = resp.proxies ?? []
  })
}

function openCreate() {
  editingId.value = 0
  resetForm()
  createVisible.value = true
}

// openEdit 回填已有代理（密码不回显，留空提交则保留原值）
function openEdit(row: Proxy) {
  editingId.value = row.ID
  form.name = row.Name; form.scheme = row.Scheme; form.host = row.Host
  form.port = row.Port; form.username = row.Username; form.password = ''
  createVisible.value = true
}

// submit 新建 / 编辑分流：editingId>0 走 PUT
async function submit() {
  if (!form.host || !form.port) {
    MessagePlugin.warning(t('proxies.errForm'))
    return
  }
  creating.value = true
  try {
    if (editingId.value > 0) {
      await proxyApi.update(editingId.value, { Name: form.name, Scheme: form.scheme, Host: form.host, Port: form.port, Username: form.username, password: form.password })
      MessagePlugin.success(t('common.saved'))
    } else {
      await proxyApi.create({ Name: form.name, Scheme: form.scheme, Host: form.host, Port: form.port, Username: form.username, password: form.password })
      MessagePlugin.success(t('common.created'))
    }
    createVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    creating.value = false
  }
}

// test 经该代理拨中立目标，回显连通性与时延
async function test(row: Proxy) {
  testingId.value = row.ID
  try {
    const r = await proxyApi.test(row.ID)
    if (r.ok) {
      MessagePlugin.success(t('proxies.testOk', { ms: r.latency_ms ?? 0 }))
    } else {
      MessagePlugin.error(t('proxies.testFail', { err: r.error ?? '' }))
    }
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    testingId.value = 0
  }
}

async function remove(id: number) {
  await proxyApi.remove(id)
  await load()
}

onMounted(load)
</script>
