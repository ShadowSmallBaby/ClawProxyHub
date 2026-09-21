<template>
  <div class="page">
    <div class="page-header">

      <t-button theme="primary" @click="openCreate">{{ $t('tasks.create') }}</t-button>
      <t-button v-if="tab === 'runs'" theme="default" variant="outline" :loading="runsLoading" @click="refreshRuns">
        {{ $t('tasks.refreshRuns') }}
      </t-button>
    </div>

    <t-tabs v-model="tab" class="task-tabs">
      <t-tab-panel value="rules" :label="$t('tasks.tabRules')">
        <t-table row-key="id" :data="rules" :columns="ruleColumns" :max-height="tableHeight">
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
              <t-link theme="primary" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
              <t-popconfirm :content="$t('tasks.confirmDelete')" @confirm="removeRule(row.id)">
                <t-link theme="danger">{{ $t('common.delete') }}</t-link>
              </t-popconfirm>
            </t-space>
          </template>
        </t-table>
      </t-tab-panel>
      <t-tab-panel value="runs" :label="$t('tasks.tabRuns')">
        <t-table row-key="id" :data="runs" :columns="runColumns" :max-height="tableHeight">
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
    <t-pagination
      class="task-pagination"
      v-model="page"
      v-model:pageSize="pageSize"
      :total="tab === 'runs' ? runTotal : ruleTotal"
      :page-size-options="[10, 30, 50, 100, 200]"
      show-jumper
      @change="onPageChange"
      @page-size-change="onPageChange"
    />

    <t-dialog v-model:visible="createVisible" :header="editingId ? $t('tasks.editTitle') : $t('tasks.createTitle')" width="560px" :confirm-btn="{ loading: creating }" @confirm="submit">
      <t-form label-width="90px">
        <t-alert v-if="editingAuto" theme="info" :message="$t('tasks.autoLocked')" style="margin-bottom: 12px" />
        <t-form-item :label="$t('tasks.plugin')" mark>
          <t-select v-model="form.plugin_id" :disabled="!!editingId" :placeholder="$t('tasks.pluginPh')" @change="onPluginChange">
            <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
          </t-select>
        </t-form-item>
        <t-form-item :label="$t('tasks.capability')" mark>
          <t-select v-model="form.capability_id" :disabled="editingAuto || !form.plugin_id" :loading="capsLoading" :placeholder="$t('tasks.pickPluginPh')">
            <t-option v-for="c in capabilities" :key="c.id" :value="c.id" :label="c.label" />
          </t-select>
        </t-form-item>
        <t-form-item :label="$t('tasks.trigger')" mark>
          <div class="trigger-box">
            <div class="trigger-row">
              <t-select v-model="form.trigger_type" style="width: 110px" :disabled="editingAuto" :placeholder="$t('tasks.triggerPh')">
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
          <t-radio-group v-model="form.target_scope" variant="default-filled" :disabled="editingAuto">
            <t-radio-button value="all">{{ $t('tasks.scopeAll') }}</t-radio-button>
            <t-radio-button value="account_ids">{{ $t('tasks.scopeOne') }}</t-radio-button>
          </t-radio-group>
        </t-form-item>
        <t-form-item v-if="form.target_scope === 'account_ids'" :label="$t('tasks.account')">
          <t-select v-model="form.target_account" :disabled="editingAuto" :loading="acctsLoading" :placeholder="$t('tasks.pickAccountPh')" style="width: 100%">
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
const runsLoading = ref(false)
// 各 tab 独立分页参数（切换 tab 互不影响）
const rulePage = ref(1)
const rulePageSize = ref(50)
const runPage = ref(1)
const runPageSize = ref(50)
const runTotal = ref(0)
const ruleTotal = ref(0)
// 分页组件按当前 tab 绑定各自的分页状态
const page = computed({
  get: () => (tab.value === 'runs' ? runPage.value : rulePage.value),
  set: (v: number) => (tab.value === 'runs' ? (runPage.value = v) : (rulePage.value = v)),
})
const pageSize = computed({
  get: () => (tab.value === 'runs' ? runPageSize.value : rulePageSize.value),
  set: (v: number) => (tab.value === 'runs' ? (runPageSize.value = v) : (rulePageSize.value = v)),
})
// 数字高度才有效（百分比在 tab 面板嵌套 DOM 里算不出），视口减去页头/tab/分页/边距
const tableHeight = ref(window.innerHeight - 400)
const createVisible = ref(false)
const creating = ref(false)
const editingId = ref<number | null>(null) // null = 新建
const editingAuto = ref(false) // 编辑对象是否系统自动生成（锁定触发类型/能力/范围）
const form = reactive({
  plugin_id: undefined as number | undefined, capability_id: '', trigger_type: 'interval',
  trigger_value: '1h', target_scope: 'all', target_account: undefined as number | undefined,
})

function resetForm() {
  form.plugin_id = undefined
  form.capability_id = ''
  form.trigger_type = 'interval'
  form.trigger_value = '1h'
  form.target_scope = 'all'
  form.target_account = undefined
}

// 账号列表懒加载（执行范围下拉用）
async function loadAccounts() {
  if (accounts.value.length) return
  acctsLoading.value = true
  try {
    const resp = await api.get<{ accounts: { id: number; display_name: string }[] }>('/admin/accounts')
    accounts.value = resp.accounts ?? []
  } finally {
    acctsLoading.value = false
  }
}

// 拉某插件的任务能力（不清空当前选择，编辑回填用）
async function loadCapabilities(pluginId?: number) {
  const name = plugins.value.find((p) => p.id === pluginId)?.name
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

// 新建：清空表单
async function openCreate() {
  editingId.value = null
  editingAuto.value = false
  resetForm()
  capabilities.value = []
  createVisible.value = true
  await loadAccounts()
}

// 编辑：回填并按 auto 锁定字段（触发值始终可改）
async function openEdit(row: TaskRule) {
  editingId.value = row.id
  editingAuto.value = row.auto
  form.plugin_id = row.plugin_id
  form.capability_id = row.capability_id
  form.trigger_type = row.trigger_type
  form.trigger_value = row.trigger_type === 'once' ? fmtTriggerValue(row) : row.trigger_value
  form.target_scope = row.target_scope
  form.target_account = parseTargetIds(row.target_json)[0]
  createVisible.value = true
  await Promise.all([loadAccounts(), loadCapabilities(row.plugin_id)])
}

// account_ids 范围原始 id 列表（编辑回填用）
function parseTargetIds(raw: string): number[] {
  try {
    const v = JSON.parse(raw || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

// 新建时切插件：拉能力并清空已选能力
async function onPluginChange() {
  form.capability_id = ''
  capabilities.value = []
  if (!form.plugin_id) return
  await loadCapabilities(form.plugin_id)
}

const ruleColumns = computed(() => [
  { colKey: 'plugin', title: t('tasks.plugin'), width: 130 },
  { colKey: 'instance', title: t('tasks.colInstance'), width: 120, align: 'center', ellipsis: true, cell: (_h: any, { row }: any) => row.instance || (row.target_scope === 'account_ids' ? '-' : t('tasks.allInstances')) },
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
  { colKey: 'instance', title: t('tasks.colInstance'), width: 120, align: 'center', ellipsis: true, cell: (_h: any, { row }: any) => row.instance || '-' },
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

// 规则分页拉取（与执行历史各自独立分页）
async function loadRules() {
  const r = await api.get<{ rules: TaskRule[]; total: number }>(
    `/admin/task-rules?page=${page.value}&page_size=${pageSize.value}`,
  )
  rules.value = r.rules ?? []
  ruleTotal.value = r.total ?? 0
}

// 分页翻页：按当前 tab 刷新对应列表
async function onPageChange() {
  if (tab.value === 'runs') await refreshRuns()
  else await loadRules()
}

async function load() {
  const [, p] = await Promise.all([
    loadRules(),
    api.get<{ plugins: { id: number; name: string; label?: string }[] }>('/admin/plugins'),
  ])
  plugins.value = p.plugins ?? []
  await refreshRuns()
}

// 执行历史手动刷新 / 分页翻页
async function refreshRuns() {
  runsLoading.value = true
  try {
    const rn = await api.get<{ runs: TaskRun[]; total: number }>(
      `/admin/task-runs?page=${page.value}&page_size=${pageSize.value}`,
    )
    runs.value = rn.runs ?? []
    runTotal.value = rn.total ?? 0
  } finally {
    runsLoading.value = false
  }
}

// 新建 / 编辑共用：auto 规则后端只采纳 trigger_value，其余字段发了也忽略
async function submit() {
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
      target_json: form.target_scope === 'account_ids' ? JSON.stringify([form.target_account]) : '[]',
    }
    if (editingId.value) {
      await api.put(`/admin/task-rules/${editingId.value}`, payload)
      MessagePlugin.success(t('common.updated'))
    } else {
      await api.post('/admin/task-rules', payload)
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
.task-tabs {
  /* tab 区域固定高度，超出由表格内部滚动 */
  height: 780px;
  max-height: 780px;
}
.task-pagination {
  margin-top: 12px;
}
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

