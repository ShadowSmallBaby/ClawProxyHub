<template>
  <div class="page">
    <div class="page-header">
      
      <t-button theme="primary" @click="openCreate">{{ $t('tasks.create') }}</t-button>
    </div>

    <t-tabs v-model="tab">
      <t-tab-panel value="rules" :label="$t('tasks.tabRules')">
        <t-table row-key="id" :data="rules" :columns="ruleColumns">
          <template #trigger="{ row }">
            <t-tag variant="light">{{ dict(triggerDict, row.trigger_type) }}</t-tag>
          </template>
          <template #accounts="{ row }">
            <t-tag v-for="a in row.accounts || ['-']" :key="a" size="small" variant="light" style="margin-right: 4px">
              {{ a }}
            </t-tag>
          </template>
          <template #enabled="{ row }">
            <t-switch :value="row.enabled" @change="(v: boolean) => toggle(row, v)" />
          </template>
          <template #op="{ row }">
            <t-space size="small">
              <t-link theme="primary" @click="run(row)">{{ $t('tasks.run') }}</t-link>
              <t-popconfirm :content="$t('tasks.confirmDelete')" @confirm="removeRule(row.id)">
                <t-link theme="danger">{{ $t('common.delete') }}</t-link>
              </t-popconfirm>
            </t-space>
          </template>
        </t-table>
      </t-tab-panel>
      <t-tab-panel value="runs" :label="$t('tasks.tabRuns')">
        <t-table row-key="id" :data="runs" :columns="runColumns">
          <template #status="{ row }">
            <!-- 错误信息并入状态 tooltip -->
            <t-tooltip
              v-if="row.status === 'failed' && row.error_message"
              :content="row.error_message"
              placement="top-left"
              :overlay-style="{ maxWidth: '640px', whiteSpace: 'pre-wrap' }"
            >
              <t-tag :theme="row.status === 'success' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'" variant="light">
                {{ dict(runStatusDict, row.status) }}
              </t-tag>
            </t-tooltip>
            <t-tag v-else :theme="row.status === 'success' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'" variant="light">
              {{ dict(runStatusDict, row.status) }}
            </t-tag>
          </template>
        </t-table>
      </t-tab-panel>
    </t-tabs>

    <t-dialog v-model:visible="createVisible" :header="$t('tasks.createTitle')" width="560px" :confirm-btn="{ loading: creating }" @confirm="create">
      <t-form label-width="90px">
        <t-form-item :label="$t('tasks.plugin')" mark>
          <t-select v-model="form.plugin_id" :placeholder="$t('tasks.pluginPh')" @change="onPluginChange">
            <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
          </t-select>
        </t-form-item>
        <t-form-item :label="$t('tasks.capability')" mark>
          <t-select v-model="form.capability_id" :disabled="!form.plugin_id" :loading="capsLoading" :placeholder="$t('tasks.pickPluginPh')">
            <t-option v-for="c in capabilities" :key="c.id" :value="c.id" :label="c.label" />
          </t-select>
        </t-form-item>
        <t-form-item :label="$t('tasks.trigger')" mark>
          <div class="trigger-box">
            <div class="trigger-row">
              <t-select v-model="form.trigger_type" style="width: 110px" :placeholder="$t('tasks.triggerPh')">
                <t-option value="interval" :label="$t('tasks.triggerInterval')" />
                <t-option value="daily" :label="$t('tasks.triggerDaily')" />
                <t-option value="once" :label="$t('tasks.triggerOnce')" />
              </t-select>
              <t-date-picker
                v-if="form.trigger_type === 'once'"
                v-model="form.trigger_value"
                class="trigger-value"
                enable-time-picker
                allow-input
                clearable
                format="YYYY-MM-DD HH:mm"
                :placeholder="$t('tasks.pickTime')"
              />
              <t-input v-else v-model="form.trigger_value" class="trigger-value" :placeholder="triggerPh" />
            </div>
            <div class="trigger-hint">{{ triggerHint }}</div>
          </div>
        </t-form-item>
        <t-form-item :label="$t('tasks.scope')">
          <t-radio-group v-model="form.target_scope" variant="default-filled">
            <t-radio-button value="all">{{ $t('tasks.scopeAll') }}</t-radio-button>
            <t-radio-button value="account_ids">{{ $t('tasks.scopeOne') }}</t-radio-button>
          </t-radio-group>
        </t-form-item>
        <t-form-item v-if="form.target_scope === 'account_ids'" :label="$t('tasks.account')">
          <t-select v-model="form.target_account" :loading="acctsLoading" :placeholder="$t('tasks.pickAccountPh')" style="width: 100%">
            <t-option v-for="a in accounts" :key="a.id" :value="a.id" :label="a.display_name || `#${a.id}`" />
          </t-select>
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api } from '../api/client'
import { dict, runStatusDict, triggerDict } from '../utils/dict'
import type { TaskRule, TaskRun } from '../api/types'

const { t } = useI18n()

const tab = ref('rules')
const rules = ref<TaskRule[]>([])
const runs = ref<TaskRun[]>([])
const plugins = ref<{ id: number; name: string; label?: string }[]>([])
const capabilities = ref<{ id: string; label: string }[]>([])
const capsLoading = ref(false)
const accounts = ref<{ id: number; display_name: string }[]>([])
const acctsLoading = ref(false)
const createVisible = ref(false)
const creating = ref(false)
const form = reactive({
  plugin_id: undefined, capability_id: '', trigger_type: 'interval',
  trigger_value: '1h', target_scope: 'all', target_account: undefined,
})

// 新建规则打开时预载账号列表（执行范围用）
async function openCreate() {
  createVisible.value = true
  if (!accounts.value.length) {
    acctsLoading.value = true
    try {
      const resp = await api.get<{ accounts: { id: number; display_name: string }[] }>('/admin/accounts')
      accounts.value = resp.accounts ?? []
    } finally {
      acctsLoading.value = false
    }
  }
}

// 切换插件：拉取该插件声明的任务能力，清空已选能力
async function onPluginChange() {
  form.capability_id = ''
  capabilities.value = []
  if (!form.plugin_id) return
  const name = plugins.value.find((p) => p.id === form.plugin_id)?.name
  if (!name) return
  capsLoading.value = true
  try {
    const resp = await api.get<{ capabilities: { id: string; label: string }[] }>(
      `/admin/plugins/${name}/task-capabilities`,
    )
    capabilities.value = resp.capabilities ?? []
  } finally {
    capsLoading.value = false
  }
}

const ruleColumns = computed(() => [
  { colKey: 'plugin', title: t('tasks.plugin'), width: 130 },
  { colKey: 'capability', title: t('tasks.colTask'), align: 'center' },
  { colKey: 'trigger', title: t('tasks.colTrigger'), width: 100, align: 'center' },
  { colKey: 'trigger_value', title: t('tasks.colTriggerValue'), width: 150, align: 'center', cell: (_h: any, { row }: any) => fmtTriggerValue(row) },
  { colKey: 'accounts', title: t('tasks.colAccounts'), width: 160, align: 'center' },
  { colKey: 'enabled', title: t('tasks.colStatus'), width: 90, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 190, align: 'center' },
])

// 插件品牌名映射（新建规则弹窗用）
function pluginLabel(pluginID: number): string {
  const p = plugins.value.find((x) => x.id === pluginID)
  return p?.label || p?.name || `#${pluginID}`
}

const runColumns = computed(() => [
  { colKey: 'plugin', title: t('tasks.plugin'), width: 170 },
  { colKey: 'capability', title: t('tasks.colTask'), width: 120, align: 'center' },
  { colKey: 'account', title: t('tasks.colAccount'), width: 140, cell: (_h: any, { row }: any) => row.account || '-', align: 'center' },
  { colKey: 'status', title: t('tasks.colResult'), width: 90, align: 'center' },
  { colKey: 'summary', title: t('tasks.colSummary'), ellipsis: true, align: 'center' },
  { colKey: 'started_at', title: t('common.colTime'), width: 190, cell: (_h: any, { row }: any) => row.started_at?.replace('T', ' ').slice(0, 19) ?? '-', align: 'center' },
])

// 触发值输入框 placeholder（短示例）；once 走日期时间选择器，格式说明在行下方
const triggerPh = computed(() => ({
  interval: '1h',
  daily: '09:00',
}[form.trigger_type] ?? ''))
const triggerHint = computed(() => ({
  interval: t('tasks.hintInterval'),
  daily: t('tasks.hintDaily'),
  once: t('tasks.hintOnce'),
}[form.trigger_type] ?? ''))

// "2026-10-01 12:10" → 本地时区 RFC3339（后端按 RFC3339 计算下次触发）
function onceToRFC3339(v: string): string {
  const m = v.trim().match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?$/)
  if (!m) return v
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] ?? 0))
  const pad = (n: number) => String(n).padStart(2, '0')
  const off = -d.getTimezoneOffset()
  const a = Math.abs(off)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}${off >= 0 ? '+' : '-'}${pad(Math.floor(a / 60))}:${pad(a % 60)}`
}

// 列表展示：once 的 RFC3339 转回本地 "YYYY-MM-DD HH:mm"
function fmtTriggerValue(row: TaskRule): string {
  if (row.trigger_type !== 'once') return row.trigger_value
  const d = new Date(row.trigger_value)
  if (isNaN(d.getTime())) return row.trigger_value
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

async function load() {
  const [r, rn, p] = await Promise.all([
    api.get<{ rules: TaskRule[] }>('/admin/task-rules'),
    api.get<{ runs: TaskRun[] }>('/admin/task-runs'),
    api.get<{ plugins: { id: number; name: string; label?: string }[] }>('/admin/plugins'),
  ])
  rules.value = r.rules ?? []
  runs.value = rn.runs ?? []
  plugins.value = p.plugins ?? []
}

async function create() {
  if (!form.plugin_id || !form.capability_id) {
    MessagePlugin.warning(t('tasks.errForm'))
    return
  }
  if (form.trigger_type === 'once' && !form.trigger_value) {
    MessagePlugin.warning(t('tasks.errTrigger'))
    return
  }
  if (form.target_scope === 'account_ids' && !form.target_account) {
    MessagePlugin.warning(t('tasks.errAccount'))
    return
  }
  creating.value = true
  try {
    const payload: Record<string, any> = {
      ...form,
      target_account: undefined,
      trigger_value: form.trigger_type === 'once' ? onceToRFC3339(form.trigger_value) : form.trigger_value,
    }
    if (form.target_scope === 'account_ids') {
      payload.target_json = JSON.stringify([form.target_account])
    } else {
      payload.target_json = '[]'
    }
    await api.post('/admin/task-rules', payload)
    MessagePlugin.success(t('common.created'))
    createVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    creating.value = false
  }
}

async function toggle(rule: TaskRule, enabled: boolean) {
  await api.post(`/admin/task-rules/${rule.id}/toggle`)
  rule.enabled = enabled
}

async function run(rule: TaskRule) {
  await api.post(`/admin/task-rules/${rule.id}/run`)
  MessagePlugin.success(t('tasks.queued'))
  setTimeout(load, 2000)
}

async function removeRule(id: number) {
  await api.del(`/admin/task-rules/${id}`)
  await load()
}

onMounted(load)
</script>

<style scoped>
.trigger-box {
  width: 100%;
}
.trigger-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.trigger-value {
  flex: 1;
}
.trigger-hint {
  margin-top: 4px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  line-height: 1.5;
}
</style>

