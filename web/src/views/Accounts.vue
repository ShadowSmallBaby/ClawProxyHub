<template>
  <div class="page">
    <div class="page-header">
      
      <t-button theme="primary" :disabled="!plugins.length" @click="openAdd">{{ $t('accounts.add') }}</t-button>
    </div>

    <t-table row-key="id" :data="accounts" :columns="columns" :loading="loading">
      <template #display_name="{ row }">
        <span class="acct-name" @click="openDetail(row.id)">{{ row.display_name || `#${row.id}` }}</span>
      </template>
      <template #group="{ row }">
        <div class="group-tags" @click.stop>
          <t-tag
            v-for="gid in row.group_ids ?? []"
            :key="gid"
            size="small"
            variant="light"
            closable
            @close="() => toggleGroup(row, gid, false)"
          >
            {{ groupName(gid) }}
          </t-tag>
          <t-popup trigger="click">
            <t-tag size="small" theme="default" variant="light" class="group-add">＋</t-tag>
            <template #content>
              <div class="group-picker">
                <div v-if="!groupsOf(row).length" class="group-picker-empty">{{ $t('accounts.noGroups') }}</div>
                <div
                  v-for="g in groupsOf(row)"
                  :key="g.id"
                  class="group-picker-item"
                  :class="{ active: (row.group_ids ?? []).includes(g.id) }"
                  @click="() => toggleGroup(row, g.id, !(row.group_ids ?? []).includes(g.id))"
                >
                  {{ g.name }}
                  <check-icon v-if="(row.group_ids ?? []).includes(g.id)" />
                </div>
              </div>
            </template>
          </t-popup>
        </div>
      </template>
      <template #schedule="{ row }">
        <t-tooltip v-if="pausedInfo(row)" :content="pausedInfo(row)!" placement="top">
          <t-tag theme="warning" variant="light">{{ pausedLabel(row) }}</t-tag>
        </t-tooltip>
        <t-tooltip v-else :content="row.status === 'disabled' && row.pause_reason ? row.pause_reason : ''" :disabled="!(row.status === 'disabled' && row.pause_reason)" placement="top">
          <t-switch
            :value="row.status === 'active'"
            size="small"
            :disabled="row.status === 'expired'"
            @change="() => toggleSchedule(row)"
          />
        </t-tooltip>
      </template>
      <template #status="{ row }">
        <t-tag v-if="row.status === 'active'" theme="success" variant="light">{{ $t('accounts.statusActive') }}</t-tag>
        <t-tag v-else :theme="row.status === 'expired' ? 'danger' : 'default'" variant="light">
          {{ dict(accountStatusDict, row.status) }}
        </t-tag>
      </template>
      <template #credits="{ row }">
        <div v-if="row.credits" class="credit-cell">
          <span>{{ $t('accounts.remaining') }}: {{ fmtNum(row.credits.remaining) }}</span>
          <span>{{ $t('accounts.totalCredits') }}: {{ fmtNum(row.credits.total) }}</span>
        </div>
        <span v-else>-</span>
      </template>
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="openEdit(row)">{{ $t('common.edit') }}</t-link>
          <t-link theme="primary" @click="openTest(row)">{{ $t('accounts.test') }}</t-link>
          <t-link theme="primary" @click="refresh(row.id)">{{ $t('common.refresh') }}</t-link>
          <t-link theme="danger" @click="askRemove(row)">{{ $t('common.delete') }}</t-link>
        </t-space>
      </template>
    </t-table>

    <!-- 账号详情：套餐/积分 + 任务执行情况 -->
    <t-drawer v-model:visible="detailVisible" :header="detailHeader" size="720px">
      <t-space v-if="detail" direction="vertical" style="width: 100%" size="large">
        <t-descriptions :column="1" bordered size="small">
          <t-descriptions-item :label="$t('accounts.account')">{{ detail.display_name || `#${detail.id}` }}</t-descriptions-item>
          <t-descriptions-item :label="$t('accounts.status')">{{ dict(accountStatusDict, detail.status) }}</t-descriptions-item>
          <t-descriptions-item v-if="detail.pause_reason" :label="$t('accounts.pauseReason')">{{ detail.pause_reason }}</t-descriptions-item>
          <t-descriptions-item v-if="detail.paused_until && !detail.manual_pause" :label="$t('accounts.resumeAt')">
            {{ detail.paused_until?.replace('T', ' ').slice(0, 19) }}
          </t-descriptions-item>
          <t-descriptions-item :label="$t('accounts.lastRefresh')">{{ fmtTime(detail.last_refresh_at) }}</t-descriptions-item>
          <t-descriptions-item :label="$t('accounts.lastUsed')">{{ fmtTime(detail.last_used_at) }}</t-descriptions-item>
          <t-descriptions-item :label="$t('accounts.credits')">{{ creditSummaryLine }}</t-descriptions-item>
        </t-descriptions>

        <!-- 动态渲染块：插件声明的 ProfileSection（签到 / 成长计划 / 积分包…），不定义即不渲染 -->
        <div v-for="sec in sections" :key="sec.id">
          <div class="section-title">{{ label(sec.title, sec.id) }}</div>
          <t-descriptions v-if="sec.entries?.length" :column="1" bordered size="small">
            <t-descriptions-item v-for="(e, i) in sec.entries" :key="i" :label="label(e.label, '')">
              <t-tag v-if="isStatus(e.value)" :theme="sectionStatusTheme(e.value)" variant="light" size="small">
                {{ statusValue(e.value) }}
              </t-tag>
              <template v-else>{{ e.value }}</template>
            </t-descriptions-item>
          </t-descriptions>
          <t-table
            v-if="sec.items?.length && sec.columns?.length"
            :data="sec.items"
            size="small"
            height="260"
            :row-key="(_r: Record<string, string>, i?: number) => String(i)"
            :columns="sectionColumns(sec.columns)"
            :show-header="true"
          >
            <template #section-cell="{ col, row }">
              <t-tag v-if="col.kind === 'status'" :theme="sectionStatusTheme(row.cells?.[col.colKey])" variant="light" size="small">
                {{ statusValue(row.cells?.[col.colKey]) }}
              </t-tag>
              <template v-else>{{ row.cells?.[col.colKey] || '-' }}</template>
            </template>
          </t-table>
        </div>

        <div>
          <div class="section-title">{{ $t('accounts.runsTitle') }}</div>
          <t-table
            v-if="detail.runs?.length"
            row-key="ID"
            :data="detail.runs"
            size="small"
            :columns="detailRunColumns"
            :show-header="true"
          >
            <template #run-status="{ row: run }">
              <t-tag :theme="run.status === 'success' ? 'success' : run.status === 'failed' ? 'danger' : 'warning'" variant="light">
                {{ dict(runStatusDict, run.status) }}
              </t-tag>
            </template>
          </t-table>
          <t-empty v-else :description="$t('accounts.noRuns')" />
        </div>
      </t-space>
    </t-drawer>

    <!-- 添加账号：向导（选择客户端 → 授权 → 配置） -->
    <t-dialog v-model:visible="addVisible" :header="$t('accounts.add')" :footer="false" width="680px" :close-on-overlay-click="false">

      <!-- 第一步：选择客户端（卡片平铺，每行四个） -->
      <template v-if="wizardStep === 'select'">
        <t-empty v-if="!plugins.length" :description="$t('accounts.noPlugins')" />
        <t-row v-else :gutter="[12, 12]">
          <t-col v-for="p in plugins" :key="p.id" :span="6">
            <div class="client-card" @click="choosePlugin(p)">
              <div class="client-icon">
                <img v-if="p.icon" :src="p.icon" :alt="p.label || p.name" />
                <span v-else>{{ (p.label || p.name).slice(0, 1) }}</span>
              </div>
              <div class="client-name">{{ p.label || p.name }}</div>
              <div class="client-caps">
                <t-tag v-for="c in (p.capabilities ?? []).slice(0, 3)" :key="c" size="small" variant="light">
                  {{ dict(capabilityDict, c) }}
                </t-tag>
              </div>
            </div>
          </t-col>
        </t-row>
      </template>

      <!-- 第二步：授权（tab = 插件声明的登录方式，表单按 schema 动态渲染） -->
      <t-space v-else-if="wizardStep === 'auth'" direction="vertical" style="width: 100%" size="large">
        <div class="wizard-back">
          <t-link theme="primary" @click="wizardStep = 'select'">{{ $t('accounts.backToSelect') }}</t-link>
          <span class="wizard-client">{{ selectedPluginLabel }}</span>
        </div>

        <!-- 目标实例（站点）：仅多实例插件展示；无实例时先去新建 -->
        <t-alert v-if="selectedPlugin?.multi_instance && !instanceOptions(selectedPluginId).length" theme="warning">
          <template #message>
            {{ $t('accounts.noInstance') }}
            <t-link theme="primary" @click="router.push('/instances')">{{ $t('menu.instances') }}</t-link>
          </template>
        </t-alert>
        <t-form v-else-if="selectedPlugin?.multi_instance" label-width="90px">
          <t-form-item :label="$t('accounts.instance')">
            <t-select v-model="wizardInstanceId" :options="instanceOptions(selectedPluginId)" style="width: 100%" />
          </t-form-item>
        </t-form>

        <t-tabs v-if="methods.length" v-model="methodId">
          <t-tab-panel v-for="m in methods" :key="m.id" :value="m.id" :label="label(m.label, m.id)">
            <div class="tab-body">
              <t-form v-if="currentFields?.length" label-width="90px">
                <t-form-item v-for="f in currentFields" :key="f.name" :label="f.type === 'textarea' ? '' : label(f.label, f.name)" :label-width="f.type === 'textarea' ? 0 : 90" :mark="f.required && f.type !== 'textarea'">
                  <div
                    v-if="f.type === 'textarea'"
                    class="drop-zone"
                    @drop.prevent="onDrop($event, f.name)"
                    @dragover.prevent
                  >
                    <t-textarea
                      v-model="form[f.name]"
                      :placeholder="f.placeholder || $t('accounts.pastePh')"
                      :autosize="{ minRows: 6, maxRows: 12 }"
                      class="scroll-textarea"
                    />
                    <span class="drop-hint">{{ $t('accounts.dropHint') }}</span>
                  </div>
                  <t-input                    v-else
                    v-model="form[f.name]"
                    :type="f.type === 'password' ? 'password' : 'text'"
                    :placeholder="f.placeholder"
                  />
                </t-form-item>
              </t-form>
              <t-alert v-else theme="info" :message="$t('accounts.noFieldsHint')" />
            </div>
          </t-tab-panel>
        </t-tabs>

        <!-- 浏览器授权：链接可复制可打开；auto 模式自动轮询 -->
        <t-alert v-if="nextStep" :theme="nextStep.action === 'open_url' ? 'warning' : 'info'">
          <template #message>
            <div>{{ label(nextStep.prompt, '') }}</div>
            <div v-if="nextStep.wait" class="mode-hint">
              {{ showCallbackInput ? $t('accounts.callbackManual') : $t('accounts.callbackAuto') }}
            </div>
            <div v-if="nextStep.url" class="login-url">
              <span class="login-url-text">{{ nextStep.url }}</span>
              <t-space size="small">
                <t-link theme="primary" @click="copyText(nextStep.url!)">{{ $t('accounts.copy') }}</t-link>
                <t-link theme="primary" @click="openURL(nextStep.url!)">{{ $t('accounts.open') }}</t-link>
              </t-space>
            </div>
          </template>
        </t-alert>
        <t-form v-if="nextStep?.action === 'input_form' && nextStep.fields?.length" label-width="90px">
          <t-form-item v-for="f in nextStep.fields" :key="f.name" :label="label(f.label, f.name)" :mark="f.required">
            <t-textarea
              v-if="f.type === 'textarea'"
              v-model="stepForm[f.name]"
              :placeholder="f.placeholder"
              :autosize="{ minRows: 2, maxRows: 6 }"
              class="scroll-textarea"
            />
            <t-input v-else v-model="stepForm[f.name]" :placeholder="f.placeholder" />
          </t-form-item>
        </t-form>
        <t-form v-else-if="nextStep?.action === 'open_url' && nextStep.fields?.length && (!nextStep.wait || showCallbackInput)" label-width="90px">
          <t-form-item v-for="f in nextStep.fields" :key="f.name" :label="label(f.label, f.name)" :mark="f.required">
            <t-textarea
              v-model="stepForm[f.name]"
              :placeholder="f.placeholder"
              :autosize="{ minRows: 2, maxRows: 6 }"
              class="scroll-textarea"
            />
          </t-form-item>
        </t-form>

        <t-button theme="primary" block :loading="submitting" :disabled="selectedPlugin?.multi_instance && !wizardInstanceId" @click="submit">
          {{ submitLabel }}
        </t-button>
      </t-space>

      <!-- 第三步：授权成功 → 基本信息 / 模型列表 / 分组 -->
      <t-space v-else direction="vertical" style="width: 100%" size="large">
        <t-alert theme="success" :message="$t('accounts.successHint')" />
        <div>
          <div class="section-title">{{ $t('accounts.basicInfo') }}</div>
          <t-form label-width="90px">
            <t-form-item :label="$t('accounts.name')">
              <t-input v-model="newAccountName" :placeholder="wizardProfileName ? $t('accounts.namePh', { name: wizardProfileName }) : $t('accounts.namePhNone')" />
            </t-form-item>
          </t-form>
        </div>
        <div>
          <div class="section-title">
            {{ $t('accounts.modelsTitle') }}
            <t-link theme="primary" style="margin-left: 8px" @click="syncModels">{{ modelsSyncing ? $t('accounts.syncing') : $t('accounts.sync') }}</t-link>
          </div>
          <div v-if="wizardModels.length" class="model-list">
            <t-tag v-for="m in wizardModels" :key="m.id" variant="light-outline" style="margin: 0 6px 6px 0">{{ m.id }}</t-tag>
          </div>
          <span v-else class="hint">{{ $t('accounts.noModels') }}</span>
        </div>
        <div>
          <div class="section-title">{{ $t('accounts.groupsTitle') }}</div>
          <bind-select v-model="newAccountGroups" :options="wizardGroupOptions" :placeholder="$t('accounts.groupsPh')" />
        </div>
        <t-button theme="primary" block :loading="savingConfig" @click="finishWizard">{{ $t('accounts.finish') }}</t-button>
      </t-space>
    </t-dialog>

    <!-- 编辑账号：改名 / 绑分组 / 绑代理 / 同步模型 -->
    <t-dialog v-model:visible="editVisible" :header="$t('accounts.editTitle')" :confirm-btn="{ loading: editSaving }" width="640px" @confirm="submitEdit">
      <t-form v-if="editRow" label-width="90px">
        <t-form-item :label="$t('accounts.name')">
          <t-input v-model="editName" :placeholder="$t('accounts.namePh')" clearable />
        </t-form-item>
        <t-form-item v-if="pluginOf(editRow.plugin_id)?.multi_instance" :label="$t('accounts.instance')">
          <t-select v-model="editInstanceId" :options="instanceOptions(editRow.plugin_id)" style="width: 100%" />
        </t-form-item>
        <t-form-item :label="$t('accounts.groupsTitle')">
          <bind-select v-model="editGroups" :options="editGroupOptions" :placeholder="$t('accounts.groupsPh')" />
        </t-form-item>
        <t-form-item :label="$t('accounts.proxyTitle')">
          <bind-select v-model="editProxies" :options="proxyOptions" :placeholder="$t('accounts.proxyPh')" />
        </t-form-item>
        <t-form-item :label="$t('accounts.modelsTitle')">
          <div style="width: 100%">
            <t-link theme="primary" @click="editSyncModels">{{ editSyncing ? $t('accounts.syncing') : $t('accounts.sync') }}</t-link>
            <div v-if="editModels.length" class="model-list" style="margin-top: 8px">
              <t-tag v-for="m in editModels" :key="m.id" closable variant="light-outline" style="margin: 0 6px 6px 0" @close="editModels = editModels.filter((x) => x.id !== m.id)">{{ m.id }}</t-tag>
            </div>
            <span v-else class="hint">{{ $t('accounts.noModels') }}</span>
          </div>
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 在线测试：选端点/模型/问题 → 响应日志 -->
    <t-drawer v-model:visible="testVisible" :header="$t('accounts.testTitle')" size="560px" :footer="false">
      <t-space v-if="testRow" direction="vertical" style="width: 100%" size="large">
        <t-form label-width="80px">
          <t-form-item :label="$t('accounts.testEndpoint')">
            <bind-select v-model="testEndpoint" :multiple="false" :options="endpointOptions" />
          </t-form-item>
          <t-form-item :label="$t('accounts.testModel')">
            <bind-select v-model="testModel" :multiple="false" :options="testModelOptions" :placeholder="$t('accounts.testModelPh')" />
          </t-form-item>
          <t-form-item :label="$t('accounts.testQuestion')">
            <t-input v-model="testQuestion" :placeholder="$t('accounts.testQuestionPh')" />
          </t-form-item>
        </t-form>
        <t-button theme="primary" block :loading="testing" :disabled="!testModel" @click="runTest">{{ $t('accounts.testRun') }}</t-button>
        <div v-if="testText" class="test-answer">{{ testText }}</div>
        <div v-if="testLogs.length" class="test-logs">
          <div v-for="(l, i) in testLogs" :key="i" class="test-log-line">{{ l }}</div>
        </div>
      </t-space>
    </t-drawer>

    <delete-impact-dialog
      v-model:visible="removeVisible"
      :header="$t('common.delete') + ' · ' + (removing?.display_name || `#${removing?.id ?? 0}`)"
      :message="$t('accounts.confirmDelete')"
      :impact-url="`/admin/accounts/${removing?.id ?? 0}/impact`"
      :delete-url="`/admin/accounts/${removing?.id ?? 0}`"
      @deleted="loadAll"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { CheckIcon } from 'tdesign-icons-vue-next'
import { api } from '../api/client'
import BindSelect from '../components/BindSelect.vue'
import DeleteImpactDialog from '../components/DeleteImpactDialog.vue'
import { accountStatusDict, capabilityDict, dict, label, runStatusDict } from '../utils/dict'
import type { Account, AccountDetail, AuthMethod, GroupInfo, InstanceInfo, LoginResp, ModelInfo, NextStep, PluginInfo } from '../api/types'

const { t } = useI18n()
const router = useRouter()

const plugins = ref<PluginInfo[]>([])
const accounts = ref<Account[]>([])
const groups = ref<GroupInfo[]>([])
const instances = ref<InstanceInfo[]>([])
const proxies = ref<{ ID: number; Scheme: string; Host: string; Port: number }[]>([])
const loading = ref(false)

// 编辑弹窗
const editVisible = ref(false)
const editRow = ref<Account | null>(null)
const editName = ref('')
const editInstanceId = ref<number | undefined>(undefined) // t-select 空值用 undefined，避免显示 0
const editGroups = ref<number[]>([])
const editProxies = ref<number[]>([])
const editModels = ref<{ id: string }[]>([])
const editSaving = ref(false)
const editSyncing = ref(false)

// 在线测试抽屉
const testVisible = ref(false)
const testRow = ref<Account | null>(null)
const testEndpoint = ref('chat_completions')
const testModel = ref('')
const testQuestion = ref('')
const testText = ref('')
const testLogs = ref<string[]>([])
const testing = ref(false)

const addVisible = ref(false)
const wizardStep = ref<'select' | 'auth' | 'done'>('select')
const pluginName = ref('')
const methods = ref<AuthMethod[]>([])
const methodId = ref('')
const form = ref<Record<string, string>>({})
const nextStep = ref<NextStep | null>(null)
const stepForm = ref<Record<string, string>>({})
const submitting = ref(false)

// 向导第三步（授权成功后的配置）
const newAccountId = ref(0)
const newAccountName = ref('')
const newAccountGroups = ref<number[]>([])
const savingConfig = ref(false)
const wizardModels = ref<{ id: string }[]>([])
const modelsSyncing = ref(false)
const wizardProfileName = ref('')
const wizardInstanceId = ref<number | undefined>(undefined)
let pollTimer: ReturnType<typeof setTimeout> | null = null

const selectedPluginLabel = computed(() => {
  const p = plugins.value.find((x) => x.name === pluginName.value)
  return p?.label || p?.name || ''
})
const selectedPlugin = computed(() => plugins.value.find((x) => x.name === pluginName.value))
const selectedPluginId = computed(() => selectedPlugin.value?.id ?? 0)
function pluginOf(id: number) {
  return plugins.value.find((x) => x.id === id)
}

// 实例下拉：按插件过滤，展示名称（+ 地址）
function instanceOptions(pluginID: number) {
  return instances.value
    .filter((i) => i.plugin_id === pluginID)
    .map((i) => ({ value: i.id, label: i.base_url ? `${i.name} · ${i.base_url}` : i.name }))
}
function instanceName(id: number): string {
  return instances.value.find((i) => i.id === id)?.name ?? (id ? `#${id}` : '-')
}

// 授权按钮文案：按登录方式形态给出（发送验证码 / 生成授权链接 / 授权）
const submitLabel = computed(() => {
  if (nextStep.value?.wait) return showCallbackInput.value ? t('accounts.submitCallback') : t('accounts.waitingAuth')
  if (nextStep.value) return t('accounts.submitNext')
  const types = currentFields.value.map((f) => f.type)
  if (types.includes('phone')) return t('accounts.sendOtp')
  if (!currentFields.value.length) return t('accounts.genAuthUrl')
  return t('accounts.authorize')
})

const columns = computed(() => [
  { colKey: 'display_name', title: t('accounts.account'), width: 160, ellipsis: true },
  { colKey: 'plugin', title: t('accounts.colPlugin'), width: 110, ellipsis: true, cell: (_h: any, { row }: any) => pluginLabel(row.plugin_id), align: 'center' },
  { colKey: 'instance', title: t('accounts.instance'), width: 132, ellipsis: true, cell: (_h: any, { row }: any) => instanceName(row.instance_id), align: 'center' },
  { colKey: 'group', title: t('accounts.groups'), align: 'center' },
  { colKey: 'credits', title: t('accounts.credits'), width: 120, align: 'center' },
  { colKey: 'status', title: t('accounts.status'), width: 90, align: 'center' },
  { colKey: 'schedule', title: t('accounts.schedule'), width: 110, align: 'center' },
  { colKey: 'last_refresh_at', title: t('accounts.lastRefresh'), width: 120, cell: (_h: any, { row }: any) => row.last_refresh_at ? timeAgo(row.last_refresh_at) : '-', align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 200, align: 'center' },
])

// 相对时间：如 5分钟前 / 1天前 / 3个月前
function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts.replace(' ', 'T')).getTime()
  const min = Math.floor(diff / 60000)
  if (min < 1) return t('common.justNow')
  if (min < 60) return t('common.minutesAgo', { n: min })
  const h = Math.floor(min / 60)
  if (h < 24) return t('common.hoursAgo', { n: h })
  const d = Math.floor(h / 24)
  if (d < 30) return t('common.daysAgo', { n: d })
  const mo = Math.floor(d / 30)
  if (mo < 12) return t('common.monthsAgo', { n: mo })
  return t('common.yearsAgo', { n: Math.floor(mo / 12) })
}

// 调度开关：active ↔ disabled（expired 需重新授权，不可直接开关）
async function toggleSchedule(row: Account) {
  if (row.status === 'active') {
    await api.post(`/admin/accounts/${row.id}/pause`)
    MessagePlugin.success(t('accounts.pausedSchedule'))
  } else {
    try {
      await api.post(`/admin/accounts/${row.id}/resume`)
      MessagePlugin.success(t('accounts.resumedSchedule'))
    } catch (e: any) {
      MessagePlugin.warning(e?.message || String(e))
    }
  }
  await loadAll()
}

// 插件品牌名映射：关联字段统一显示品牌而非 id
function pluginLabel(pluginID: number): string {
  const p = plugins.value.find((x) => x.id === pluginID)
  return p?.label || p?.name || `#${pluginID}`
}

// ---------- 暂停展示 ----------

// 自动暂停（429 限时 / 402 手动）判定：active 但 paused_until 在未来
function pausedInfo(row: Account): string {
  if (row.status !== 'active' || !row.paused_until) return ''
  const until = new Date(row.paused_until).getTime()
  if (!until || until <= Date.now()) return ''
  const untilText = row.paused_until.replace('T', ' ').slice(0, 19)
  return until - Date.now() > 365 * 24 * 3600 * 1000
    ? t('accounts.pausedManual', { reason: row.pause_reason || t('accounts.autoPause') })
    : t('accounts.pausedRateLimited', { until: untilText, reason: row.pause_reason || '' })
}

function pausedLabel(row: Account): string {
  const until = row.paused_until ? new Date(row.paused_until).getTime() : 0
  return until - Date.now() > 365 * 24 * 3600 * 1000 ? t('accounts.pausedManualTag') : t('accounts.pausedRateLimitedTag')
}

// ---------- 账号详情 ----------

const detailVisible = ref(false)
const detail = ref<AccountDetail | null>(null)

const detailRunColumns = computed(() => [
  { colKey: 'run-status', title: t('tasks.colResult'), width: 80 },
  { colKey: 'capability', title: t('tasks.colTask'), width: 110, align: 'center' },
  { colKey: 'summary', title: t('tasks.colSummary'), ellipsis: true, align: 'center' },
  { colKey: 'started_at', title: t('common.colTime'), width: 160, cell: (_h: any, { row }: any) => fmtTime(row.started_at), align: 'center' },
])

// 抽屉标题：账号详情（带名称后缀）
const detailHeader = computed(() =>
  detail.value?.display_name ? `${t('accounts.detailTitle')} · ${detail.value.display_name}` : t('accounts.detailTitle'),
)

// 积分合并一行：
// - 有积分包/免费池的插件（如 lobsterai）：可用 X · N 个积分包 · 免费池 used/limit
// - 其余（如 workbuddy）：剩余 X / 已用 X / 总 X
const creditSummaryLine = computed(() => {
  const c = detail.value?.credits as any
  if (!c) return '-'
  const pkgCount = Array.isArray(c.packages) ? c.packages.length : 0
  if (pkgCount > 0 || c.free_limit !== undefined) {
    const parts: string[] = []
    if (c.remaining !== undefined) parts.push(t('accounts.avail', { v: fmtNum(c.remaining) }))
    if (pkgCount > 0) parts.push(t('accounts.packagesN', { n: pkgCount }))
    if (c.free_limit !== undefined) {
      parts.push(t('accounts.freePool', { used: fmtNum(c.free_used), limit: fmtNum(c.free_limit) }))
    }
    return parts.join(' · ') || '-'
  }
  const legacy = [
    t('accounts.legacyRemaining', { v: fmtNum(c.remaining) }),
    t('accounts.legacyUsed', { v: fmtNum(c.used) }),
    t('accounts.legacyTotal', { v: fmtNum(c.total) }),
  ].filter((p) => !p.endsWith(' -'))
  return legacy.join(' / ') || '-'
})

// ---------- 动态渲染块（插件声明的 ProfileSection） ----------

interface ProfileSection {
  id: string
  title: Record<string, string>
  entries?: { label: Record<string, string>; value: string; kind?: string }[]
  columns?: { key: string; title: Record<string, string>; kind?: string }[]
  items?: Record<string, string>[]
}

// profile 快照里的动态块（核心随 profile_json 持久化，插件声明 → 主框架渲染）
const sections = computed<ProfileSection[]>(() => {
  const raw = detail.value?.profile?.sections
  return Array.isArray(raw) ? (raw as ProfileSection[]) : []
})

// status 值协议："status:xxx" 前缀 = 徽章；其余为纯文本
function isStatus(v: string): boolean {
  return v.startsWith('status:')
}
function statusValue(v: string): string {
  return v.startsWith('status:') ? v.slice('status:'.length) : v
}

// status → 徽章色（成功类绿 / 警示类橙 / 失效类红 / 其余默认）
function sectionStatusTheme(v: string): string {
  const s = statusValue(v)
  if (/已签到|active|正常|有效/.test(s)) return 'success'
  if (/expiringSoon|临期|待领奖|未签到/.test(s)) return 'warning'
  if (/expired|已过期|已领奖/.test(s)) return s === '已领奖' ? 'success' : 'danger'
  return 'default'
}

// 动态表格列：TDesign 列描述（统一走 section-cell 插槽渲染）
function sectionColumns(cols: { key: string; title: Record<string, string>; kind?: string }[]) {
  return cols.map((c) => ({
    colKey: c.key,
    title: label(c.title, c.key),
    kind: c.kind,
    cell: 'section-cell',
    ellipsis: c.kind !== 'status',
  }))
}

function fmtNum(v: string | undefined | null): string {
  if (v === undefined || v === null || v === '') return '-'
  const n = Number(v)
  return Number.isFinite(n) ? n.toLocaleString('en-US', { maximumFractionDigits: 2 }) : v
}

function fmtTime(t: string | null | undefined): string {
  return t ? t.replace('T', ' ').slice(0, 19) : '-'
}

async function openDetail(id: number) {
  detail.value = await api.get<AccountDetail>(`/admin/accounts/${id}/detail`)
  detailVisible.value = true
}

const currentFields = computed(() => methods.value.find((m) => m.id === methodId.value)?.fields ?? [])
const currentMethod = computed(() => methods.value.find((m) => m.id === methodId.value))
// 分组归属实例：向导第三步只列目标实例的分组
const pluginGroups = computed(() => groups.value.filter((g) => g.plugin === pluginName.value && g.instance_id === wizardInstanceId.value))

// 管理界面是否本机访问（决定 auto_wait 的走向：本机 127.0.0.1 回调可达）
const isLocal = ['localhost', '127.0.0.1', '::1'].includes(location.hostname)

// wait 步骤是否渲染回调粘贴框：按插件声明的 callback 模式，未声明按 next 下发推断
const showCallbackInput = computed(() => {
  switch (currentMethod.value?.callback) {
    case 'auto': return false
    case 'wait': return true
    case 'auto_wait': return !isLocal
    default: return !!nextStep.value?.fields?.length
  }
})

// 是否自动轮询等待插件侧完成（auto_wait 非本机时等用户手动提交）
const autoPolling = computed(() => {
  if (!nextStep.value?.wait) return false
  switch (currentMethod.value?.callback) {
    case 'auto': return true
    case 'wait': return false
    case 'auto_wait': return isLocal
    default: return true
  }
})

async function loadAll() {
  loading.value = true
  try {
    const [p, a, g, px, ins] = await Promise.all([
      api.get<{ plugins: PluginInfo[] }>('/admin/plugins'),
      api.get<{ accounts: Account[] }>('/admin/accounts'),
      api.get<{ groups: GroupInfo[] }>('/admin/groups'),
      api.get<{ proxies: typeof proxies.value }>('/admin/proxies'),
      api.get<{ instances: InstanceInfo[] }>('/admin/instances'),
    ])
    plugins.value = p.plugins ?? []
    accounts.value = a.accounts ?? []
    groups.value = g.groups ?? []
    proxies.value = px.proxies ?? []
    instances.value = ins.instances ?? []
  } finally {
    loading.value = false
  }
}

const proxyOptions = computed(() =>
  proxies.value.map((px) => ({ value: px.ID, label: `${px.Scheme}://${px.Host}:${px.Port}` })),
)
const wizardGroupOptions = computed(() =>
  pluginGroups.value.map((g) => ({ value: g.id, label: `${g.name} (${g.plugin_label || g.plugin})` })),
)
const editGroupOptions = computed(() => {
  const row = editRow.value
  return groups.value
    .filter((g) => g.plugin_id === row?.plugin_id && g.instance_id === editInstanceId.value)
    .map((g) => ({ value: g.id, label: `${g.name} (${g.plugin_label || g.plugin})` }))
})
const endpointOptions = [
  { value: 'chat_completions', label: 'chat/completions' },
  { value: 'messages', label: 'messages' },
  { value: 'responses', label: 'responses' },
]
const testModelOptions = computed(() => editModels.value.map((m) => ({ value: m.id, label: m.id })))

// openEdit 打开编辑弹窗，回填名称/分组/代理/模型
async function openEdit(row: Account) {
  editRow.value = row
  editName.value = row.display_name
  editInstanceId.value = row.instance_id || undefined
  editGroups.value = [...(row.group_ids ?? [])]
  editModels.value = []
  editProxies.value = []
  editVisible.value = true
  const [px, detail] = await Promise.all([
    api.get<{ proxy_ids: number[] }>(`/admin/accounts/${row.id}/proxies`).catch(() => ({ proxy_ids: [] })),
    api.get<AccountDetail>(`/admin/accounts/${row.id}/detail`).catch(() => null),
  ])
  editProxies.value = px.proxy_ids ?? []
  editModels.value = (detail?.models ?? []).map((m) => ({ id: m.id }))
}

// editSyncModels 拉上游模型目录（?refresh=1 落库）
async function editSyncModels() {
  if (!editRow.value || editSyncing.value) return
  editSyncing.value = true
  try {
    const resp = await api.get<{ models: ModelInfo[] | null }>(`/admin/accounts/${editRow.value.id}/models?refresh=1`)
    editModels.value = (resp.models ?? []).map((m) => ({ id: m.id }))
  } catch (e: any) {
    MessagePlugin.warning(t('accounts.syncFailed', { msg: e.message }))
  } finally {
    editSyncing.value = false
  }
}

// submitEdit 保存名称/分组/代理/模型（模型以用户勾选为准）
async function submitEdit() {
  if (!editRow.value) return
  editSaving.value = true
  try {
    const id = editRow.value.id
    await api.put(`/admin/accounts/${id}`, { display_name: editName.value, group_ids: editGroups.value, instance_id: editInstanceId.value ?? 0 })
    await api.put(`/admin/accounts/${id}/proxies`, { proxy_ids: editProxies.value })
    await api.put(`/admin/accounts/${id}/models`, { models: editModels.value })
    MessagePlugin.success(t('common.saved'))
    editVisible.value = false
    await loadAll()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    editSaving.value = false
  }
}

// openTest 打开在线测试抽屉，模型候选取账号已存模型
async function openTest(row: Account) {
  testRow.value = row
  testEndpoint.value = 'chat_completions'
  testQuestion.value = ''
  testText.value = ''
  testLogs.value = []
  testModel.value = ''
  testVisible.value = true
  const detail = await api.get<AccountDetail>(`/admin/accounts/${row.id}/detail`).catch(() => null)
  editModels.value = (detail?.models ?? []).map((m) => ({ id: m.id }))
  if (editModels.value.length) testModel.value = editModels.value[0].id
}

// runTest 直调插件 Chat（绕路由/key），输出响应与日志
async function runTest() {
  if (!testRow.value || !testModel.value) return
  testing.value = true
  testText.value = ''
  testLogs.value = []
  try {
    const resp = await api.post<{ text: string; logs: string[] }>(`/admin/accounts/${testRow.value.id}/test`, {
      endpoint: testEndpoint.value, model: testModel.value, question: testQuestion.value,
    })
    testText.value = resp.text ?? ''
    testLogs.value = resp.logs ?? []
  } catch (e: any) {
    testLogs.value = ['✗ ' + (e.message || 'error')]
  } finally {
    testing.value = false
  }
}

function openAdd() {
  addVisible.value = true
  wizardStep.value = 'select'
  nextStep.value = null
  form.value = {}
  stepForm.value = {}
  wizardModels.value = []
  stopPolling()
}

// 第一步点选客户端 → 进入授权（按插件拉实例列表，保证默认实例存在并预选第一个）
async function choosePlugin(p: PluginInfo) {
  pluginName.value = p.name
  wizardStep.value = 'auth'
  loadMethods(p.name)
  const resp = await api.get<{ instances: InstanceInfo[] }>(`/admin/instances?plugin_id=${p.id}`).catch(() => ({ instances: [] }))
  const list = resp.instances ?? []
  instances.value = [...instances.value.filter((i) => i.plugin_id !== p.id), ...list]
  wizardInstanceId.value = list[0]?.id
}

async function loadMethods(name: string) {
  const resp = await api.get<{ auth_methods: AuthMethod[] }>(`/admin/plugins/${name}/auth-methods`)
  methods.value = resp.auth_methods ?? []
  methodId.value = methods.value[0]?.id ?? ''
  nextStep.value = null
  stepForm.value = {}
  stopPolling()
}

watch(methodId, () => {
  form.value = {}
  nextStep.value = null
  stepForm.value = {}
  stopPolling()
})

function onDrop(e: DragEvent, field: string) {
  const file = e.dataTransfer?.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    form.value[field] = String(reader.result ?? '')
    MessagePlugin.success(t('accounts.loadedN', { name: file.name }))
  }
  reader.readAsText(file)
}

function copyText(text: string) {
  navigator.clipboard.writeText(text)
  MessagePlugin.success(t('common.copied'))
}

function openURL(url: string) {
  window.open(url, '_blank')
}

async function submit() {
  if (!pluginName.value || !methodId.value) return
  submitting.value = true
  stopPolling() // 手动提交优先于轮询，避免并发打插件
  try {
    const payload = nextStep.value
      ? { plugin: pluginName.value, method_id: methodId.value, form: stepForm.value, state: nextStep.value.state ?? '', instance_id: wizardInstanceId.value ?? 0 }
      : { plugin: pluginName.value, method_id: methodId.value, form: form.value, state: '', instance_id: wizardInstanceId.value ?? 0 }
    const resp = await api.post<LoginResp>('/admin/accounts/login', payload)
    if (resp.done) {
      enterDoneStep(resp.account_id ?? 0)
    } else {
      nextStep.value = resp.next ?? null
      stepForm.value = {}
      // 浏览器授权：自动打开页面；能自动回调才轮询，否则等用户粘贴提交
      const step = nextStep.value
      if (step?.wait) {
        if (step.url) openURL(step.url)
        if (autoPolling.value) startPolling()
      }
    }
  } catch (e: any) {
    MessagePlugin.error(String(e.message))
    stopPolling()
  } finally {
    submitting.value = false
  }
}

// enterDoneStep 授权成功：进入向导第三步（名称 / 模型列表 / 分组）
function enterDoneStep(accountID: number) {
  stopPolling()
  newAccountId.value = accountID
  newAccountName.value = ''
  newAccountGroups.value = []
  wizardModels.value = []
  wizardProfileName.value = ''
  wizardStep.value = 'done'
  // 默认展示名称（详情接口取 profile）
  api.get<AccountDetail>(`/admin/accounts/${accountID}/detail`).then((d) => {
    wizardProfileName.value = d.display_name || (d.profile?.displayName ?? '')
  }).catch(() => {})
  syncModels()
}

// syncModels 同步客户端模型目录（账号凭据）
async function syncModels() {
  if (!newAccountId.value || modelsSyncing.value) return
  modelsSyncing.value = true
  try {
    const resp = await api.get<{ models: { id: string }[] | null }>(`/admin/accounts/${newAccountId.value}/models?refresh=1`)
    wizardModels.value = resp.models ?? []
  } catch (e: any) {
    MessagePlugin.warning(t('accounts.syncFailed', { msg: e.message }))
  } finally {
    modelsSyncing.value = false
  }
}

// finishWizard 完成向导：保存名称与分组
async function finishWizard() {
  if (newAccountId.value) {
    savingConfig.value = true
    try {
      const body: Record<string, unknown> = {}
      if (newAccountName.value) body.display_name = newAccountName.value
      body.group_ids = newAccountGroups.value
      await api.put(`/admin/accounts/${newAccountId.value}`, body)
    } finally {
      savingConfig.value = false
    }
  }
  addVisible.value = false
  MessagePlugin.success(t('accounts.added'))
  await loadAll()
}

// auto 回调：2 秒一次无表单提交，插件侧等浏览器跳回调
function startPolling() {
  stopPolling()
  pollTimer = setTimeout(async () => {
    if (!autoPolling.value) return
    try {
      const resp = await api.post<LoginResp>('/admin/accounts/login', {
        plugin: pluginName.value, method_id: methodId.value,
        form: {}, state: nextStep.value?.state ?? '', instance_id: wizardInstanceId.value ?? 0,
      })
      if (resp.done) {
        enterDoneStep(resp.account_id ?? 0)
      } else if (resp.next?.wait && autoPolling.value) {
        nextStep.value = resp.next
        startPolling()
      }
    } catch {
      // 网络抖动继续等
      startPolling()
    }
  }, 2000)
}

function stopPolling() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

// 行内分组：账号所属实例的分组
function groupsOf(row: Account): GroupInfo[] {
  return groups.value.filter((g) => g.plugin_id === row.plugin_id && g.instance_id === row.instance_id)
}

function groupName(id: number): string {
  return groups.value.find((g) => g.id === id)?.name ?? `#${id}`
}

// 单个分组增减（tag 关闭 / 弹层勾选）
async function toggleGroup(row: Account, groupID: number, add: boolean) {
  const cur = row.group_ids ?? []
  const next = add ? [...cur, groupID] : cur.filter((id) => id !== groupID)
  row.group_ids = next // 乐观更新
  try {
    await api.put(`/admin/accounts/${row.id}`, { group_ids: next })
  } catch (e: any) {
    row.group_ids = cur
    MessagePlugin.error(e.message)
  }
}

async function refresh(id: number) {
  try {
    await api.post(`/admin/accounts/${id}/refresh`)
    MessagePlugin.success(t('common.refreshed'))
    await loadAll()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  }
}

const removeVisible = ref(false)
const removing = ref<Account | null>(null)

function askRemove(row: Account) {
  removing.value = row
  removeVisible.value = true
}

onBeforeUnmount(stopPolling)
onMounted(loadAll)
</script>

<style scoped>
/* 账号名称：点击开详情，移入高亮 */
.acct-name {
  cursor: pointer;
  transition: color 0.15s ease;
}
.acct-name:hover {
  color: var(--td-brand-color);
  text-decoration: underline;
}
.tab-body {
  padding: 12px 4px;
}
.login-url {
  margin-top: 6px;
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.login-url-text {
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
  flex: 1;
}
.mode-hint {
  margin-top: 4px;
  font-size: 12px;
  opacity: 0.85;
}
.section-title {
  font-weight: 600;
  margin-bottom: 8px;
}
.credit-summary {
  display: flex;
  gap: 16px;
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
.credit-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.model-list {
  max-height: 160px;
  overflow-y: auto;
}
.hint {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}
.test-answer {
  padding: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
}
.test-logs {
  padding: 8px 12px;
  font-family: monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
}
.test-log-line {
  word-break: break-all;
  line-height: 1.7;
}
.wizard-back {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.wizard-client {
  font-weight: 600;
}
.client-card {
  border: 1px solid var(--td-component-border);
  border-radius: var(--td-radius-medium);
  padding: 16px 12px;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s;
}
.client-card:hover {
  border-color: var(--td-brand-color);
  box-shadow: var(--td-shadow-1);
}
.client-icon {
  width: 44px;
  height: 44px;
  margin: 0 auto 8px;
  border-radius: 50%;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 700;
  overflow: hidden;
}
.client-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.client-name {
  font-weight: 600;
  margin-bottom: 6px;
}
.client-caps {
  min-height: 22px;
}
.drop-zone {
  position: relative;
  width: 100%;
}
.drop-hint {
  position: absolute;
  right: 8px;
  bottom: 6px;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  pointer-events: none;
}
.scroll-textarea :deep(.t-textarea__inner) {
  max-height: 320px;
  overflow-y: auto;
}

/* 行内分组 tag：动态增减 */
.group-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}
.group-add {
  cursor: pointer;
  min-width: 22px;
  text-align: center;
}
.group-picker {
  min-width: 160px;
  max-height: 240px;
  overflow-y: auto;
}
.group-picker-empty {
  padding: 8px 12px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.group-picker-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 12px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  white-space: nowrap;
  transition: background-color 0.15s ease;
}
.group-picker-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.group-picker-item.active {
  color: var(--td-brand-color);
}
</style>
