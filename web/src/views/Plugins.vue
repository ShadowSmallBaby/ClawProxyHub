<template>
  <div class="page">
    <div class="page-header">

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
    </div>

    <!-- 已安装 -->
    <t-empty v-if="!plugins.length" :description="$t('plugins.emptyInstalled')" />
    <div class="plugin-grid">
      <t-card v-for="p in plugins" :key="p.name">
        <template #header>
          <div class="plugin-head">
            <div class="plugin-icon">
              <img v-if="p.icon" :src="p.icon" :alt="p.label || p.name" />
              <span v-else>{{ (p.label || p.name).slice(0, 1) }}</span>
            </div>
            <div class="plugin-head-meta">
              <div class="plugin-name">{{ p.label || p.name }}</div>
              <div class="plugin-sub">
                v{{ p.version }} · {{ p.author }}
                <t-tag size="small" variant="outline" class="proto-tag" :title="$t('plugins.protocol')">P{{ p.protocol_version ?? 1 }}</t-tag>
              </div>
            </div>
          </div>
        </template>
        <t-space direction="vertical" style="width: 100%">
          <t-space size="small">
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
            <t-link theme="primary" @click="restart(p.name)">{{ $t('plugins.restart') }}</t-link>
            <t-link theme="warning" @click="stop(p.name)">{{ $t('plugins.stop') }}</t-link>
            <t-link theme="danger" @click="askUninstall(p)">{{ $t('plugins.uninstall') }}</t-link>
          </t-space>
        </t-space>
      </t-card>
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
            <div class="market-icon">
              <img v-if="e.icon" :src="e.icon" :alt="e.label?.zh ?? e.name" />
              <span v-else>{{ (e.label?.zh ?? e.name).slice(0, 1) }}</span>
            </div>
            <div class="market-meta">
              <div class="market-name">{{ label(e.label, e.name) }}</div>
              <div class="market-sub">{{ e.author || $t('plugins.unknownAuthor') }}</div>
            </div>
          </div>
          <div class="market-foot">
            <span class="market-date">{{ e.published_at || '' }}</span>
            <t-button v-if="!e.installed" size="small" theme="primary" :loading="installing === e.author + '/' + e.name" @click="installFromMarket(e)">
              {{ $t('plugins.install') }}
            </t-button>
            <t-button v-else-if="e.updatable" size="small" theme="warning" variant="outline" :loading="installing === e.author + '/' + e.name" @click="installFromMarket(e)">
              {{ $t('plugins.upgrade') }}
            </t-button>
            <t-tag v-else size="small" theme="success" variant="light">{{ $t('plugins.installed') }}</t-tag>
          </div>
        </div>
      </t-loading>
    </t-drawer>

    <!-- 插件设置：schema 动态渲染 -->
    <t-dialog
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
    </t-dialog>

    <!-- 插件实例：多实例插件的实例列表，统一在此增改删 -->
    <t-drawer v-model:visible="instancesVisible" :header="$t('plugins.instancesHeader', { name: instancesPlugin?.label || (instancesPlugin?.name ?? '') })" size="640px" :footer="false">
      <div style="margin-bottom: 12px">
        <t-button theme="primary" size="small" @click="openInstanceForm(null)">{{ $t('instances.add') }}</t-button>
      </div>
      <t-table row-key="id" :data="pluginInstances" :columns="instanceColumns" size="small">
        <template #base_url="{ row }"><span class="mono">{{ row.base_url || '-' }}</span></template>
        <template #op="{ row }">
          <t-space size="small">
            <t-link theme="primary" @click="openInstanceForm(row)">{{ $t('common.edit') }}</t-link>
            <t-link theme="danger" @click="askRemoveInstance(row)">{{ $t('common.delete') }}</t-link>
          </t-space>
        </template>
      </t-table>
    </t-drawer>
    <instance-form-dialog v-model:visible="instanceFormVisible" :plugin="instancesPlugin" :instance="instanceEditing" @saved="loadPluginInstances" />
    <delete-impact-dialog
      v-model:visible="removeVisible"
      :header="removeTarget?.header ?? ''"
      :message="removeTarget?.message ?? ''"
      :impact-url="removeTarget?.impactUrl ?? ''"
      :delete-url="removeTarget?.deleteUrl ?? ''"
      @deleted="removeTarget?.after()"
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
    <t-dialog v-model:visible="sourceFormVisible" :header="sourceEditing ? $t('plugins.sourceEdit') : $t('plugins.sourceAdd')" :confirm-btn="{ loading: savingSources }" @confirm="saveSourceForm">
      <t-form label-width="90px">
        <t-form-item :label="$t('plugins.sourceName')" required-mark>
          <t-input v-model="sourceForm.name" :disabled="sourceEditing?.name === 'official'" placeholder="my-source" />
        </t-form-item>
        <t-form-item :label="$t('plugins.sourceUrl')" required-mark>
          <t-input v-model="sourceForm.url" placeholder="https://.../index.json" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import type { ResponseType } from 'tdesign-vue-next'
import { api, getToken } from '../api/client'
import InstanceFormDialog from '../components/InstanceFormDialog.vue'
import DeleteImpactDialog from '../components/DeleteImpactDialog.vue'
import { capabilityDict, dict, label } from '../utils/dict'
import type { InstanceInfo, PluginInfo, PluginSource } from '../api/types'

const { t } = useI18n()

const plugins = ref<PluginInfo[]>([])

// ---------- 市场 ----------

interface MarketEntry {
  name: string; version: string; author?: string; icon?: string; label?: Record<string, string>
  published_at?: string; source?: string; installed?: boolean; updatable?: boolean
}
const marketVisible = ref(false)
const marketLoading = ref(false)
const marketEntries = ref<MarketEntry[]>([])
const marketOnline = ref('')
const marketSourceName = ref('official')
const installing = ref('')

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
    const resp = await api.get<{ plugins: MarketEntry[]; source?: string }>(`/admin/plugins/marketplace?source=${encodeURIComponent(marketSourceName.value)}`)
    marketEntries.value = resp.plugins ?? []
    marketOnline.value = resp.source ?? ''
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    marketLoading.value = false
  }
}

async function installFromMarket(e: MarketEntry) {
  installing.value = e.author + '/' + e.name
  try {
    await api.post('/admin/plugins/install-market', { name: e.name, author: e.author ?? '', source: e.source ?? '' })
    MessagePlugin.success(t('plugins.installedN', { name: e.name }))
    await load()
    if (marketVisible.value) await loadMarket()
  } catch (err: any) {
    MessagePlugin.error(err.message)
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
  const resp = await api.get<{ sources: PluginSource[] }>('/admin/plugin-sources')
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
    await api.put('/admin/plugin-sources', { sources: list.map((s) => ({ name: s.name, url: s.url, enabled: s.enabled })) })
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
      await api.get(`/admin/plugin-sources/probe?url=${encodeURIComponent(url)}`)
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
  const resp = await api.get<{ instances: InstanceInfo[] }>(`/admin/instances?plugin_id=${instancesPlugin.value.id}`)
  pluginInstances.value = resp.instances ?? []
}

function openInstanceForm(row: InstanceInfo | null) {
  instanceEditing.value = row
  instanceFormVisible.value = true
}

// ---------- 删除确认（插件卸载 / 实例删除共用一个影响面弹窗） ----------

interface RemoveTarget { header: string; message: string; impactUrl: string; deleteUrl: string; after: () => void }
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

function askUninstall(p: PluginInfo) {
  const name = p.label || p.name
  removeTarget.value = {
    header: t('plugins.uninstall') + ' · ' + name,
    message: t('plugins.confirmUninstall', { name }),
    impactUrl: `/admin/plugins/${p.name}/impact`,
    deleteUrl: `/admin/plugins/${p.name}`,
    after: async () => {
      MessagePlugin.success(t('plugins.uninstalledN', { name: p.name }))
      await load()
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
  const resp = await api.get<{ schema: Record<string, any>; values: Record<string, any> }>(`/admin/plugins/${p.name}/settings`)
  const props = resp.schema?.properties ?? {}
  settingFields.value = Object.entries(props).map(([key, def]: [string, any]) => ({
    key, title: def.title ?? key, description: def.description ?? '',
    type: def.type ?? 'string', default: def.default ?? '', options: def.enum ?? [],
  }))
  settingsValues.value = { ...(resp.values ?? {}) }
  settingsVisible.value = true
}

async function saveSettings() {
  if (!settingsPlugin.value) return
  savingSettings.value = true
  try {
    await api.put(`/admin/plugins/${settingsPlugin.value.name}/settings`, { values: settingsValues.value })
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
    await api.put(`/admin/plugins/${settingsPlugin.value.name}/settings`, { values: {} })
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

async function load() {
  const resp = await api.get<{ plugins: PluginInfo[] }>('/admin/plugins')
  plugins.value = resp.plugins ?? []
}

// t-upload 自定义上传：multipart 直发安装端点
async function uploadInstall({ raw }: { raw: File }): Promise<ResponseType> {
  const form = new FormData()
  form.append('package', raw)
  const resp = await fetch('/admin/plugins/install-upload', {
    method: 'POST',
    headers: { Authorization: `Bearer ${getToken()}` },
    body: form,
  })
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

async function restart(name: string) {
  await api.post(`/admin/plugins/${name}/stop`)
  await api.post(`/admin/plugins/${name}/start`)
  MessagePlugin.success(t('plugins.restarted'))
  await load()
}

async function stop(name: string) {
  await api.post(`/admin/plugins/${name}/stop`)
  MessagePlugin.success(t('plugins.stopped'))
  await load()
}

onMounted(load)
</script>

<style scoped>
.plugin-head { display: flex; align-items: center; gap: 12px; }
.plugin-icon {
  width: 44px; height: 44px; border-radius: 10px;
  background: var(--td-brand-color-light); color: var(--td-brand-color);
  display: flex; align-items: center; justify-content: center;
  font-size: 20px; font-weight: 700; overflow: hidden; flex-shrink: 0;
}
.plugin-icon img { width: 100%; height: 100%; object-fit: cover; }
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
.market-icon {
  width: 40px; height: 40px; border-radius: 10px;
  background: var(--td-brand-color-light); color: var(--td-brand-color);
  display: flex; align-items: center; justify-content: center;
  font-size: 18px; font-weight: 700; overflow: hidden; flex-shrink: 0;
}
.market-icon img { width: 100%; height: 100%; object-fit: cover; }
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
