<template>
  <!-- 设置页：tab 切换，页面整体固定占满内容区，tab 内容各自内部滚动（三端统一） -->
  <div class="settings-page">
    <!-- 桌面/平板/手机：tab 切换统一 -->
    <c-tabs v-model="tab" class="settings-tabs" size="medium">
      <t-tab-panel v-if="gatewayEnabled" value="gateway" :label="$t('settings.gateway')">
        <component :is="gateway.settings" ref="gatewaySettings" :settings="loadedSettings" :save="save" :saving="saving" />
      </t-tab-panel>

      <t-tab-panel v-if="!supportsConnections" value="network" :label="$t('settings.network')">
        <div class="panel">
          <div class="s-form">
            <form-item :label="$t('settings.githubProxy')" :tip="$t('settings.githubProxyHelp')">
              <t-input v-model="netForm.github_proxy" placeholder="https://ghproxy.com" class="w-xl" />
            </form-item>
            <div class="save-row">
              <t-button theme="primary" :loading="saving" @click="save({ github_proxy: netForm.github_proxy.trim() })">{{ $t('common.save') }}</t-button>
            </div>
          </div>
        </div>
      </t-tab-panel>

      <t-tab-panel value="logs" :label="$t('settings.logs')">
        <div class="panel">
          <div class="s-form">
            <form-item :label="$t('settings.logRetention')" :tip="$t('settings.logRetentionHelp')">
              <t-select v-model="logForm.log_retention_days" class="w-sm" @change="save({ log_retention_days: logForm.log_retention_days })">
                <t-option :value="0" :label="$t('settings.retentionForever')" />
                <t-option v-for="d in [7, 14, 30, 60, 90, 180, 365]" :key="d" :value="d" :label="$t('settings.retentionDays', { n: d })" />
              </t-select>
            </form-item>
            <form-item :label="$t('settings.runLevel')" :tip="$t('settings.runLevelHelp')">
              <t-select v-model="logForm.run_level" class="w-sm" @change="save({ run_level: logForm.run_level })">
                <t-option value="error" :label="$t('settings.runLevelError')" />
                <t-option value="warn" :label="$t('settings.runLevelWarn')" />
                <t-option value="debug" :label="$t('settings.runLevelDebug')" />
                <t-option value="info" :label="$t('settings.runLevelInfo')" />
              </t-select>
            </form-item>
            <component :is="gateway.logActions" v-if="gatewayEnabled && gateway.logActions" @changed="loadSys" />
          </div>
        </div>
      </t-tab-panel>

      <t-tab-panel value="task" :label="$t('settings.task')">
        <div class="panel">
          <div class="s-form">
            <form-item :label="$t('settings.timezone')" :tip="$t('settings.timezoneHelp')">
              <t-auto-complete v-model="taskForm.timezone" :options="timezoneOptions" filterable class="w-md" placeholder="Asia/Shanghai" :disabled="saving" />
            </form-item>
            <form-item :label="$t('settings.taskJitter')" :tip="$t('settings.taskJitterHelp')">
              <t-input-number v-model="taskForm.task_daily_jitter" :min="0" :max="45" :suffix="$t('settings.taskJitterUnit')" theme="column" class="w-sm" />
            </form-item>
            <div class="save-row">
              <t-button theme="primary" :loading="saving" @click="saveTask">{{ $t('common.save') }}</t-button>
            </div>
          </div>
        </div>
      </t-tab-panel>

      <t-tab-panel value="mcp" label="MCP">
        <MCPSettings v-if="tab === 'mcp'" ref="mcpSettings" />
      </t-tab-panel>
      <t-tab-panel v-if="!supportsConnections" value="system" :label="$t('settings.system')">
        <div class="panel">
          <!-- 系统信息：手机端走独立抽屉，仅 PC 展示 -->
          <template v-if="!isPhone">
            <div class="section-title">{{ $t('settings.sysInfo') }}</div>
            <sys-info-card :sys="sys" :column="2" />
          </template>

          <!-- 站点品牌：logo / 名称 / 缩写；留空恢复默认 -->
          <div class="section-title">{{ $t('settings.siteBrand') }}</div>
          <div class="s-form sys-form">
            <form-item :label="$t('settings.siteLogo')" :tip="$t('settings.siteLogoHelp')">
              <div class="logo-row">
                <img class="logo-preview" :src="siteForm.site_logo ? resourceURL(siteForm.site_logo) : DEFAULT_LOGO" alt="logo" />
                <input ref="logoEl" type="file" accept="image/png,image/jpeg,image/svg+xml,image/webp" hidden @change="onPickLogo" />
                <t-button variant="outline" @click="logoEl?.click()">
                  <template #icon><upload-icon /></template>{{ $t('settings.siteLogoPick') }}
                </t-button>
                <t-button v-if="siteForm.site_logo" variant="text" theme="default" @click="siteForm.site_logo = ''">{{ $t('settings.siteLogoReset') }}</t-button>
              </div>
            </form-item>
            <form-item :label="$t('settings.siteName')" :tip="$t('settings.siteNameHelp')">
              <t-input v-model="siteForm.site_name" :maxlength="32" placeholder="ClawProxyHub" clearable class="w-md" />
            </form-item>
            <form-item :label="$t('settings.siteAbbr')" :tip="$t('settings.siteAbbrHelp')">
              <t-input v-model="siteForm.site_abbr" :maxlength="8" placeholder="CPH" clearable class="w-sm" />
            </form-item>
            <div class="save-row">
              <t-button theme="primary" :loading="saving" @click="saveSite">{{ $t('common.save') }}</t-button>
            </div>
          </div>

          <div class="section-title">{{ $t('settings.backupTitle') }}</div>
          <div class="s-form sys-form">
            <form-item :label="$t('settings.backupExport')" :tip="$t('settings.backupExportHelp')">
              <t-button variant="outline" :loading="backingUp" @click="exportBackup">
                <template #icon><download-icon /></template>{{ $t('settings.backupBtn') }}
              </t-button>
            </form-item>
            <form-item :label="$t('settings.backupImport')" :tip="$t('settings.backupImportHelp')">
              <input ref="fileEl" type="file" accept=".zip" hidden @change="onPickBackup" />
              <t-button variant="outline" :loading="restoring" @click="fileEl?.click()">
                <template #icon><upload-icon /></template>{{ $t('settings.restoreBtn') }}
              </t-button>
            </form-item>
          </div>
        </div>
      </t-tab-panel>
    </c-tabs>

    <!-- 手机端：分区标题 + 悬浮按钮切分区/保存/系统信息 -->
    <div v-if="isPhone" class="phone-tab">{{ currentTabLabel }}</div>
    <mobile-fab v-if="isPhone">
      <t-popup v-model:visible="menuOpen" placement="top-right" trigger="click">
        <t-button theme="primary" shape="circle" size="large">
          <template #icon><setting-icon /></template>
        </t-button>
        <template #content>
          <div class="phone-menu">
            <div
              v-for="o in tabOptions"
              :key="o.value"
              class="phone-menu-item"
              :class="{ on: tab === o.value }"
              @click="pickTab(o.value)"
            >
              {{ o.label }}
            </div>
          </div>
        </template>
      </t-popup>
      <t-button v-if="!supportsConnections" theme="default" shape="circle" size="large" @click="openSys">
        <template #icon><desktop-icon /></template>
      </t-button>
      <t-button v-if="['gateway', 'network', 'task', 'system', 'mcp'].includes(tab)" theme="success" shape="circle" size="large" :loading="saving || !!mcpSettings?.loading" :aria-label="$t('common.save')" @click="saveCurrent">
        <template #icon><save-icon /></template>
      </t-button>
    </mobile-fab>

    <!-- 手机端系统信息抽屉：一列展示 -->
    <c-drawer v-if="isPhone && !supportsConnections" v-model:visible="sysOpen" :header="$t('settings.sysInfo')" :footer="false">
      <sys-info-card :sys="sys" />
    </c-drawer>

  </div>
</template>

<script setup lang="ts">
import { CDrawer, CTabs, MobileFab } from '@/components/base'
import { FormItem, SysInfoCard } from '@/components'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { DownloadIcon, UploadIcon, SettingIcon, SaveIcon, DesktopIcon } from 'tdesign-icons-vue-next'
import { settingsApi, systemApi, type SysInfo, type AdminSettings } from '@/api/settings'
import { refreshBranding, DEFAULT_LOGO } from '@/utils/branding'
import { resourceURL } from '@/api/client'
import { supportsConnections } from '@/api/connections'
import { gateway } from '@profile'
import { hasCapability } from '@/features/access'
import { useIsMobile } from '@/composables'
import MCPSettings from './MCPSettings.vue'

const { t } = useI18n()
const gatewayEnabled = computed(() => !!gateway.settings && hasCapability('gateway'))
const { isPhone } = useIsMobile()

const tabOptions = computed(() => [
  ...(gatewayEnabled.value ? [{ value: 'gateway', label: t('settings.gateway') }] : []),
  ...(!supportsConnections ? [{ value: 'network', label: t('settings.network') }] : []),
  { value: 'logs', label: t('settings.logs') },
  { value: 'task', label: t('settings.task') },
  { value: 'mcp', label: 'MCP' },
  ...(!supportsConnections ? [{ value: 'system', label: t('settings.system') }] : []),
])

const tab = ref(gatewayEnabled.value ? 'gateway' : supportsConnections ? 'task' : 'network')
const sysOpen = ref(false)
const menuOpen = ref(false)

// 悬浮菜单切分区：选完自动收起；system 进入时加载系统信息
function pickTab(v: string) {
  menuOpen.value = false
  tab.value = v
  if (v === 'system') loadSys()
}

// 系统信息抽屉（独立悬浮按钮）
function openSys() {
  sysOpen.value = true
  loadSys()
}
const currentTabLabel = computed(() => tabOptions.value.find((o) => o.value === tab.value)?.label ?? '')

// 手机端悬浮保存：按 tab 分块提交当前块
function saveCurrent() {
  if (tab.value === 'gateway') gatewaySettings.value?.save()
  else if (tab.value === 'network') save({ github_proxy: netForm.github_proxy.trim() })
  else if (tab.value === 'task') saveTask()
  else if (tab.value === 'system') saveSite()
  else if (tab.value === 'mcp') mcpSettings.value?.save()
}
function saveTask() {
  return save({ timezone: taskForm.timezone.trim(), task_daily_jitter: taskForm.task_daily_jitter })
}
const loadedSettings = ref<AdminSettings>()
const gatewaySettings = ref<{ save: () => Promise<void> }>()
const mcpSettings = ref<InstanceType<typeof MCPSettings>>()
const netForm = reactive({ github_proxy: '' })
const logForm = reactive({ log_retention_days: 0, run_level: 'error' })
const taskForm = reactive({ task_daily_jitter: 30, timezone: 'Asia/Shanghai' })
const timezoneOptions = computed(() => [
  { text: 'Asia/Shanghai', label: t('settings.timezoneShanghai') },
  ...['UTC', 'Asia/Hong_Kong', 'Asia/Tokyo', 'Asia/Singapore', 'Asia/Kolkata', 'Europe/London', 'Europe/Berlin', 'America/New_York', 'America/Los_Angeles', 'Australia/Sydney'],
])
const siteForm = reactive({ site_name: '', site_abbr: '', site_logo: '' })
const saving = ref(false)
const backingUp = ref(false)
const restoring = ref(false)
const fileEl = ref<HTMLInputElement>()
const logoEl = ref<HTMLInputElement>()

const sys = ref<SysInfo | null>(null)

async function load() {
  const r = await settingsApi.get()
  loadedSettings.value = r.settings
  netForm.github_proxy = r.settings?.github_proxy ?? ''
  logForm.log_retention_days = r.settings?.log_retention_days ?? 0
  logForm.run_level = r.settings?.run_level ?? 'error'
  taskForm.task_daily_jitter = r.settings?.task_daily_jitter ?? 30
  taskForm.timezone = r.settings?.timezone ?? 'Asia/Shanghai'
  siteForm.site_name = r.settings?.site_name ?? ''
  siteForm.site_abbr = r.settings?.site_abbr ?? ''
  siteForm.site_logo = r.settings?.site_logo ?? ''
}
async function loadSys() {
  sys.value = await systemApi.info().catch(() => null)
}

// 按 tab 分块保存：只提交本块字段，其余保持原值
async function save(patch: Record<string, unknown>) {
  saving.value = true
  try {
    await settingsApi.save(patch)
    MessagePlugin.success(t('settings.saved'))
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    saving.value = false
  }
}

// 站点品牌保存后刷新全局品牌（侧栏 / 标题即时生效）
async function saveSite() {
  await save({ site_name: siteForm.site_name.trim(), site_abbr: siteForm.site_abbr.trim(), site_logo: siteForm.site_logo })
  refreshBranding()
}

// logo 读成 data URL 存进设置（限 200KB 原图，base64 后约 270KB 以内）
function onPickLogo(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (logoEl.value) logoEl.value.value = ''
  if (!file) return
  if (file.size > 190 * 1024) {
    MessagePlugin.warning(t('settings.siteLogoTooBig'))
    return
  }
  const reader = new FileReader()
  reader.onload = () => { siteForm.site_logo = String(reader.result ?? '') }
  reader.readAsDataURL(file)
}

// 带鉴权头下载：API 层负责 blob 落成文件，此处只管错误提示
const exportBackup = () => systemApi.backup(backingUp).catch((e: any) => MessagePlugin.error(e.message))

function onPickBackup(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (fileEl.value) fileEl.value.value = ''
  if (!file) return
  const dlg = DialogPlugin.confirm({
    header: t('settings.restoreConfirmTitle'), body: t('settings.restoreConfirmBody'), theme: 'warning',
    onConfirm: async () => {
      dlg.destroy()
      restoring.value = true
      try {
        await systemApi.restore(file)
        MessagePlugin.success(t('settings.restoreQueued'))
        loadSys()
      } catch (e: any) {
        MessagePlugin.error(e.message)
      } finally {
        restoring.value = false
      }
    },
  })
}

// PC system tab 切入时加载系统信息（手机端走抽屉 pickTab）
// PC tab 切入 system 时加载（手机端走 pickTab/openSys）
watch(tab, (v) => { if (v === 'system') loadSys() })
onMounted(load)
</script>

<style scoped>
/* 页面占满内容区高度，不随内容增长；tab 头固定，面板内部滚动 */
.settings-page {
  height: 100%;
  padding: 20px 28px 24px;
  box-sizing: border-box;
  display: flex;
  justify-content: center;
}
.settings-tabs {
  width: 100%;
  max-width: 1080px;
  height: 100%;
  display: flex;
  flex-direction: column;
  border-radius: 12px;
  background: var(--td-bg-color-container);
  box-shadow: var(--td-shadow-1);
  overflow: hidden;
}
.settings-tabs :deep(.t-tabs__header) {
  flex-shrink: 0;
  padding: 0 12px;
}
.panel {
  padding: 24px 28px 32px;
}
@media (max-width: 767px) {
  .settings-page {
    padding: 0;
  }
  .panel {
    padding: 16px 12px 24px;
  }
}
.settings-tabs :deep(.t-tabs__content) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}
/* panel 撑满滚动容器（三端统一固定高度，内容滚动） */
.settings-tabs :deep(.t-tab-panel) {
  flex: 1;
  min-height: 0;
}
.section-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 12px;
}
.sys-desc {
  margin-bottom: 20px;
}
.sys-desc code {
  font-size: 12px;
  word-break: break-all;
}
.counts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.counts b {
  color: var(--td-text-color-primary);
  font-variant-numeric: tabular-nums;
}
.restore-alert {
  margin-bottom: 16px;
}
.sys-form {
  margin-top: 8px;
  margin-bottom: 20px;
}
.logo-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.logo-preview {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  border: 1px solid var(--td-component-border);
  object-fit: cover;
}
.save-row {
  padding: 16px 0 0;
}
/* 手机端：保存走悬浮按钮，隐藏 PC 行内保存 */
@media (max-width: 767px) {
  .save-row {
    display: none;
  }
}

/* 手机端：分区标题 + 悬浮菜单样式 */
.phone-tab {
  position: fixed;
  top: 60px;
  left: 12px;
  z-index: 90;
  font-size: 15px;
  font-weight: 700;
  pointer-events: none;
}
.phone-menu {
  display: flex;
  flex-direction: column;
  min-width: 140px;
}
.phone-menu-item {
  padding: 10px 16px;
  font-size: 14px;
  cursor: pointer;
}
.phone-menu-item.on {
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
  font-weight: 600;
}

/* 手机端：tab 头隐藏（悬浮菜单替代），系统信息 tab 整个隐藏（抽屉承载） */
@media (max-width: 767px) {
  .settings-tabs :deep(.t-tabs__header) {
    display: none;
  }
  .settings-page {
    padding-top: 40px;
  }
}
</style>
