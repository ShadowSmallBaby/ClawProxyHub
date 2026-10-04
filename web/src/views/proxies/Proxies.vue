<template>
  <page-layout :body-key="`${page}:${pageSize}`" :scroll="false">
    <template v-if="!isPhone" #header>
      <page-header>
      
        <t-button theme="primary" @click="openCreate">{{ $t('proxies.create') }}</t-button>
      </page-header>
    </template>
    <c-table fill row-key="id" :data="pageItems" :columns="columns" :loading="loading" mobile-cards>
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" :loading="testingId === row.id" @click="test(row)">{{ $t('proxies.test') }}</t-link>
          <t-link theme="default" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
          <t-popconfirm :content="$t('proxies.confirmDelete')" @confirm="remove(row.id)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </c-table>
    <template #footer>
      <c-pagination v-model="page" v-model:pageSize="pageSize" :total="total" />
    </template>
    <template #overlays>


    <c-drawer v-model:visible="createVisible" :header="editingId ? $t('proxies.edit') : $t('proxies.create')" :confirm-btn="{ loading: creating }" @confirm="submit">
      <t-form>
        <form-item :label="$t('proxies.name')">
          <t-input v-model="form.name" :placeholder="$t('common.optional')" />
        </form-item>
        <form-item :label="$t('proxies.scheme')" mark>
          <t-radio-group v-model="form.scheme" variant="default-filled">
            <t-radio-button value="http">HTTP</t-radio-button>
            <t-radio-button value="https">HTTPS</t-radio-button>
            <t-radio-button value="socks5">SOCKS5</t-radio-button>
          </t-radio-group>
        </form-item>
        <form-item :label="$t('proxies.host')" mark>
          <t-input v-model="form.host" :placeholder="$t('proxies.hostPh')" />
        </form-item>
        <form-item :label="$t('proxies.port')" mark>
          <t-input-number v-model="form.port" :min="1" :max="65535" theme="column" class="w-sm" />
        </form-item>
        <form-item :label="$t('proxies.username')">
          <t-input v-model="form.username" :placeholder="$t('common.optional')" />
        </form-item>
        <form-item :label="$t('proxies.password')">
          <t-input v-model="form.password" type="password" :placeholder="editingId ? $t('proxies.pwdKeep') : $t('common.optional')" />
        </form-item>
        <t-alert theme="info" :message="$t('proxies.hint')" />
      </t-form>
    </c-drawer>

    <mobile-fab v-if="isPhone">
      <t-button theme="primary" shape="circle" size="large" @click="openCreate">
        <template #icon><add-icon /></template>
      </t-button>
    </mobile-fab>
    </template>
  </page-layout>
</template>

<script setup lang="ts">
import { PageLayout, PageHeader } from '@/components'
import { CPagination } from '@/components/base'
import { useClientPagination } from '@/composables'
import { CDrawer } from '@/components/base'
import { FormItem } from '@/components'
import { CCard, CTable, MobileFab } from '@/components/base'
import { useAsync, useIsMobile } from '@/composables'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AddIcon } from 'tdesign-icons-vue-next'
import { MessagePlugin } from 'tdesign-vue-next'
import { proxyApi, type Proxy } from '@/api/entities'

const { t } = useI18n()
const { isPhone } = useIsMobile()

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
  { colKey: 'id', title: t('common.colId'), width: 70 },
  { colKey: 'name', title: t('common.colName'), align: 'center' },
  { colKey: 'scheme', title: t('proxies.colScheme'), width: 90, align: 'center' },
  { colKey: 'host', title: t('proxies.colHost'), align: 'center' },
  { colKey: 'port', title: t('proxies.colPort'), width: 90, align: 'center' },
  { colKey: 'username', title: t('proxies.colUser'), width: 120, align: 'center' },
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
  editingId.value = row.id
  form.name = row.name; form.scheme = row.scheme; form.host = row.host
  form.port = row.port; form.username = row.username; form.password = ''
  createVisible.value = true
}

// submit 新建 / 编辑分流：editingId>0 走 PUT
async function submit() {
  if (!form.host.trim() || form.port < 1 || form.port > 65535) {
    MessagePlugin.warning(t('proxies.errForm'))
    return
  }
  creating.value = true
  try {
    if (editingId.value > 0) {
      await proxyApi.update(editingId.value, { name: form.name, scheme: form.scheme, host: form.host, port: form.port, username: form.username, password: form.password })
      MessagePlugin.success(t('common.saved'))
    } else {
      await proxyApi.create({ name: form.name, scheme: form.scheme, host: form.host, port: form.port, username: form.username, password: form.password })
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
  testingId.value = row.id
  try {
    const r = await proxyApi.test(row.id)
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

const { page, pageSize, total, items: pageItems } = useClientPagination(proxies)
</script>
