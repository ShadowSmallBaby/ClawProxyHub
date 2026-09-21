<template>
  <!-- 设置页：tab 切换，页面整体固定占满内容区，tab 内容各自内部滚动 -->
  <div class="settings-page">
    <c-tabs v-model="tab" class="settings-tabs" size="medium">
      <t-tab-panel value="gateway" :label="$t('settings.gateway')">
        <div class="panel">
          <t-form label-width="140px">
            <t-form-item :label="$t('settings.firstTokenTimeout')" :help="$t('settings.firstTokenTimeoutHelp')">
              <t-input-number v-model="gwForm.first_event_timeout" :min="5" :max="3600" theme="column" style="width: 160px" />
            </t-form-item>
            <t-form-item :label="$t('settings.userAgent')" :help="$t('settings.userAgentHelp')">
              <t-input v-model="gwForm.user_agent" :placeholder="$t('settings.uaPh')" style="width: 480px" />
            </t-form-item>
            <t-form-item :label="$t('settings.browserUserAgent')" :help="$t('settings.browserUserAgentHelp')">
              <t-input v-model="gwForm.browser_user_agent" :placeholder="$t('settings.uaPh')" style="width: 480px" />
            </t-form-item>
            <t-form-item>
              <t-button theme="primary" :loading="saving" @click="save({ first_event_timeout: gwForm.first_event_timeout, user_agent: gwForm.user_agent.trim(), browser_user_agent: gwForm.browser_user_agent.trim() })">{{ $t('common.save') }}</t-button>
            </t-form-item>
          </t-form>
        </div>
      </t-tab-panel>

      <t-tab-panel value="network" :label="$t('settings.network')">
        <div class="panel">
          <t-form label-width="140px">
            <t-form-item :label="$t('settings.githubProxy')" :help="$t('settings.githubProxyHelp')">
              <t-input v-model="netForm.github_proxy" placeholder="https://ghproxy.com" style="width: 360px" />
            </t-form-item>
            <t-form-item>
              <t-button theme="primary" :loading="saving" @click="save({ github_proxy: netForm.github_proxy.trim() })">{{ $t('common.save') }}</t-button>
            </t-form-item>
          </t-form>
        </div>
      </t-tab-panel>

      <t-tab-panel value="logs" :label="$t('settings.logs')">
        <div class="panel">
          <t-form label-width="140px">
            <t-form-item :label="$t('settings.logRetention')" :help="$t('settings.logRetentionHelp')">
              <t-select v-model="logForm.log_retention_days" style="width: 200px" @change="save({ log_retention_days: logForm.log_retention_days })">
                <t-option :value="0" :label="$t('settings.retentionForever')" />
                <t-option v-for="d in [7, 14, 30, 60, 90, 180, 365]" :key="d" :value="d" :label="$t('settings.retentionDays', { n: d })" />
              </t-select>
            </t-form-item>
            <t-form-item :label="$t('settings.logExport')" :help="$t('settings.logExportHelp')">
              <t-button variant="outline" :loading="exporting" @click="exportLogs">
                <template #icon><download-icon /></template>{{ $t('settings.exportBtn') }}
              </t-button>
            </t-form-item>
            <t-form-item :label="$t('settings.logClear')" :help="$t('settings.logClearHelp')">
              <t-button theme="danger" variant="outline" @click="confirmClear">
                <template #icon><delete-icon /></template>{{ $t('settings.clearBtn') }}
              </t-button>
            </t-form-item>
          </t-form>
        </div>
      </t-tab-panel>

      <t-tab-panel value="system" :label="$t('settings.system')">
        <div class="panel">
          <div class="section-title">{{ $t('settings.sysInfo') }}</div>
          <t-descriptions v-if="sys" :column="2" bordered size="small" class="sys-desc">
            <t-descriptions-item :label="$t('settings.sysVersion')">v{{ sys.version }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysProtocol')">v{{ sys.protocol_version }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysRuntime')">{{ sys.go_version }} · {{ sys.os }}/{{ sys.arch }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysStarted')">{{ sys.started_at.replace('T', ' ').slice(0, 19) }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysUptime')">{{ uptime }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysDataDir')"><code>{{ sys.data_dir }}</code></t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysDbSize')">{{ fmtBytes(sys.db_size_bytes) }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysMigration')">{{ sys.migration_version }}</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysMem')">{{ fmtBytes(sys.mem_alloc_bytes) }} · {{ sys.goroutines }} goroutines</t-descriptions-item>
            <t-descriptions-item :label="$t('settings.sysCounts')">
              <span class="counts">
                <span v-for="c in countItems" :key="c.key"><b>{{ sys.counts[c.key] ?? 0 }}</b> {{ $t(c.label) }}</span>
              </span>
            </t-descriptions-item>
          </t-descriptions>
          <t-alert v-if="sys?.pending_restore" theme="warning" class="restore-alert" :message="$t('settings.pendingRestore')" />

          <!-- 站点品牌：logo / 名称 / 缩写；留空恢复默认 -->
          <div class="section-title">{{ $t('settings.siteBrand') }}</div>
          <t-form label-width="140px" class="sys-form">
            <t-form-item :label="$t('settings.siteLogo')" :help="$t('settings.siteLogoHelp')">
              <div class="logo-row">
                <img class="logo-preview" :src="siteForm.site_logo || '/logo.png'" alt="logo" />
                <input ref="logoEl" type="file" accept="image/png,image/jpeg,image/svg+xml,image/webp" hidden @change="onPickLogo" />
                <t-button variant="outline" @click="logoEl?.click()">
                  <template #icon><upload-icon /></template>{{ $t('settings.siteLogoPick') }}
                </t-button>
                <t-button v-if="siteForm.site_logo" variant="text" theme="default" @click="siteForm.site_logo = ''">{{ $t('settings.siteLogoReset') }}</t-button>
              </div>
            </t-form-item>
            <t-form-item :label="$t('settings.siteName')" :help="$t('settings.siteNameHelp')">
              <t-input v-model="siteForm.site_name" :maxlength="32" placeholder="ClawProxyHub" clearable style="width: 280px" />
            </t-form-item>
            <t-form-item :label="$t('settings.siteAbbr')" :help="$t('settings.siteAbbrHelp')">
              <t-input v-model="siteForm.site_abbr" :maxlength="8" placeholder="CPH" clearable style="width: 160px" />
            </t-form-item>
            <t-form-item>
              <t-button theme="primary" :loading="saving" @click="saveSite">{{ $t('common.save') }}</t-button>
            </t-form-item>
          </t-form>

          <div class="section-title">{{ $t('settings.backupTitle') }}</div>
          <t-form label-width="140px" class="sys-form">
            <t-form-item :label="$t('settings.backupExport')" :help="$t('settings.backupExportHelp')">
              <t-button variant="outline" :loading="backingUp" @click="exportBackup">
                <template #icon><download-icon /></template>{{ $t('settings.backupBtn') }}
              </t-button>
            </t-form-item>
            <t-form-item :label="$t('settings.backupImport')" :help="$t('settings.backupImportHelp')">
              <input ref="fileEl" type="file" accept=".zip" hidden @change="onPickBackup" />
              <t-button variant="outline" :loading="restoring" @click="fileEl?.click()">
                <template #icon><upload-icon /></template>{{ $t('settings.restoreBtn') }}
              </t-button>
            </t-form-item>
          </t-form>
        </div>
      </t-tab-panel>
    </c-tabs>
  </div>
</template>

<script setup lang="ts">
import { CTabs } from '../../components/base'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { DeleteIcon, DownloadIcon, UploadIcon } from 'tdesign-icons-vue-next'
import { settingsApi, systemApi, type SysInfo } from '../../api/settings'
import { logsApi } from '../../api/logs'
import { refreshBranding } from '../../utils/branding'

const { t } = useI18n()

const tab = ref('gateway')
const gwForm = reactive({ first_event_timeout: 90, user_agent: '', browser_user_agent: '' })
const netForm = reactive({ github_proxy: '' })
const logForm = reactive({ log_retention_days: 0 })
const siteForm = reactive({ site_name: '', site_abbr: '', site_logo: '' })
const saving = ref(false)
const exporting = ref(false)
const backingUp = ref(false)
const restoring = ref(false)
const fileEl = ref<HTMLInputElement>()
const logoEl = ref<HTMLInputElement>()

const sys = ref<SysInfo | null>(null)
const countItems = [
  { key: 'plugins', label: 'settings.countPlugins' }, { key: 'instances', label: 'settings.countInstances' },
  { key: 'accounts', label: 'settings.countAccounts' }, { key: 'groups', label: 'settings.countGroups' },
  { key: 'routes', label: 'settings.countRoutes' }, { key: 'keys', label: 'settings.countKeys' },
  { key: 'request_logs', label: 'settings.countLogs' }, { key: 'task_runs', label: 'settings.countRuns' },
]
const uptime = computed(() => {
  const s = sys.value?.uptime_seconds ?? 0
  return t('settings.uptimeFmt', { d: Math.floor(s / 86400), h: Math.floor((s % 86400) / 3600), m: Math.floor((s % 3600) / 60) })
})

async function load() {
  const r = await settingsApi.get()
  gwForm.first_event_timeout = r.settings?.first_event_timeout ?? 90
  gwForm.user_agent = r.settings?.user_agent ?? ''
  gwForm.browser_user_agent = r.settings?.browser_user_agent ?? ''
  netForm.github_proxy = r.settings?.github_proxy ?? ''
  logForm.log_retention_days = r.settings?.log_retention_days ?? 0
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
const exportLogs = () => logsApi.exportCsv(exporting).catch((e: any) => MessagePlugin.error(e.message))
const exportBackup = () => systemApi.backup(backingUp).catch((e: any) => MessagePlugin.error(e.message))

function confirmClear() {
  const dlg = DialogPlugin.confirm({
    header: t('settings.clearConfirmTitle'), body: t('settings.clearConfirmBody'), theme: 'danger',
    onConfirm: async () => {
      try {
        const r = await logsApi.clear()
        MessagePlugin.success(t('settings.cleared', { n: r.deleted }))
        loadSys()
      } catch (e: any) {
        MessagePlugin.error(e.message)
      }
      dlg.destroy()
    },
  })
}

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

function fmtBytes(n: number): string {
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(2) + ' GB'
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(1) + ' KB'
  return n + ' B'
}

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
.settings-tabs :deep(.t-tabs__content) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.panel {
  padding: 24px 28px 32px;
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
</style>
