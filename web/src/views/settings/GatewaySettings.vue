<template>
  <div class="panel">
    <div class="s-form">
      <form-item :label="$t('settings.firstEventTimeout')" :tip="$t('settings.firstEventTimeoutHelp')">
        <t-input-number v-model="gwForm.first_event_timeout" :min="5" :max="3600" theme="column" class="w-sm" />
      </form-item>
      <form-item :label="$t('settings.firstTokenTimeout')" :tip="$t('settings.firstTokenTimeoutHelp')">
        <t-input-number v-model="gwForm.first_token_timeout" :min="5" :max="3600" theme="column" class="w-sm" />
      </form-item>
      <form-item :label="$t('settings.maxRetries')" :tip="$t('settings.maxRetriesHelp')">
        <t-input-number v-model="gwForm.max_retries" :min="1" :max="10" theme="column" class="w-sm" />
      </form-item>
      <form-item :label="$t('settings.contextTruncate')" :tip="$t('settings.contextTruncateHelp')">
        <t-switch v-model="gwForm.context_truncate_enabled" />
      </form-item>
      <form-item :label="$t('settings.contextTruncateRatio')" :tip="$t('settings.contextTruncateRatioHelp')">
        <t-input-number v-model="gwForm.context_truncate_ratio" :min="0.1" :max="1" :step="0.05" :decimal-places="2" theme="column" class="w-sm" />
      </form-item>
      <form-item :label="$t('settings.contextBytesPerToken')" :tip="$t('settings.contextBytesPerTokenHelp')">
        <t-input-number v-model="gwForm.context_bytes_per_token" :min="1" :max="100" :step="0.5" :decimal-places="1" theme="column" class="w-sm" />
      </form-item>
      <form-item :label="$t('settings.userAgent')" :tip="$t('settings.userAgentHelp')">
        <t-input v-model="gwForm.user_agent" :placeholder="$t('settings.uaPh')" class="w-2xl" />
      </form-item>
      <form-item :label="$t('settings.browserUserAgent')" :tip="$t('settings.browserUserAgentHelp')">
        <t-input v-model="gwForm.browser_user_agent" :placeholder="$t('settings.uaPh')" class="w-2xl" />
      </form-item>
      <div class="save-row">
        <t-button theme="primary" :loading="saving" @click="save">{{ $t('common.save') }}</t-button>
      </div>
    </div>
  </div>

</template>
<script setup lang="ts">
import { reactive, watch } from 'vue'
import { FormItem } from '@/components'
import type { AdminSettings } from '@/api/settings'
const props = defineProps<{ settings?: AdminSettings; saving: boolean; save: (patch: Record<string, unknown>) => Promise<void> }>()
const gwForm = reactive({ first_event_timeout: 60, first_token_timeout: 120, max_retries: 3, user_agent: '', browser_user_agent: '', context_truncate_enabled: true, context_truncate_ratio: 0.9, context_bytes_per_token: 3.5 })
watch(() => props.settings, settings => {
  const r = { settings }
  gwForm.first_token_timeout = r.settings?.first_token_timeout ?? 120
  gwForm.first_event_timeout = r.settings?.first_event_timeout ?? 60
  gwForm.max_retries = r.settings?.max_retries ?? 3
  gwForm.user_agent = r.settings?.user_agent ?? ''
  gwForm.browser_user_agent = r.settings?.browser_user_agent ?? ''
  gwForm.context_truncate_enabled = r.settings?.context_truncate_enabled ?? true
  gwForm.context_truncate_ratio = r.settings?.context_truncate_ratio ?? 0.9
  gwForm.context_bytes_per_token = r.settings?.context_bytes_per_token ?? 3.5
}, { immediate: true })
function save() {
  return props.save({ first_event_timeout: gwForm.first_event_timeout, first_token_timeout: gwForm.first_token_timeout, max_retries: gwForm.max_retries, user_agent: gwForm.user_agent.trim(), browser_user_agent: gwForm.browser_user_agent.trim(), context_truncate_enabled: gwForm.context_truncate_enabled, context_truncate_ratio: gwForm.context_truncate_ratio, context_bytes_per_token: gwForm.context_bytes_per_token })
}
defineExpose({ save })
</script>
<style scoped>
.panel { padding: 24px 28px 32px; }
.save-row { padding-top: 16px; }
@media (max-width: 767px) { .panel { padding: 16px 12px 24px; } .save-row { display: none; } }
</style>
