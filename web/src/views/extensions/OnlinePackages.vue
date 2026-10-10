<template>
  <c-drawer v-model:visible="visible" :header="t('extensions.market')" :footer="false" width="420px">
    <t-loading :loading="loading" class="market-content">
      <t-empty v-if="!loading && !catalog?.packages.length" :description="t(unavailable ? 'extensions.onlineUnavailable' : 'extensions.onlineEmpty')" />
      <div class="market-list">
        <c-card v-for="item in catalog?.packages || []" :key="item.sha256" class="market-card">
          <div class="market-head">
            <entity-icon :name="localized(item.label) || item.display_name || item.id" size="40px" />
            <div class="market-meta">
              <div class="market-name">{{ localized(item.label) || item.display_name || item.id }}</div>
              <div class="market-sub">{{ item.id }}</div>
            </div>
            <t-tag size="small" variant="light">v{{ item.version }}</t-tag>
          </div>
          <p v-if="localized(item.desc)" class="market-description">{{ localized(item.desc) }}</p>
          <template v-if="item.kind !== 'runtime'">
            <p class="market-sub">{{ t('extensions.supportedEnvironments') }}: {{ environmentLabel(item) }}</p>
            <t-tag v-if="!acceptsUI(item)" size="small" variant="light">{{ t('extensions.unsupportedEnvironment') }}</t-tag>
          </template>
          <div class="market-sub" v-if="item.installed_version">{{ t('extensions.installedVersion') }}: {{ item.installed_version }}</div>
          <div class="market-foot">
            <span class="market-sub">{{ fmtBytes(item.size) }}</span>
            <t-link v-if="acceptsPackage(item) && ['available', 'update'].includes(item.status)" theme="primary" :disabled="busy || loading" @click="emit('select', item.sha256)">
              {{ t(item.status === 'update' ? item.version === item.installed_version ? 'extensions.update' : 'extensions.upgrade' : 'extensions.install') }}
            </t-link>
            <t-tag v-else-if="acceptsPackage(item)" size="small" variant="light" :theme="item.status === 'installed' ? 'success' : 'default'">{{ t('extensions.packageStatus.' + item.status) }}</t-tag>
          </div>
        </c-card>
      </div>
    </t-loading>
  </c-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CCard, CDrawer, EntityIcon } from '@/components'
import { useAsync } from '@/composables/useAsync'
import { useDialog } from '@/composables/useDialogVisible'
import { useLocalizedText } from '@/composables/useLocale'
import { useExtensionEnvironment } from '@/composables/useExtensionEnvironment'
import { extensionApi, type OnlineCatalog } from '@/api/extensions'
import { fmtBytes } from '@/utils/format'

defineProps<{ busy: boolean }>()
const emit = defineEmits<{ select: [sha256: string] }>()
const { t } = useI18n(), { loading, run } = useAsync(), { visible } = useDialog()
const localized = useLocalizedText()
const { acceptsPackage, acceptsUI, environmentLabel } = useExtensionEnvironment()
const catalog = ref<OnlineCatalog>(), unavailable = ref(false)
let request = 0
async function reload(refresh = false) {
  const current = ++request
  await run(async () => {
    try {
      const result = await extensionApi.online(refresh)
      if (current === request) { catalog.value = result; unavailable.value = false }
    } catch {
      if (current === request) unavailable.value = true
    }
  }, { silent: true })
}
function open() { visible.value = true; void reload(true) }
function refresh() { if (visible.value) return reload() }
defineExpose({ open, refresh })
</script>

<style scoped>
.market-content { width: 100%; padding-top: 24px; box-sizing: border-box; }
.market-list { display: flex; flex-direction: column; gap: 12px; }
.market-head { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.market-meta { flex: 1; min-width: 0; }
.market-name { font-weight: 600; line-height: 1.4; overflow-wrap: anywhere; }
.market-sub { font-size: 12px; color: var(--td-text-color-secondary); overflow-wrap: anywhere; }
.market-description { color: var(--td-text-color-secondary); font-size: 13px; line-height: 1.6; margin: 12px 0; overflow-wrap: anywhere; }
.market-foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 12px; }
</style>
