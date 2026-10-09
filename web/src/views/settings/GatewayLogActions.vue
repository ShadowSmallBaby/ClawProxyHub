<template>
  <form-item :label="$t('settings.logExport')" :tip="$t('settings.logExportHelp')">
    <t-button variant="outline" :loading="exporting" @click="exportLogs">
      <template #icon><download-icon /></template>{{ $t('settings.exportBtn') }}
    </t-button>
  </form-item>
  <form-item :label="$t('settings.logClear')" :tip="$t('settings.logClearHelp')">
    <t-button theme="danger" variant="outline" @click="confirmClear">
      <template #icon><delete-icon /></template>{{ $t('settings.clearBtn') }}
    </t-button>
  </form-item>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { DownloadIcon, DeleteIcon } from 'tdesign-icons-vue-next'
import { FormItem } from '@/components'
import { logsApi } from '@/api/logs'
const { t } = useI18n()
const emit = defineEmits<{ changed: [] }>()
const exporting = ref(false)
const exportLogs = () => logsApi.exportCsv(exporting).catch((e: any) => MessagePlugin.error(e.message))
function confirmClear() {
  const dlg = DialogPlugin.confirm({
    header: t('settings.clearConfirmTitle'), body: t('settings.clearConfirmBody'), theme: 'danger',
    onConfirm: async () => {
      try {
        const r = await logsApi.clear()
        MessagePlugin.success(t('settings.cleared', { n: r.deleted }))
        emit('changed')
      } catch (e: any) {
        MessagePlugin.error(e.message)
      }
      dlg.destroy()
    },
  })
}

</script>
