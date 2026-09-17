<template>
  <div class="page">
    <div class="page-header">
      
    </div>

    <t-card :title="$t('settings.gateway')" class="card" :bordered="false">
      <t-form label-width="140px">
        <t-form-item :label="$t('settings.firstTokenTimeout')" :help="$t('settings.firstTokenTimeoutHelp')">
          <t-input-number v-model="gwForm.first_event_timeout" :min="5" :max="3600" theme="column" style="width: 160px" />
        </t-form-item>
        <t-form-item>
          <t-button theme="primary" :loading="savingGw" @click="saveGw">{{ $t('common.save') }}</t-button>
        </t-form-item>
      </t-form>
    </t-card>

    <t-card :title="$t('settings.network')" class="card" :bordered="false">
      <t-form label-width="140px">
        <t-form-item :label="$t('settings.githubProxy')" :help="$t('settings.githubProxyHelp')">
          <t-input v-model="netForm.github_proxy" placeholder="https://ghproxy.com" style="width: 360px" />
        </t-form-item>
        <t-form-item :label="$t('settings.marketplaceUrl')" :help="$t('settings.marketplaceHelp')">
          <t-input v-model="netForm.marketplace_url" :placeholder="$t('settings.marketplacePh')" style="width: 360px" />
        </t-form-item>
        <t-form-item>
          <t-button theme="primary" :loading="savingNet" @click="saveNet">{{ $t('common.save') }}</t-button>
        </t-form-item>
      </t-form>
    </t-card>

    <t-card :title="$t('settings.adminPassword')" class="card" :bordered="false">
      <t-form label-width="140px">
        <t-form-item :label="$t('settings.newPassword')" mark>
          <t-input v-model="pwForm.password" type="password" :placeholder="$t('settings.passwordPh')" style="width: 280px" />
        </t-form-item>
        <t-form-item :label="$t('settings.confirmPassword')" mark>
          <t-input v-model="pwForm.confirm" type="password" style="width: 280px" />
        </t-form-item>
        <t-form-item>
          <t-button theme="primary" :loading="savingPw" @click="savePw">{{ $t('settings.changePassword') }}</t-button>
        </t-form-item>
      </t-form>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api } from '../api/client'

const { t } = useI18n()

const gwForm = reactive({ first_event_timeout: 90 })
const netForm = reactive({ github_proxy: '', marketplace_url: '' })
const pwForm = reactive({ password: '', confirm: '' })
const savingGw = ref(false)
const savingNet = ref(false)
const savingPw = ref(false)

async function load() {
  const r = await api.get<{ settings: { first_event_timeout: number; github_proxy?: string; marketplace_url?: string } }>('/admin/settings')
  gwForm.first_event_timeout = r.settings?.first_event_timeout ?? 90
  netForm.github_proxy = r.settings?.github_proxy ?? ''
  netForm.marketplace_url = r.settings?.marketplace_url ?? ''
}

async function saveGw() {
  savingGw.value = true
  try {
    await api.put('/admin/settings', {
      first_event_timeout: gwForm.first_event_timeout,
      github_proxy: netForm.github_proxy.trim(),
      marketplace_url: netForm.marketplace_url.trim(),
    })
    MessagePlugin.success(t('settings.saved'))
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    savingGw.value = false
  }
}

async function saveNet() {
  savingNet.value = true
  try {
    await api.put('/admin/settings', {
      first_event_timeout: gwForm.first_event_timeout,
      github_proxy: netForm.github_proxy.trim(),
      marketplace_url: netForm.marketplace_url.trim(),
    })
    MessagePlugin.success(t('settings.saved'))
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    savingNet.value = false
  }
}

async function savePw() {
  if (pwForm.password.length < 6) {
    MessagePlugin.warning(t('settings.errPassword'))
    return
  }
  if (pwForm.password !== pwForm.confirm) {
    MessagePlugin.warning(t('settings.errConfirm'))
    return
  }
  savingPw.value = true
  try {
    await api.post('/admin/password', { password: pwForm.password })
    MessagePlugin.success(t('settings.passwordChanged'))
    pwForm.password = ''
    pwForm.confirm = ''
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    savingPw.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.card { max-width: 720px; margin-bottom: 16px }
</style>
