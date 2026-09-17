<template>
  <div class="page">
    <div class="page-header">
      
      <t-space>
        <t-button variant="outline" :loading="marketLoading" @click="loadMarket">{{ $t('plugins.market') }}</t-button>
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
                <div class="plugin-sub">v{{ p.version }} · {{ p.author }}</div>
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
              <t-link theme="primary" @click="restart(p.name)">{{ $t('plugins.restart') }}</t-link>
              <t-link theme="warning" @click="stop(p.name)">{{ $t('plugins.stop') }}</t-link>
              <t-popconfirm :content="$t('plugins.confirmUninstall', { name: p.label || p.name })" @confirm="uninstall(p.name)">
                <t-link theme="danger">{{ $t('plugins.uninstall') }}</t-link>
              </t-popconfirm>
            </t-space>
          </t-space>
        </t-card>
    </div>

    <!-- 市场 -->
    <t-drawer v-model:visible="marketVisible" :header="$t('plugins.market')" size="420px">
      <t-alert
        v-if="marketSource === 'offline'"
        theme="warning"
        :message="$t('plugins.offlineHint')"
        style="margin-bottom: 12px"
      />
      <t-empty v-if="!marketEntries.length" :description="$t('plugins.marketEmpty')" />
      <div class="market-card" v-for="e in marketEntries" :key="e.author + '/' + e.name">
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
          <t-button
            v-if="!e.installed"
            size="small"
            theme="primary"
            :loading="installing === e.author + '/' + e.name"
            @click="installFromMarket(e)"
          >
            {{ $t('plugins.install') }}
          </t-button>
          <t-button
            v-else-if="e.updatable"
            size="small"
            theme="warning"
            variant="outline"
            :loading="installing === e.author + '/' + e.name"
            @click="installFromMarket(e)"
          >
            {{ $t('plugins.upgrade') }}
          </t-button>
          <t-tag v-else size="small" theme="success" variant="light">{{ $t('plugins.installed') }}</t-tag>
        </div>
      </div>
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
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import type { ResponseType } from 'tdesign-vue-next'
import { api, getToken } from '../api/client'
import { capabilityDict, dict, label } from '../utils/dict'
import type { PluginInfo } from '../api/types'

const { t } = useI18n()

const plugins = ref<PluginInfo[]>([])
const marketVisible = ref(false)
const marketLoading = ref(false)
const marketEntries = ref<{
  name: string
  version: string
  author?: string
  icon?: string
  label?: Record<string, string>
  published_at?: string
  installed?: boolean
  updatable?: boolean
}[]>([])
const marketSource = ref('')
const installing = ref('')

// ---------- 插件设置 ----------

interface SettingField {
  key: string
  title: string
  description: string
  type: string
  default: unknown
  options: unknown[]
}

const settingsVisible = ref(false)
const settingsPlugin = ref<PluginInfo | null>(null)
const settingsValues = ref<Record<string, any>>({})
const settingFields = ref<SettingField[]>([])
const savingSettings = ref(false)

async function openSettings(p: PluginInfo) {
  settingsPlugin.value = p
  const resp = await api.get<{ schema: Record<string, any>; values: Record<string, any> }>(
    `/admin/plugins/${p.name}/settings`,
  )
  const props = resp.schema?.properties ?? {}
  settingFields.value = Object.entries(props).map(([key, def]: [string, any]) => ({
    key,
    title: def.title ?? key,
    description: def.description ?? '',
    type: def.type ?? 'string',
    default: def.default ?? '',
    options: def.enum ?? [],
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

async function load() {
  const resp = await api.get<{ plugins: PluginInfo[] }>('/admin/plugins')
  plugins.value = resp.plugins ?? []
}

async function loadMarket() {
  marketLoading.value = true
  try {
    const resp = await api.get<{ plugins: typeof marketEntries.value; source?: string }>('/admin/plugins/marketplace')
    marketEntries.value = resp.plugins ?? []
    marketSource.value = resp.source ?? ''
    marketVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    marketLoading.value = false
  }
}

function installFromMarket(e: (typeof marketEntries.value)[number]) {
  installing.value = e.author + '/' + e.name
  try {
    void (async () => {
      await api.post('/admin/plugins/install-market', { name: e.name, author: e.author ?? '' })
      MessagePlugin.success(t('plugins.installedN', { name: e.name }))
      await load()
      // 弹窗开着时同步刷新市场条目的安装状态
      if (marketVisible.value) await loadMarket()
    })()
  } catch (err: any) {
    MessagePlugin.error(err.message)
  }
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
    // 弹窗开着时同步刷新市场条目的安装状态
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

async function uninstall(name: string) {
  try {
    await api.del(`/admin/plugins/${name}`)
    MessagePlugin.success(t('plugins.uninstalledN', { name }))
    await load()
    // 弹窗开着时同步刷新市场条目的安装状态
    if (marketVisible.value) await loadMarket()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  }
}

onMounted(load)
</script>

<style scoped>
.plugin-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.plugin-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 700;
  overflow: hidden;
  flex-shrink: 0;
}
.plugin-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.plugin-name {
  font-weight: 600;
  line-height: 1.3;
}
.plugin-sub {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.methods-title {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin-bottom: 4px;
}
.plugin-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 14px;
}
.market-card {
  position: relative;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  padding: 12px 14px;
  margin-bottom: 12px;
  overflow: hidden;
}
.market-version {
  position: absolute;
  top: 0;
  right: 0;
  margin: 0;
  padding: 2px 10px;
  border: none;
  border-bottom-left-radius: 12px;
  border-top-right-radius: 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-variant-numeric: tabular-nums;
  font-family: ui-monospace, monospace;
}
.market-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.market-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  font-weight: 700;
  overflow: hidden;
  flex-shrink: 0;
}
.market-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.market-meta {
  flex: 1;
  min-width: 0;
}
.market-name {
  font-weight: 600;
  line-height: 1.3;
}
.market-sub {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.market-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 10px;
}
.market-date {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
