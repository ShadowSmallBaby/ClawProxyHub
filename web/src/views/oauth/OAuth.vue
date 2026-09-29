<template>
  <div class="page">
    <page-header v-if="!isPhone">
      <t-button theme="primary" @click="openCreate">{{ $t('oauth.create') }}</t-button>
    </page-header>
    <c-table row-key="id" :data="creds" :columns="columns" :loading="loading" mobile-cards>
      <template #has_token="{ row }">
        <t-tag v-if="row.has_token" theme="success" variant="light" size="small">{{ $t('oauth.tokenSet') }}</t-tag>
        <t-tag v-else theme="warning" variant="light" size="small">{{ $t('oauth.tokenEmpty') }}</t-tag>
      </template>
      <template #expires_at="{ row }">
        {{ row.expires_at ? fmtTime(row.expires_at) : $t('oauth.noExpiry') }}
      </template>
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="default" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
          <t-popconfirm :content="$t('oauth.confirmDelete')" @confirm="remove(row.id)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </c-table>

    <c-drawer
      v-model:visible="dialogVisible"
      :header="editingId ? $t('oauth.edit') : $t('oauth.create')"
      :confirm-btn="{ loading: saving }"
      @confirm="submit"
    >
      <t-form>
        <form-item :label="$t('oauth.platform')" mark>
          <t-select v-model="form.platform" :placeholder="$t('oauth.platformPh')" style="width: 100%">
            <t-option v-for="p in platformOptions" :key="p.value" :value="p.value" :label="p.label" />
          </t-select>
        </form-item>
        <form-item :label="$t('oauth.accountLabel')">
          <t-input v-model="form.account_label" :placeholder="$t('oauth.accountLabelPh')" />
        </form-item>
        <form-item :label="$t('oauth.token')" :mark="!editingId">
          <t-textarea
            v-model="form.token"
            :placeholder="editingId ? $t('oauth.tokenKeep') : $t('oauth.tokenPh')"
            :autosize="{ minRows: 2, maxRows: 5 }"
          />
        </form-item>
        <form-item :label="$t('oauth.expiresAt')">
          <t-input v-model="form.expires_at" :placeholder="$t('oauth.expiresAtPh')" />
        </form-item>
        <form-item :label="$t('oauth.extra')">
          <t-textarea v-model="form.extra_json" :placeholder="$t('oauth.extraPh')" :autosize="{ minRows: 2, maxRows: 4 }" />
        </form-item>
        <t-alert theme="info" :message="$t('oauth.hint')" />
      </t-form>
    </c-drawer>

    <mobile-fab v-if="isPhone">
      <t-button theme="primary" shape="circle" size="large" @click="openCreate">
        <template #icon><add-icon /></template>
      </t-button>
    </mobile-fab>
  </div>
</template>

<script setup lang="ts">
import { CDrawer } from '@/components/base'
import { FormItem } from '@/components'
import { CCard, CTable, MobileFab } from '@/components/base'
import PageHeader from '@/components/PageHeader.vue'
import { useAsync, useIsMobile } from '@/composables'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AddIcon } from 'tdesign-icons-vue-next'
import { MessagePlugin } from 'tdesign-vue-next'
import { oauthApi, type OAuthCred } from '@/api/entities'
import { fmtTime } from '@/utils/format'

const { t } = useI18n()
const { isPhone } = useIsMobile()

// 固定支持的第三方平台（value 落库，label 展示）
const platformOptions = [
  { value: 'linuxdo', label: 'LinuxDo' },
  { value: 'github', label: 'GitHub' },
]

const creds = ref<OAuthCred[]>([])
const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref(0) // 0 = 新建，>0 = 编辑
const form = reactive({ platform: '', account_label: '', token: '', expires_at: '', extra_json: '' })

const columns = computed(() => [
  { colKey: 'id', title: t('common.colId'), width: 70 },
  { colKey: 'platform', title: t('oauth.colPlatform'), align: 'center' },
  { colKey: 'account_label', title: t('oauth.colLabel'), align: 'center' },
  { colKey: 'has_token', title: t('oauth.colToken'), width: 110, align: 'center' },
  { colKey: 'expires_at', title: t('oauth.colExpires'), width: 180, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 140, align: 'center' },
])

function resetForm() {
  form.platform = ''; form.account_label = ''; form.token = ''; form.expires_at = ''; form.extra_json = ''
}

const { loading, run } = useAsync()

async function load() {
  await run(async () => {
    const resp = await oauthApi.list()
    creds.value = resp.credentials ?? []
  })
}

function openCreate() {
  editingId.value = 0
  resetForm()
  dialogVisible.value = true
}

// openEdit 回填（token 不回显，留空提交保留原值）
function openEdit(row: OAuthCred) {
  editingId.value = row.id
  form.platform = row.platform; form.account_label = row.account_label
  form.token = ''; form.expires_at = row.expires_at ?? ''; form.extra_json = row.extra_json || ''
  dialogVisible.value = true
}

async function submit() {
  if (!form.platform.trim()) {
    MessagePlugin.warning(t('oauth.errForm'))
    return
  }
  // 附加字段本地先校验 JSON，避免落库脏数据
  if (form.extra_json.trim()) {
    try { JSON.parse(form.extra_json) } catch {
      MessagePlugin.warning(t('oauth.errExtra'))
      return
    }
  }
  saving.value = true
  try {
    if (editingId.value > 0) {
      await oauthApi.update(editingId.value, { ...form })
      MessagePlugin.success(t('common.saved'))
    } else {
      await oauthApi.create({ ...form })
      MessagePlugin.success(t('common.created'))
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(id: number) {
  await oauthApi.remove(id)
  await load()
}

onMounted(load)
</script>
