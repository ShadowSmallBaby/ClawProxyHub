<template>
  <page-layout>
    <template v-if="!isPhone" #header>
      <page-header>
        <t-space>
          <t-button variant="outline" @click="trust?.open()">{{ t('extensions.trust') }}</t-button>
          <t-button variant="outline" @click="online?.open()">{{ t('extensions.market') }}</t-button>
          <t-button :disabled="loading" @click="input?.click()">{{ t('extensions.localInstall') }}</t-button>
        </t-space>
      </page-header>
    </template>

    <input ref="input" type="file" :accept="runtimeManagement === 'native' ? '.cphext' : '.cphext,.cphhost'" hidden @change="pick">
    <t-alert v-if="runtimeManagement === 'native'" class="extension-notice" theme="info" :message="t('extensions.nativeManagement')" />
    <t-loading :loading="loading" style="width: 100%">
      <t-empty v-if="!loading && !cards.length" :description="t('common.noData')" />
      <div class="extension-list">
        <c-card v-for="card in cards" :key="card.id" :data-extension="card.id">
          <template #header>
            <div class="extension-head">
              <entity-icon :name="card.name" />
              <div class="extension-meta">
                <div class="extension-name">{{ card.name }}</div>
                <div class="extension-tags">
                  <span v-if="card.version">v{{ card.version }}</span>
                  <t-tag v-if="card.bundled" size="small" theme="primary" variant="light">{{ t('extensions.bundled') }}</t-tag>
                  <t-tag size="small" variant="light" :theme="card.state?.available ? 'success' : card.state?.enabled ? 'warning' : 'default'">
                    {{ t('extensions.state.' + status(card)) }}
                  </t-tag>
                  <t-tag v-if="(card.state?.manifest || card.bundled?.manifest)?.kind !== 'runtime' && !acceptsUI(card.state?.manifest || card.bundled?.manifest)" size="small" variant="light">{{ t('extensions.unsupportedEnvironment') }}</t-tag>
                </div>
              </div>
            </div>
          </template>
          <p v-if="card.description" class="extension-description">{{ card.description }}</p>
          <div class="extension-info">
            <div v-if="card.state || card.bundled?.manifest" class="extension-id">{{ card.id }}</div>
            <div v-if="card.publisher">{{ card.publisher }}</div>
            <div v-if="(card.state || card.bundled?.manifest) && (card.state?.manifest || card.bundled?.manifest)?.kind !== 'runtime'">{{ t('extensions.supportedEnvironments') }}: {{ environmentLabel(card.state?.manifest || card.bundled?.manifest) }}</div>
            <div>
              {{ t((card.state?.manifest || card.bundled?.manifest)?.kind === 'runtime' ? 'extensions.runtime' : 'extensions.feature') }}
              <span v-if="card.state"> · {{ fmtBytes(card.state.bytes) }}</span>
            </div>
            <div v-if="card.bundled?.status === 'update' && card.bundled.manifest?.version !== card.state?.manifest.version">{{ t('extensions.availableVersion', { version: card.bundled.manifest?.version }) }}</div>
          </div>
          <t-alert v-if="card.state?.error || card.bundled?.error" class="extension-error" theme="warning" :message="card.state?.error || card.bundled?.error" />
          <t-space class="extension-actions" size="small" break-line>
            <t-link v-if="installable(card.bundled)" theme="primary" :disabled="loading" @click="pickMounted(card.bundled!)">
              {{ installLabel(card.bundled) }}
            </t-link>
            <template v-if="card.state">
              <t-link v-if="card.state.manifest.settings?.length" theme="primary" :disabled="loading" @click="settings?.open(card.state)">{{ t('extensions.settings') }}</t-link>
              <t-link theme="primary" :disabled="loading || !card.state.enabled && !acceptsPackage(card.state.manifest)" @click="toggle(card.state)">
                {{ t(card.state.enabled ? 'extensions.disable' : 'extensions.enable') }}
              </t-link>
              <t-link theme="primary" :disabled="loading" @click="inspectImpact(card.id)">{{ t('extensions.impactShort') }}</t-link>
            </template>
            <t-link v-if="card.state || card.bundled?.manifest" theme="primary" :disabled="loading" @click="openCleanup(card.id)">{{ t('extensions.cleanup') }}</t-link>
            <t-link v-if="card.state" theme="danger" :disabled="loading" @click="inspectImpact(card.id, 'uninstall')">{{ t('extensions.uninstall') }}</t-link>
          </t-space>
        </c-card>
      </div>
    </t-loading>

    <template #overlays>
      <mobile-fab v-if="isPhone">
        <t-button theme="default" variant="outline" shape="circle" size="large" :aria-label="t('extensions.trust')" :title="t('extensions.trust')" @click="trust?.open()">
          <template #icon><app-icon /></template>
        </t-button>
        <t-button theme="default" variant="outline" shape="circle" size="large" :aria-label="t('extensions.market')" :title="t('extensions.market')" @click="online?.open()">
          <template #icon><shop-icon /></template>
        </t-button>
        <t-button shape="circle" size="large" :disabled="loading" :aria-label="t('extensions.localInstall')" :title="t('extensions.localInstall')" @click="input?.click()">
          <template #icon><cloud-upload-icon /></template>
        </t-button>
      </mobile-fab>
      <online-packages ref="online" :busy="loading" @select="pickOnline" />
      <trust-sources ref="trust" @changed="run(reload)" />
      <extension-settings ref="settings" />
      <c-drawer v-model:visible="previewVisible" :header="t('extensions.review')" width="560px" :confirm-btn="{ disabled: !authorized || !previewSupported, loading, content: previewAction }" @confirm="install">
        <div v-if="preview" class="extension-review">
          <div class="extension-review-title">
            <span class="extension-name">{{ localized(preview.manifest.label) || preview.manifest.name }} · {{ preview.manifest.version }}</span>
            <t-tooltip v-if="preview.hash" :content="'SHA-256: ' + preview.hash">
              <info-circle-icon class="extension-hash" tabindex="0" :aria-label="t('extensions.packageHash')" />
            </t-tooltip>
          </div>
          <p>{{ preview.publisher }}</p>
          <template v-if="preview.manifest.kind !== 'runtime'">
            <p>{{ t('extensions.supportedEnvironments') }}: {{ environmentLabel(preview.manifest) }}</p>
            <t-alert v-if="!previewSupported" theme="warning" :message="t('extensions.unsupportedEnvironment')" />
            <t-alert v-else-if="!acceptsUI(preview.manifest)" theme="info" :message="t('extensions.environmentManagementOnly')" />
          </template>
          <t-space size="small" break-line>
            <t-tooltip v-for="permission in preview.manifest.permissions" :key="permission" :content="permission">
              <t-tag variant="light">{{ permissionLabel(permission) }}</t-tag>
            </t-tooltip>
          </t-space>
          <t-alert v-if="previewUpdating && preview.manifest.kind === 'runtime'" theme="info" :message="t('extensions.runtimeUpdate')" />
          <t-alert v-else-if="sameVersionUpdate" theme="info" :message="t('extensions.sameVersionUpdate')" />
          <t-checkbox v-model="authorized" :disabled="!previewSupported">{{ t('extensions.authorize') }}</t-checkbox>
        </div>
      </c-drawer>
      <c-drawer v-model:visible="impactVisible" :header="t('extensions.impact')" width="560px" :footer="pendingOperation ? undefined : false"
        :confirm-btn="{ loading, content: t(pendingOperation === 'uninstall' ? 'extensions.uninstall' : 'extensions.disable') }" @confirm="confirmRemoval">
        <template v-if="impact">
          <t-alert v-if="pendingOperation === 'uninstall'" class="extension-notice" theme="warning" :message="t('extensions.uninstallHint')" />
          <t-alert v-if="pendingOperation === 'disable' && pendingID === 'lua-runtime'" class="extension-notice" theme="warning" :message="t('extensions.runtimeDisable')" />
          <p>{{ t('extensions.dependencies') }}: {{ impact.extensions.map(extensionName).join(', ') || '—' }}</p>
          <p>{{ t('menu.plugins') }}: {{ impact.plugins.join(', ') || '—' }}</p>
          <p>{{ t('menu.tasks') }}: {{ impact.tasks.join(', ') || '—' }}</p>
          <p>{{ t('extensions.pages') }}: {{ impact.pages.map(pageName).join(', ') || '—' }}</p>
          <p>{{ t('extensions.commands') }}: {{ impact.actions.map(actionName).join(', ') || '—' }}</p>
        </template>
      </c-drawer>
      <c-drawer v-model:visible="cleanupVisible" :header="t('extensions.cleanup')" width="560px"
        :confirm-btn="{ loading, content: t('extensions.cleanup'), theme: cleanupScopes.some(scope => scope !== 'cache') ? 'danger' : 'primary', disabled: !cleanupScopes.length || cleanupInstalled && cleanupScopes.includes('data') }"
        @confirm="clean">
        <p class="extension-name">{{ cleanupName }}</p>
        <t-checkbox-group v-model="cleanupScopes" class="cleanup-options">
          <div>
            <t-checkbox value="cache">{{ t('extensions.cache') }}</t-checkbox>
            <p>{{ t('extensions.cacheHint') }}</p>
          </div>
          <div>
            <t-checkbox value="data" :disabled="cleanupInstalled">{{ t('extensions.retainedData') }}</t-checkbox>
            <p>{{ t('extensions.dataHint') }}</p>
          </div>
          <div v-if="cleanupInstalled">
            <t-checkbox value="obsolete" :disabled="!obsolete?.tables.length">{{ t('extensions.obsoleteTables') }}</t-checkbox>
            <p>{{ t('extensions.obsoleteHint') }}</p>
            <p v-if="obsolete?.tables.length">{{ obsolete.tables.join(', ') }}</p>
          </div>
        </t-checkbox-group>
        <t-alert v-if="cleanupScopes.includes('data')" theme="warning" :message="t('extensions.clearDataHint')" />
        <t-alert v-else-if="cleanupScopes.includes('obsolete')" theme="warning" :message="t('extensions.clearObsoleteHint')" />
      </c-drawer>
    </template>
  </page-layout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { PageLayout, PageHeader, CCard, CDrawer, EntityIcon } from '@/components'
import { MobileFab } from '@/components/base'
import { AppIcon, ShopIcon, CloudUploadIcon, InfoCircleIcon } from 'tdesign-icons-vue-next'
import { useIsMobile } from '@/composables'
import { useAsync } from '@/composables/useAsync'
import { useDialog } from '@/composables/useDialogVisible'
import { useLocalizedText } from '@/composables/useLocale'
import { useExtensionEnvironment } from '@/composables/useExtensionEnvironment'
import { extensionApi, type ExtensionState, type ExtensionImpact, type PackageEntry } from '@/api/extensions'
import { extensionStates, refreshExtensions, runtimeManagement } from '@/features/extensions'
import { extensionCards, type ExtensionCard } from '@/features/extensionCatalog'
import { fmtBytes } from '@/utils/format'
import TrustSources from './TrustSources.vue'
import OnlinePackages from './OnlinePackages.vue'
import ExtensionSettings from './ExtensionSettings.vue'

const { t, te } = useI18n(), { run, loading } = useAsync(), { isPhone } = useIsMobile(), localized = useLocalizedText()
const { acceptsPackage, acceptsUI, environmentLabel } = useExtensionEnvironment()
const catalog = ref<PackageEntry[]>([])
const cards = computed(() => extensionCards(extensionStates.value, catalog.value).map(card => ({
  ...card, name: localized(card.label) || card.name, description: localized(card.desc),
})))
const online = ref<InstanceType<typeof OnlinePackages>>(), trust = ref<InstanceType<typeof TrustSources>>()
const settings = ref<InstanceType<typeof ExtensionSettings>>()
const input = ref<HTMLInputElement>(), preview = ref<ExtensionState>(), authorized = ref(false)
const previewSupported = computed(() => acceptsPackage(preview.value?.manifest))
const previewInstalled = computed(() => extensionStates.value.find(state => state.manifest.id === preview.value?.manifest.id))
const previewUpdating = computed(() => !!previewInstalled.value && previewInstalled.value.hash !== preview.value?.hash)
const sameVersionUpdate = computed(() => previewUpdating.value && previewInstalled.value?.manifest.version === preview.value?.manifest.version)
const previewAction = computed(() => t(previewUpdating.value ? sameVersionUpdate.value ? 'extensions.update' : 'extensions.upgrade' : 'extensions.install'))
const { visible: previewVisible } = useDialog(), { visible: impactVisible } = useDialog()
const { visible: cleanupVisible } = useDialog(), cleanupID = ref(''), cleanupScopes = ref<string[]>([])
const cleanupInstalled = computed(() => extensionStates.value.some(state => state.manifest.id === cleanupID.value))
const cleanupName = computed(() => cards.value.find(card => card.id === cleanupID.value)?.name || cleanupID.value)
const obsolete = ref<{hash:string;tables:string[]}>()
const impact = ref<ExtensionImpact>(), pendingOperation = ref<'disable' | 'uninstall'>(), pendingID = ref('')
let selected: File | undefined, selectedDigest = '', selectedTicket = ''

function permissionLabel(permission: string) {
  const key = 'extensions.permissions.' + permission.replaceAll('.', '_')
  return te(key) ? t(key) : permission
}
function extensionName(id: string) {
  const manifest = extensionStates.value.find(state => state.manifest.id === id)?.manifest
  return localized(manifest?.label) || manifest?.name || id
}
function pageName(id: string) {
  for (const { manifest } of extensionStates.value) {
    const page = manifest.pages?.find(item => `${manifest.id}/${item.id}` === id)
    if (page) return localized(page.labels) || (manifest.pages?.length === 1 ? localized(manifest.label) : '') || page.title || id
  }
  return id
}
function actionName(id: string) {
  for (const { manifest } of extensionStates.value) {
    const action = manifest.actions?.find(item => `${manifest.id}.${item.id}` === id)
    if (!action) continue
    const label = localized(action.labels)
    const key = 'mcp.tools.' + (action.target || id).replaceAll('.', '_') + '.title'
    return label || (te(key) ? t(key) : action.title) || id
  }
  return id
}
function openCleanup(id: string) {
  cleanupID.value = id; cleanupScopes.value = ['cache']; obsolete.value = undefined; cleanupVisible.value = true
  if (cleanupInstalled.value) void run(async () => { const result = await extensionApi.obsolete(id); if (cleanupID.value === id) obsolete.value = result })
}
async function clean() {
  if (!cleanupID.value || !cleanupScopes.value.length || cleanupInstalled.value && cleanupScopes.value.includes('data')) return
  await run(async () => {
    if (cleanupScopes.value.includes('cache')) await extensionApi.cleanCache(cleanupID.value)
    if (cleanupScopes.value.includes('data')) await extensionApi.cleanData(cleanupID.value)
    if (cleanupScopes.value.includes('obsolete') && obsolete.value) await extensionApi.cleanObsolete(cleanupID.value, obsolete.value.hash)
    cleanupVisible.value = false
    MessagePlugin.success(t('extensions.cleaned'))
    await reload()
  })
}
function status(card: ExtensionCard) {
  return !card.state ? 'notInstalled' : card.state.available ? 'enabled' : card.state.enabled ? 'unavailable' : 'disabled'
}
function installable(item?: PackageEntry) {
  return !!item?.manifest && acceptsPackage(item.manifest) && !!item.sha256 && ['available', 'update', 'removed', 'blocked'].includes(item.status)
}
function installLabel(item?: PackageEntry) {
  return t(item?.status === 'update' ? item.manifest?.version === item.installed_version ? 'extensions.update' : 'extensions.upgrade' : 'extensions.install')
}
async function reload() {
  const [, result] = await Promise.all([refreshExtensions(), extensionApi.catalog()])
  catalog.value = result.packages || []
  void online.value?.refresh()
}
async function inspectImpact(id: string, operation?: 'disable' | 'uninstall') {
  await run(async () => {
    impact.value = await extensionApi.impact(id)
    pendingID.value = id; pendingOperation.value = operation; impactVisible.value = true
  })
}
async function confirmRemoval() {
  await run(async () => {
    if (pendingOperation.value === 'uninstall') {
      await extensionApi.uninstall(pendingID.value)
      MessagePlugin.success(t('extensions.removed'))
    }
    else if (pendingOperation.value === 'disable') await extensionApi.enable(pendingID.value, false)
    impactVisible.value = false
    await reload()
  })
}
async function pick(event: Event) {
  const target = event.target as HTMLInputElement
  selected = target.files?.[0]; selectedDigest = ''; selectedTicket = ''; target.value = ''
  if (!selected) return
  authorized.value = false
  await run(async () => { preview.value = await extensionApi.inspect(selected!); previewVisible.value = true })
}
function pickMounted(item: PackageEntry) {
  if (!item.manifest || !item.sha256) return
  selected = undefined; selectedTicket = ''; selectedDigest = item.sha256; authorized.value = false
  preview.value = { manifest: item.manifest, hash: item.sha256, signer: item.signer || '', publisher: item.publisher || '',
    enabled: false, available: false, status: item.status, bytes: 0 }
  previewVisible.value = true
}
async function pickOnline(sha256: string) {
  selected = undefined; selectedDigest = ''; selectedTicket = ''; authorized.value = false
  await run(async () => {
    const result = await extensionApi.inspectOnline(sha256)
    selectedTicket = result.ticket; preview.value = result; previewVisible.value = true
  })
}
async function install() {
  if ((!selected && !selectedDigest && !selectedTicket) || !preview.value || !authorized.value || !previewSupported.value) return
  const success = previewUpdating.value ? 'common.updated' : 'extensions.installed'
  await run(async () => {
    const grants = preview.value!.manifest.permissions
    if (selected) await extensionApi.install(selected, grants)
    else if (selectedTicket) await extensionApi.installStaged(selectedTicket, grants)
    else await extensionApi.installMounted(selectedDigest, grants)
    previewVisible.value = false; selectedTicket = ''
    MessagePlugin.success(t(success))
    await reload()
  })
}
async function toggle(state: ExtensionState) {
  if (state.enabled) await inspectImpact(state.manifest.id, 'disable')
  else if (acceptsPackage(state.manifest)) await run(async () => { await extensionApi.enable(state.manifest.id, true); await reload() })
}
onMounted(() => run(reload))
</script>

<style scoped>
.extension-list { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(350px, 100%), 1fr)); gap: 16px; padding-bottom: 16px; min-width: 0; }
.extension-list :deep(.c-card) { min-width: 0; display: flex; flex-direction: column; }
.extension-list :deep(.t-card__body) { display: flex; flex-direction: column; flex: 1; min-width: 0; }
.extension-head { display: flex; align-items: center; gap: 12px; }
.extension-meta { min-width: 0; }
.extension-name { font-weight: 600; line-height: 1.4; overflow-wrap: anywhere; }
.extension-tags { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: 6px; font-size: 12px; color: var(--td-text-color-secondary); }
.extension-info { display: flex; flex-direction: column; gap: 6px; color: var(--td-text-color-secondary); font-size: 13px; overflow-wrap: anywhere; }
.extension-description { color: var(--td-text-color-secondary); font-size: 13px; line-height: 1.6; margin: 0 0 12px; overflow-wrap: anywhere; }
.extension-id { font-family: ui-monospace, monospace; font-size: 12px; overflow-wrap: anywhere; }
.extension-actions { margin-top: auto; padding-top: 16px; }
.extension-error { margin-top: 12px; overflow-wrap: anywhere; }
.extension-notice { margin-bottom: 16px; }
.extension-review { display: flex; flex-direction: column; gap: 16px; padding-top: 20px; }
.extension-review-title { display: flex; align-items: center; gap: 8px; }
.extension-hash { flex: none; color: var(--td-text-color-placeholder); font-size: 14px; cursor: help; }
.extension-review p { margin: 0; }
.cleanup-options { display: flex; flex-direction: column; gap: 20px; margin: 20px 0; }
.cleanup-options p { color: var(--td-text-color-secondary); line-height: 1.6; margin: 8px 0 0; }
</style>
