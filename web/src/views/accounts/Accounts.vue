<template>
  <div class="page">
    <page-header>
      
      <t-button theme="primary" :disabled="!plugins.length" @click="openAdd">{{ $t('accounts.add') }}</t-button>
    </page-header>

    <c-table row-key="id" :data="accounts" :columns="columns" :loading="loading">
      <template #display_name="{ row }">
        <span class="acct-name" @click="openDetail(row.id)">{{ row.display_name || `#${row.id}` }}</span>
      </template>
      <template #group="{ row }">
        <group-picker
          :model-value="row.group_ids"
          :groups="groupsOf(row)"
          :name-of="groupName"
          @update:model-value="(v: number[]) => toggleGroup(row, v)"
        />
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
    </c-table>

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
          <c-table
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
          </c-table>
        </div>

        <div>
          <div class="section-title">{{ $t('accounts.runsTitle') }}</div>
          <c-table
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
          </c-table>
          <t-empty v-else :description="$t('accounts.noRuns')" />
        </div>
      </t-space>
    </t-drawer>

    <!-- 添加账号：向导（选择客户端 → 授权 → 配置） -->
    <c-dialog v-model:visible="addVisible" :header="$t('accounts.add')" :footer="false" width="680px" :close-on-overlay-click="false">

      <!-- 第一步：选择客户端（卡片平铺，每行四个；登录要走插件进程，只列运行中的） -->
      <template v-if="wizardStep === 'select'">
        <t-empty v-if="!runningPlugins.length" :description="$t('accounts.noPlugins')" />
        <div v-else class="client-grid">
          <div v-for="p in runningPlugins" :key="p.id" class="client-card" @click="choosePlugin(p)">
            <div class="client-head">
              <entity-icon :icon="p.icon" :name="p.label || p.name" />
              <div class="client-name">{{ p.label || p.name }}</div>
            </div>
            <div class="client-caps">
              <t-tag v-for="c in (p.capabilities ?? []).slice(0, 3)" :key="c" size="small" variant="light">
                {{ dict(capabilityDict, c) }}
              </t-tag>
            </div>
          </div>
        </div>
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

        <c-tabs v-if="methods.length" v-model="methodId">
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
        </c-tabs>

        <!-- 浏览器授权：链接可复制可打开；二维码 data URL 直接内联渲染；auto 模式自动轮询 -->
        <t-alert v-if="nextStep" :theme="nextStep.action === 'open_url' ? 'warning' : 'info'">
          <template #message>
            <div>{{ label(nextStep.prompt, '') }}</div>
            <div v-if="nextStep.wait" class="mode-hint">
              {{ showCallbackInput ? $t('accounts.callbackManual') : $t('accounts.callbackAuto') }}
            </div>
            <div v-if="isQrDataUrl(nextStep.url)" class="qr-wrap">
              <img :src="nextStep.url" alt="QR" class="qr-img" />
            </div>
            <div v-else-if="nextStep.url" class="login-url">
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
    </c-dialog>

    <!-- 编辑账号：改名 / 绑分组 / 绑代理 / 同步模型 -->
    <c-dialog v-model:visible="editVisible" :header="$t('accounts.editTitle')" :confirm-btn="{ loading: editSaving }" width="640px" @confirm="submitEdit">
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
    </c-dialog>

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
        <template v-if="testRequest || testEvents.length">
          <t-collapse>
            <t-collapse-panel v-if="testRequest" :header="$t('accounts.testRequest')">
              <pre class="test-raw">{{ testRequest }}</pre>
            </t-collapse-panel>
            <t-collapse-panel v-if="testEvents.length" :header="$t('accounts.testEvents')">
              <pre class="test-raw">{{ testEvents.join('\n') }}</pre>
            </t-collapse-panel>
          </t-collapse>
          <t-button variant="outline" block @click="exportTest">{{ $t('accounts.testExport') }}</t-button>
        </template>
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
import { CCard, CDialog, CTable, CTabs } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import EntityIcon from '../../components/EntityIcon.vue'
import GroupPicker from './GroupPicker.vue'
import { pluginLabelOf, instanceNameOf } from '../../utils/lookup'
import { timeAgo, fmtNum, fmtTime } from '../../utils/format'
import { copyText } from '../../utils/common'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { accountApi, groupApi, instanceApi, pluginApi, proxyApi } from '../../api/entities'
import BindSelect from '../../components/BindSelect.vue'
import DeleteImpactDialog from '../../components/DeleteImpactDialog.vue'
import { accountStatusDict, capabilityDict, dict, label, runStatusDict } from '../../utils/dict'
import type { Account, AccountDetail, AuthMethod, GroupInfo, InstanceInfo, LoginResp, ModelInfo, NextStep, PluginInfo } from '../../api/types'
import { isQrDataUrl } from '../../api/types'

const { t } = useI18n()
const router = useRouter()

const plugins = ref<PluginInfo[]>([])
// 已停止的插件仍在列表里（账号列品牌名要查得到），但新建账号只能选运行中的
const runningPlugins = computed(() => plugins.value.filter((p) => p.running))
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
const testRequest = ref('')
const testEvents = ref<string[]>([])
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
const instanceName = (id: number) => instanceNameOf(instances.value, id)

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

// 调度开关：active ↔ disabled（expired 需重新授权，不可直接开关）
async function toggleSchedule(row: Account) {
  if (row.status === 'active') {
    await accountApi.pause(row.id)
    MessagePlugin.success(t('accounts.pausedSchedule'))
  } else {
    try {
      await accountApi.resume(row.id)
      MessagePlugin.success(t('accounts.resumedSchedule'))
    } catch (e: any) {
      MessagePlugin.warning(e?.message || String(e))
    }
  }
  await loadAll()
}

const pluginLabel = (pluginID: number) => pluginLabelOf(plugins.value, pluginID)

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

async function openDetail(id: number) {
  detail.value = await accountApi.detail(id)
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
      pluginApi.list(), accountApi.list(), groupApi.list(), proxyApi.list(), instanceApi.list(),
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
    accountApi.proxies(row.id).catch(() => ({ proxy_ids: [] })),
    accountApi.detail(row.id).catch(() => null),
  ])
  editProxies.value = px.proxy_ids ?? []
  editModels.value = (detail?.models ?? []).map((m) => ({ id: m.id }))
}

// editSyncModels 拉上游模型目录（?refresh=1 落库）
async function editSyncModels() {
  if (!editRow.value || editSyncing.value) return
  editSyncing.value = true
  try {
    const resp = await accountApi.models(editRow.value.id, true)
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
    await accountApi.update(id, { display_name: editName.value, group_ids: editGroups.value, instance_id: editInstanceId.value ?? 0 })
    await accountApi.saveProxies(id, editProxies.value)
    await accountApi.saveModels(id, editModels.value)
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
  testRequest.value = ''
  testEvents.value = []
  testModel.value = ''
  testVisible.value = true
  const detail = await accountApi.detail(row.id).catch(() => null)
  editModels.value = (detail?.models ?? []).map((m) => ({ id: m.id }))
  if (editModels.value.length) testModel.value = editModels.value[0].id
}

// runTest 直调插件 Chat（绕路由/key），输出响应与日志
async function runTest() {
  if (!testRow.value || !testModel.value) return
  testing.value = true
  testText.value = ''
  testLogs.value = []
  testRequest.value = ''
  testEvents.value = []
  try {
    const resp = await accountApi.test(testRow.value.id, {
      endpoint: testEndpoint.value, model: testModel.value, question: testQuestion.value,
    })
    testText.value = resp.text ?? ''
    testLogs.value = resp.logs ?? []
    testRequest.value = resp.request ?? ''
    testEvents.value = resp.events ?? []
  } catch (e: any) {
    testLogs.value = ['✗ ' + (e.message || 'error')]
  } finally {
    testing.value = false
  }
}

// exportTest 导出本次测试的 请求/事件/回答 为 JSON blob 下载
function exportTest() {
  const data = JSON.stringify(
    { request: JSON.parse(testRequest.value || 'null'), events: testEvents.value, text: testText.value, logs: testLogs.value },
    null, 2,
  )
  const url = URL.createObjectURL(new Blob([data], { type: 'application/json' }))
  const a = document.createElement('a')
  a.href = url
  a.download = `test-${testRow.value?.id ?? 0}-${Date.now()}.json`
  a.click()
  URL.revokeObjectURL(url)
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
  const resp = await instanceApi.list(p.id).catch(() => ({ instances: [] }))
  const list = resp.instances ?? []
  instances.value = [...instances.value.filter((i) => i.plugin_id !== p.id), ...list]
  wizardInstanceId.value = list[0]?.id
}

async function loadMethods(name: string) {
  const resp = await pluginApi.authMethods(name)
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
    const resp = await accountApi.login(payload)
    if (resp.done) {
      enterDoneStep(resp.account_id ?? 0)
    } else {
      nextStep.value = resp.next ?? null
      stepForm.value = {}
      // 浏览器授权：自动打开页面（二维码 data URL 只渲染不打开）；能自动回调才轮询
      const step = nextStep.value
      if (step?.wait) {
        if (step.url && !isQrDataUrl(step.url)) openURL(step.url)
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
  accountApi.detail(accountID).then((d) => {
    wizardProfileName.value = d.display_name || (d.profile?.displayName ?? '')
  }).catch(() => {})
  syncModels()
}

// syncModels 同步客户端模型目录（账号凭据）
async function syncModels() {
  if (!newAccountId.value || modelsSyncing.value) return
  modelsSyncing.value = true
  try {
    const resp = await accountApi.models(newAccountId.value, true)
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
      await accountApi.update(newAccountId.value, body)
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
      const resp = await accountApi.login({
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

// 分组增减（GroupPicker 回调，乐观更新）
async function toggleGroup(row: Account, next: number[]) {
  const cur = row.group_ids ?? []
  row.group_ids = next
  try {
    await accountApi.update(row.id, { group_ids: next })
  } catch (e: any) {
    row.group_ids = cur
    MessagePlugin.error(e.message)
  }
}

async function refresh(id: number) {
  try {
    await accountApi.refresh(id)
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
.qr-wrap {
  margin-top: 8px;
  text-align: center;
}
.qr-img {
  width: 200px;
  height: 200px;
  border-radius: 8px;
  background: #fff;
  padding: 8px;
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
.test-raw {
  margin: 0;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
  max-height: 280px;
  overflow: auto;
}
.wizard-back {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.wizard-client {
  font-weight: 600;
}
.client-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  max-height: 420px;
  overflow-y: auto;
  padding-right: 4px;
}
.client-card {
  border: 1px solid var(--td-component-border);
  border-radius: var(--td-radius-medium);
  padding: 14px 16px;
  cursor: pointer;
  transition: all 0.2s;
}
.client-card:hover {
  border-color: var(--td-brand-color);
  box-shadow: var(--td-shadow-1);
}
.client-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.client-name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.client-caps {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
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

</style>
