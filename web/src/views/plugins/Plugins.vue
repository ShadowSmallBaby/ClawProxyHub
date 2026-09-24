<template>
  <div class="page">
    <page-header>

      <t-space>
        <t-button variant="outline" @click="openSources">{{ $t('plugins.sources') }}</t-button>
        <t-button variant="outline" :loading="marketLoading" @click="openMarket">{{ $t('plugins.market') }}</t-button>
        <t-upload
          :auto-upload="false"
          :show-upload-progress="false"
          accept=".cphplugin,.zip"
          :request-method="uploadInstall"
          @fail="onUploadFail"
        >
          <t-button theme="primary">{{ $t('plugins.upload') }}</t-button>
        </t-upload>
      </t-space>
    </page-header>

    <!-- 已安装（磁盘为准，含已停止的：内容不变，只多一个状态标签） -->
    <t-empty v-if="!plugins.length" :description="$t('plugins.emptyInstalled')" />
    <div class="plugin-grid">
      <c-card v-for="p in plugins" :key="p.name">
        <template #header>
          <div class="plugin-head">
            <entity-icon :icon="p.icon" :name="p.label || p.name" />
            <div class="plugin-head-meta">
              <div class="plugin-name">{{ p.label || p.name }}</div>
              <div class="plugin-sub">
                v{{ p.version }} · {{ p.author }}
                <t-tag size="small" variant="light" :theme="p.runtime === 'lua' ? 'primary' : 'default'" :title="$t('plugins.runtime')">{{ p.runtime === 'lua' ? $t('plugins.runtimeLua') : $t('plugins.runtimeGo') }}</t-tag>
                <t-tag size="small" variant="outline" class="proto-tag" :title="$t('plugins.protocol')">P{{ p.protocol_version ?? 1 }}</t-tag>
                <t-tag v-if="!p.running" size="small" theme="warning" variant="light">{{ $t('plugins.stoppedTag') }}</t-tag>
              </div>
            </div>
          </div>
        </template>
        <t-space direction="vertical" style="width: 100%">
          <t-space v-if="p.capabilities?.length" size="small">
            <t-tag v-for="c in p.capabilities" :key="c" size="small" variant="light">{{ dict(capabilityDict, c) }}</t-tag>
          </t-space>
          <div v-if="p.auth_methods?.length" class="methods">
            <div class="methods-title">{{ $t('plugins.authMethods') }}</div>
            <t-space size="small">
              <t-tag v-for="m in p.auth_methods" :key="m.id" theme="primary" variant="light-outline">
                {{ label(m.label, m.id) }}
              </t-tag>
            </t-space>
          </div>
          <t-space size="small" style="margin-top: 4px">
            <t-link theme="primary" @click="openSettings(p)">{{ $t('plugins.settings') }}</t-link>
            <t-link v-if="p.multi_instance" theme="primary" @click="openInstances(p)">{{ $t('menu.instances') }}</t-link>
            <t-link theme="primary" @click="restart(p)">{{ $t('plugins.restart') }}</t-link>
            <t-link theme="warning" :disabled="!p.running" @click="stop(p.name)">{{ $t('plugins.stop') }}</t-link>
            <t-link theme="danger" @click="askUninstall(p)">{{ $t('plugins.uninstall') }}</t-link>
          </t-space>
        </t-space>
      </c-card>
    </div>

    <!-- 市场：多源时按源下拉懒加载，默认 official -->
    <t-drawer v-model:visible="marketVisible" :header="$t('plugins.market')" size="420px">
      <t-select v-if="sources.length > 1" v-model="marketSourceName" style="width: 100%; margin-bottom: 12px" @change="loadMarket">
        <t-option v-for="s in sources.filter((x) => x.enabled)" :key="s.name" :value="s.name" :label="s.name" />
      </t-select>
      <t-alert v-if="marketOnline === 'offline'" theme="warning" :message="$t('plugins.offlineHint')" style="margin-bottom: 12px" />
      <t-loading :loading="marketLoading" size="small" style="width: 100%">
        <t-empty v-if="!marketEntries.length" :description="$t('plugins.marketEmpty')" />
        <div class="market-card" v-for="e in marketEntries" :key="(e.source ?? '') + '/' + e.author + '/' + e.name">
          <t-tag size="small" variant="light" class="market-version">v{{ e.version }}</t-tag>
          <div class="market-head">
            <entity-icon :icon="e.icon" :name="e.label?.zh ?? e.name" size="40px" />
            <div class="market-meta">
              <div class="market-name">{{ label(e.label, e.name) }}</div>
              <div class="market-sub">
                {{ e.author || $t('plugins.unknownAuthor') }}
                <t-tag size="small" variant="light" :theme="e.runtime === 'lua' ? 'primary' : 'default'">{{ e.runtime === 'lua' ? $t('plugins.runtimeLua') : $t('plugins.runtimeGo') }}</t-tag>
              </div>
            </div>
          </div>
          <div class="market-foot">
            <span class="market-date">{{ e.published_at || '' }}</span>
            <t-button v-if="!e.installed" size="small" theme="primary" :loading="installing === marketKey(e)" @click="installFromMarket(e)">
              {{ $t('plugins.install') }}
            </t-button>
            <t-button v-else-if="e.updatable" size="small" theme="warning" variant="outline" :loading="installing === marketKey(e)" @click="installFromMarket(e)">
              {{ $t('plugins.upgrade') }}
            </t-button>
            <t-tag v-else size="small" theme="success" variant="light">{{ $t('plugins.installed') }}</t-tag>
          </div>
        </div>
      </t-loading>
    </t-drawer>

    <!-- 操作进度：安装 / 升级 / 重启 / 卸载；含下载的操作可中途取消 -->
    <op-progress-dialog
      v-model:visible="op.visible"
      :header="op.header"
      :steps="op.steps"
      :logs="op.logs"
      :running="op.running"
      :cancelable="!!op.cancel"
      @cancel="op.cancel?.()"
    />

    <!-- 插件设置：schema 动态渲染 -->
    <c-dialog
      v-model:visible="settingsVisible"
      :header="$t('plugins.settingsHeader', { name: settingsPlugin?.label || (settingsPlugin?.name ?? '') })"
      :confirm-btn="{ loading: savingSettings }"
      :cancel-btn="{ content: $t('plugins.resetBtn'), loading: savingSettings }"
      @confirm="saveSettings"
      @cancel="resetSettings"
    >
      <t-alert v-if="!settingFields.length" theme="info" :message="$t('plugins.noSettings')" />
      <t-form v-else label-width="140px">
        <t-form-item v-for="f in settingFields" :key="f.key" :label="f.title" :description="f.description">
          <t-switch v-if="f.type === 'boolean'" v-model="settingsValues[f.key]" />
          <t-select v-else-if="f.options?.length" v-model="settingsValues[f.key]" clearable style="width: 100%">
            <t-option v-for="o in f.options" :key="String(o)" :value="o" :label="String(o)" />
          </t-select>
          <t-input-number v-else-if="f.type === 'number'" v-model="settingsValues[f.key]" theme="column" style="width: 160px" />
          <t-input v-else v-model="settingsValues[f.key]" :placeholder="f.default ? $t('plugins.phDefault', { d: f.default }) : $t('plugins.phDefaultNone')" />
        </t-form-item>
      </t-form>
      <t-alert v-if="settingFields.length" theme="info" :message="$t('plugins.settingsHint')" style="margin-top: 12px" />
    </c-dialog>

    <!-- 插件实例：多实例插件的实例列表，统一在此增改删 -->
    <t-drawer v-model:visible="instancesVisible" :header="$t('plugins.instancesHeader', { name: instancesPlugin?.label || (instancesPlugin?.name ?? '') })" size="640px" :footer="false">
      <div style="margin-bottom: 12px">
        <t-button theme="primary" size="small" @click="openInstanceForm(null)">{{ $t('instances.add') }}</t-button>
      </div>
      <c-table row-key="id" :data="pluginInstances" :columns="instanceColumns" size="small">
        <template #base_url="{ row }"><span class="mono">{{ row.base_url || '-' }}</span></template>
        <template #op="{ row }">
          <t-space size="small">
            <t-link theme="primary" @click="openInstanceForm(row)">{{ $t('common.edit') }}</t-link>
            <t-link theme="danger" @click="askRemoveInstance(row)">{{ $t('common.delete') }}</t-link>
          </t-space>
        </template>
      </c-table>
    </t-drawer>
    <instance-form-dialog v-model:visible="instanceFormVisible" :plugin="instancesPlugin" :instance="instanceEditing" @saved="loadPluginInstances" />
    <delete-impact-dialog
      v-model:visible="removeVisible"
      :header="removeTarget?.header ?? ''"
      :message="removeTarget?.message ?? ''"
      :impact-url="removeTarget?.impactUrl ?? ''"
      :delete-url="removeTarget?.deleteUrl"
      @deleted="removeTarget?.after()"
      @confirm="removeTarget?.after()"
    />

    <!-- 插件源：卡片式（首卡 = 添加） -->
    <t-drawer v-model:visible="sourcesVisible" :header="$t('plugins.sourcesTitle')" size="760px" :footer="false">
      <div class="source-grid">
        <div class="source-card source-add" @click="openSourceForm(null)">
          <div class="source-add-plus">＋</div>
          <div>{{ $t('plugins.sourceAdd') }}</div>
        </div>
        <div v-for="s in sources" :key="s.name" class="source-card" :class="{ disabled: !s.enabled }">
          <div class="source-name">
            <a :href="s.url" target="_blank" rel="noopener" class="source-link">{{ s.name }}</a>
            <t-tag size="small" :theme="s.name === 'official' ? 'primary' : 'default'" variant="light">
              {{ s.name === 'official' ? $t('plugins.sourceOfficial') : $t('plugins.sourceThirdParty') }}
            </t-tag>
          </div>
          <div class="source-count">
            <template v-if="s.reachable">{{ $t('plugins.sourceStats', { n: s.plugin_count ?? 0, m: s.installed_count ?? 0 }) }}</template>
            <template v-else>{{ $t('plugins.sourceUnreachable') }}</template>
          </div>
          <div class="source-ops">
            <template v-if="s.name !== 'official'">
              <t-switch size="small" :value="s.enabled" @change="(v: boolean) => toggleSource(s, v)" />
              <t-link theme="danger" size="small" @click="removeSource(s)">{{ $t('common.delete') }}</t-link>
            </template>
            <t-link theme="primary" size="small" class="source-edit" @click="openSourceForm(s)">{{ $t('common.edit') }}</t-link>
          </div>
        </div>
      </div>
    </t-drawer>

    <!-- 源新建/编辑：英文名全局唯一；保存前探测索引可达并记录条目数 -->
    <c-dialog v-model:visible="sourceFormVisible" :header="sourceEditing ? $t('plugins.sourceEdit') : $t('plugins.sourceAdd')" :confirm-btn="{ loading: savingSources }" @confirm="saveSourceForm">
      <t-form label-width="90px">
        <t-form-item :label="$t('plugins.sourceName')" required-mark>
          <t-input v-model="sourceForm.name" :disabled="sourceEditing?.name === 'official'" placeholder="my-source" />
        </t-form-item>
        <t-form-item :label="$t('plugins.sourceUrl')" required-mark>
          <t-input v-model="sourceForm.url" placeholder="https://.../index.json" />
        </t-form-item>
      </t-form>
    </c-dialog>
  </div>
</template>

<script setup lang="ts">
import { CCard, CDialog, CTable } from '../../components/base'
import EntityIcon from '../../components/EntityIcon.vue'
import PageHeader from '../../components/PageHeader.vue'
import { useAsync } from '../../composables'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import type { ResponseType } from 'tdesign-vue-next'
import { pluginApi, pluginSourceApi, instanceApi, type MarketEntry } from '../../api/entities'
import InstanceFormDialog from '../../components/InstanceFormDialog.vue'
import DeleteImpactDialog from '../../components/DeleteImpactDialog.vue'
import OpProgressDialog, { type OpLog, type OpStep } from '../../components/OpProgressDialog.vue'
import { capabilityDict, dict, label } from '../../utils/dict'
import { notifyDeleteImpact } from '../../utils/impact'
import type { InstanceInfo, PluginInfo, PluginSource } from '../../api/types'

const { t } = useI18n()

const plugins = ref<PluginInfo[]>([])

// ---------- 市场 ----------

const marketVisible = ref(false)
const marketLoading = ref(false)
const marketEntries = ref<MarketEntry[]>([])
const marketOnline = ref('')
const marketSourceName = ref('official')
const installing = ref('') // 正在安装的条目键（同一时间只装一个）

// 同插件判定键 author/name（与后端 pluginKey 一致）
const marketKey = (e: MarketEntry) => (e.author ?? '') + '/' + e.name

// ---------- 操作进度弹窗（安装 / 升级 / 重启 / 卸载共用） ----------

// cancel 非空 = 可取消（含下载的安装/升级）；置空 = 不可取消（重启/卸载）
const op = reactive({ visible: false, header: '', steps: [] as OpStep[], logs: [] as OpLog[], running: false, cancel: null as null | (() => void) })

// 进入某步：前面的全部完成，本步进行中
function opEnter(key: string) {
  let reached = false
  for (const s of op.steps) {
    if (s.key === key) { s.status = 'active'; reached = true }
    else if (!reached) s.status = 'done'
  }
}

// 追加一行带时间戳的日志（HH:MM:SS）
function opLog(text: string, level: 'info' | 'error' = 'info') {
  op.logs.push({ time: new Date().toLocaleTimeString('zh-CN', { hour12: false }), text, level })
}

// 原地更新最后一行日志（下载百分比高频刷新，避免日志刷屏）
function opLogUpdate(text: string) {
  const last = op.logs[op.logs.length - 1]
  if (last) { last.text = text; last.time = new Date().toLocaleTimeString('zh-CN', { hour12: false }) }
}

// 跑一个多步操作：成功全部打勾后自动关闭，失败停在当前步展示错误；结束后刷新列表。
// onCancel 非空则弹窗可取消（点击时调用它中断，如 abort 下载）；取消抛 AbortError 走静默关闭。
async function runOp(header: string, keys: string[], exec: (enter: typeof opEnter, log: typeof opLog, logUpdate: typeof opLogUpdate) => Promise<void>, doneMsg: string, onCancel?: () => void) {
  op.header = header
  op.steps = keys.map((k) => ({ key: k, label: t('plugins.step' + k[0].toUpperCase() + k.slice(1)), status: 'pending' }))
  op.logs = []
  op.visible = true
  op.running = true
  op.cancel = onCancel ?? null
  try {
    await exec(opEnter, opLog, opLogUpdate)
    op.steps.forEach((s) => { s.status = 'done' })
    opLog(t('plugins.opDone'))
    MessagePlugin.success(doneMsg)
    setTimeout(() => { op.visible = false }, 800)
  } catch (e: any) {
    if (e?.name === 'AbortError') {
      opLog(t('common.canceled'))
      op.running = false
      op.cancel = null
      MessagePlugin.info(t('common.canceled'))
      setTimeout(() => { op.visible = false }, 800)
      await load()
      return
    }
    const cur = op.steps.find((s) => s.status === 'active') ?? op.steps[op.steps.length - 1]
    if (cur) cur.status = 'error'
    opLog(t('plugins.opFailed', { msg: e.message }), 'error')
  } finally {
    op.running = false
    op.cancel = null
  }
  await load()
}

const mb = (n: number) => (n / 1048576).toFixed(1)

async function openMarket() {
  await loadSources()
  if (!sources.value.some((s) => s.name === marketSourceName.value && s.enabled)) {
    marketSourceName.value = sources.value.find((s) => s.enabled)?.name ?? 'official'
  }
  marketVisible.value = true
  await loadMarket()
}

// 按当前选中源懒加载
async function loadMarket() {
  marketLoading.value = true
  try {
    const resp = await pluginApi.marketplace(marketSourceName.value)
    marketEntries.value = resp.plugins ?? []
    marketOnline.value = resp.source ?? ''
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    marketLoading.value = false
  }
}

// 安装：下载 → 安装 → 运行；升级多一步「停止」旧进程（阶段由后端进度流给出）。
// 下载可能无整体超时，弹窗提供「取消」→ abort 请求中断下载。
async function installFromMarket(e: MarketEntry) {
  installing.value = marketKey(e)
  const name = label(e.label, e.name)
  const upgrade = !!e.updatable
  const header = `${t(upgrade ? 'plugins.upgrade' : 'plugins.install')} · ${name}`
  const keys = upgrade ? ['downloading', 'stopping', 'installing', 'starting'] : ['downloading', 'installing', 'starting']
  const controller = new AbortController()
  try {
    await runOp(header, keys, async (enter, log, logUpdate) => {
      enter('downloading')
      let dlStarted = false
      await pluginApi.installMarket(e.name, e.author ?? '', e.source ?? '', (p) => {
        if (p.phase === 'downloading') {
          const recv = mb(p.received ?? 0)
          const info = p.total && p.total > 0
            ? `${recv} / ${mb(p.total)} MB (${Math.floor(((p.received ?? 0) / p.total) * 100)}%)`
            : `${recv} MB`
          if (!dlStarted) {
            dlStarted = true
            if (p.total && p.total > 0) log(t('plugins.logGotSize', { size: mb(p.total) }))
            log(t('plugins.logDownloading', { info }))
          } else {
            logUpdate(t('plugins.logDownloading', { info }))
          }
        } else if (p.phase === 'stopping') {
          enter('stopping'); log(t('plugins.logStopping'))
        } else if (p.phase === 'installing') {
          enter('installing'); log(t('plugins.logUnpacking'))
        } else if (p.phase === 'starting') {
          enter('starting'); log(t('plugins.logRunning'))
        }
      }, controller.signal)
    }, t('plugins.installedN', { name: e.name }), () => controller.abort())
    if (marketVisible.value) await loadMarket()
  } finally {
    installing.value = ''
  }
}

// ---------- 插件源 ----------

const sourcesVisible = ref(false)
const sources = ref<PluginSource[]>([])
const savingSources = ref(false)
const sourceFormVisible = ref(false)
const sourceEditing = ref<PluginSource | null>(null)
const sourceForm = reactive({ name: '', url: '' })

async function loadSources() {
  const resp = await pluginSourceApi.list()
  sources.value = resp.sources ?? []
}

async function openSources() {
  await loadSources()
  sourcesVisible.value = true
}

function openSourceForm(s: PluginSource | null) {
  sourceEditing.value = s
  sourceForm.name = s?.name ?? ''
  sourceForm.url = s?.url ?? ''
  sourceFormVisible.value = true
}

// 全量回写源列表（后端做最终校验），成功后重拉（带实时计数）
async function persistSources(list: PluginSource[]) {
  savingSources.value = true
  try {
    await pluginSourceApi.save(list.map((s) => ({ name: s.name, url: s.url, enabled: s.enabled })))
    await loadSources()
    MessagePlugin.success(t('plugins.sourcesSaved'))
    return true
  } catch (e: any) {
    MessagePlugin.error(e.message)
    return false
  } finally {
    savingSources.value = false
  }
}

async function saveSourceForm() {
  const name = sourceForm.name.trim()
  const url = sourceForm.url.trim()
  if (!/^[A-Za-z0-9_-]{1,32}$/.test(name)) {
    MessagePlugin.warning(t('plugins.sourceNameRule'))
    return
  }
  if (sources.value.some((s) => s.name === name && s !== sourceEditing.value)) {
    MessagePlugin.warning(t('plugins.sourceNameDup', { name }))
    return
  }
  if (!/^https?:\/\//.test(url)) {
    MessagePlugin.warning(t('plugins.sourceUrlRule'))
    return
  }
  // 地址变更时探测可达
  if (url !== sourceEditing.value?.url) {
    savingSources.value = true
    try {
      await pluginSourceApi.probe(url)
    } catch (e: any) {
      MessagePlugin.error(e.message)
      savingSources.value = false
      return
    }
  }
  const next = sources.value.map((s) => (s === sourceEditing.value ? { ...s, name, url } : s))
  if (!sourceEditing.value) next.push({ name, url, enabled: true })
  if (await persistSources(next)) sourceFormVisible.value = false
}

function toggleSource(s: PluginSource, enabled: boolean) {
  persistSources(sources.value.map((x) => (x === s ? { ...x, enabled } : x)))
}

function removeSource(s: PluginSource) {
  persistSources(sources.value.filter((x) => x !== s))
}

// ---------- 插件实例（多实例插件） ----------

const instancesVisible = ref(false)
const instancesPlugin = ref<PluginInfo | null>(null)
const pluginInstances = ref<InstanceInfo[]>([])
const instanceFormVisible = ref(false)
const instanceEditing = ref<InstanceInfo | null>(null)

const instanceColumns = computed(() => [
  { colKey: 'name', title: t('instances.colName'), width: 140, ellipsis: true },
  { colKey: 'base_url', title: t('instances.colBaseUrl'), ellipsis: true },
  { colKey: 'account_count', title: t('instances.colAccounts'), width: 80, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 110, align: 'center' },
])

async function openInstances(p: PluginInfo) {
  instancesPlugin.value = p
  instancesVisible.value = true
  await loadPluginInstances()
}

async function loadPluginInstances() {
  if (!instancesPlugin.value) return
  const resp = await instanceApi.list(instancesPlugin.value.id)
  pluginInstances.value = resp.instances ?? []
}

function openInstanceForm(row: InstanceInfo | null) {
  instanceEditing.value = row
  instanceFormVisible.value = true
}

// ---------- 删除确认（插件卸载 / 实例删除共用一个影响面弹窗） ----------

// deleteUrl 缺省 = 弹窗只做确认，确认后由 after 自行执行（插件卸载走进度弹窗）
interface RemoveTarget { header: string; message: string; impactUrl: string; deleteUrl?: string; after: () => void }
const removeVisible = ref(false)
const removeTarget = ref<RemoveTarget | null>(null)

function askRemoveInstance(row: InstanceInfo) {
  removeTarget.value = {
    header: t('common.delete') + ' · ' + row.name,
    message: t('instances.confirmDelete'),
    impactUrl: `/admin/instances/${row.id}/impact`,
    deleteUrl: `/admin/instances/${row.id}`,
    after: loadPluginInstances,
  }
  removeVisible.value = true
}

// 卸载：确认影响面 → 停止 → 删除（文件 + 级联记录）
function askUninstall(p: PluginInfo) {
  const name = p.label || p.name
  removeTarget.value = {
    header: t('plugins.uninstall') + ' · ' + name,
    message: t('plugins.confirmUninstall', { name }),
    impactUrl: `/admin/plugins/${p.name}/impact`,
    after: async () => {
      await runOp(`${t('plugins.uninstall')} · ${name}`, ['stopping', 'removing'], async (enter, log) => {
        enter('stopping'); log(t('plugins.logStopping'))
        await pluginApi.stop(p.name)
        enter('removing'); log(t('plugins.logRemoving'))
        const resp = await pluginApi.uninstall(p.name)
        notifyDeleteImpact(resp.impact, t)
      }, t('plugins.uninstalledN', { name: p.name }))
      if (marketVisible.value) await loadMarket()
    },
  }
  removeVisible.value = true
}

// ---------- 插件设置 ----------

interface SettingField { key: string; title: string; description: string; type: string; default: unknown; options: unknown[] }

const settingsVisible = ref(false)
const settingsPlugin = ref<PluginInfo | null>(null)
const settingsValues = ref<Record<string, any>>({})
const settingFields = ref<SettingField[]>([])
const savingSettings = ref(false)

async function openSettings(p: PluginInfo) {
  settingsPlugin.value = p
  const resp = await pluginApi.settings(p.name)
  const props = resp.schema?.properties ?? {}
  settingFields.value = Object.entries(props).map(([key, def]: [string, any]) => ({
    key, title: def.title ?? key, description: def.description ?? '',
    type: def.type ?? 'string', default: def.default ?? '', options: def.enum ?? [],
  }))
  const values = { ...(resp.values ?? {}) }
  // 未保存的字段用 schema 默认值预填（与实例设置弹窗一致，打开即回显默认值）
  for (const f of settingFields.value) {
    if (values[f.key] === undefined && f.default !== '') values[f.key] = f.default
  }
  settingsValues.value = values
  settingsVisible.value = true
}

async function saveSettings() {
  if (!settingsPlugin.value) return
  savingSettings.value = true
  try {
    await pluginApi.saveSettings(settingsPlugin.value.name, settingsValues.value)
    MessagePlugin.success(t('plugins.saved'))
    settingsVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    savingSettings.value = false
  }
}

// 恢复默认：清空全部自定义值保存
async function resetSettings() {
  if (!settingsPlugin.value) return
  savingSettings.value = true
  try {
    await pluginApi.saveSettings(settingsPlugin.value.name, {})
    MessagePlugin.success(t('plugins.resetDone'))
    settingsValues.value = {}
    settingsVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    savingSettings.value = false
  }
}

// ---------- 已装插件 ----------

const { run } = useAsync()

async function load() {
  await run(async () => {
    const resp = await pluginApi.list()
    plugins.value = resp.plugins ?? []
  })
}

// t-upload 自定义上传：multipart 直发安装端点
async function uploadInstall({ raw }: { raw: File }): Promise<ResponseType> {
  const resp = await pluginApi.uploadInstall(raw)
  if (resp.ok) {
    MessagePlugin.success(t('plugins.installOk'))
    await load()
    if (marketVisible.value) await loadMarket()
    return { status: 'success' }
  }
  return { status: 'fail', error: { message: (await resp.text()).slice(0, 200) } } as any
}

function onUploadFail({ file }: any) {
  MessagePlugin.error(t('plugins.uploadFailed', { name: file?.name ?? '' }))
}

// 重启：停止 → 运行（已停止的插件等同启动）；停止不弹窗，卡片直接挂「已停止」标签
function restart(p: PluginInfo) {
  return runOp(`${t('plugins.restart')} · ${p.label || p.name}`, ['stopping', 'starting'], async (enter, log) => {
    enter('stopping'); log(t('plugins.logStopping'))
    await pluginApi.stop(p.name)
    enter('starting'); log(t('plugins.logRunning'))
    await pluginApi.start(p.name)
  }, t('plugins.restarted'))
}

async function stop(name: string) {
  try {
    await pluginApi.stop(name)
    MessagePlugin.success(t('plugins.stopped'))
  } catch (e: any) {
    MessagePlugin.error(e.message)
  }
  await load()
}

onMounted(load)
</script>

<style scoped>
.plugin-head { display: flex; align-items: center; gap: 12px; }
.plugin-name { font-weight: 600; line-height: 1.3; }
.plugin-sub { font-size: 12px; color: var(--td-text-color-secondary); display: flex; align-items: center; gap: 6px; }
.proto-tag { font-family: ui-monospace, monospace; }
.methods-title { font-size: 13px; color: var(--td-text-color-secondary); margin-bottom: 4px; }
.plugin-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(350px, 1fr)); gap: 14px; }
.mono { font-family: ui-monospace, monospace; font-size: 12px; }

/* 市场卡片 */
.market-card { position: relative; border: 1px solid var(--td-component-stroke); border-radius: 10px; padding: 12px 14px; margin-bottom: 12px; overflow: hidden; }
.market-version {
  position: absolute; top: 0; right: 0; margin: 0; padding: 2px 10px; border: none;
  border-bottom-left-radius: 12px; border-top-right-radius: 10px;
  background: var(--td-brand-color-light); color: var(--td-brand-color);
  font-variant-numeric: tabular-nums; font-family: ui-monospace, monospace;
}
.market-head { display: flex; align-items: center; gap: 10px; }
.market-meta { flex: 1; min-width: 0; }
.market-name { font-weight: 600; line-height: 1.3; }
.market-sub { font-size: 12px; color: var(--td-text-color-secondary); }
.market-foot { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; }
.market-date { font-size: 12px; color: var(--td-text-color-placeholder); }

/* 插件源卡片：每行三个，首卡为添加 */
.source-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
.source-card {
  border: 1px solid var(--td-component-stroke); border-radius: 10px; padding: 12px;
  display: flex; flex-direction: column; gap: 6px; min-height: 120px;
}
.source-card.disabled { opacity: 0.55; }
.source-add {
  align-items: center; justify-content: center; cursor: pointer;
  border-style: dashed; color: var(--td-text-color-secondary);
}
.source-add:hover { border-color: var(--td-brand-color); color: var(--td-brand-color); }
.source-add-plus { font-size: 28px; line-height: 1; }
.source-name { font-weight: 600; display: flex; align-items: center; gap: 6px; }
.source-link { color: inherit; text-decoration: none; }
.source-link:hover { color: var(--td-brand-color); text-decoration: underline; }
.source-count { font-size: 12px; color: var(--td-text-color-secondary); }
.source-ops { margin-top: auto; display: flex; align-items: center; gap: 10px; }
.source-edit { margin-left: auto; }
</style>
