<template>
  <div class="setup-wrap">
    <div class="setup-card">
      <h2>{{ $t('setup.title') }}</h2>
      <p class="hint">{{ $t('setup.hint') }}</p>
      <t-form @submit="onSetup">
        <t-form-item :label="$t('setup.username')">
          <t-input v-model="username" :placeholder="$t('setup.usernamePh')" @enter="onSetup" />
        </t-form-item>
        <t-form-item :label="$t('setup.password')">
          <t-input v-model="password" type="password" :placeholder="$t('setup.passwordPh')" @enter="onSetup" />
        </t-form-item>
        <t-form-item :label="$t('setup.confirm')">
          <t-input v-model="confirm" type="password" @enter="onSetup" />
        </t-form-item>
        <t-button theme="primary" block :loading="loading" @click="onSetup">{{ $t('setup.submit') }}</t-button>
      </t-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { setToken } from '../../api/client'
import { authApi } from '../../api/auth'

const { t } = useI18n()
const router = useRouter()
const username = ref('')
const password = ref('')
const confirm = ref('')
const loading = ref(false)

onMounted(async () => {
  // 已初始化过则直接去登录
  const status = await authApi.setupStatus()
  if (status.initialized) router.replace('/login')
})

async function onSetup() {
  if (!username.value) {
    MessagePlugin.warning(t('setup.errUsername'))
    return
  }
  if (password.value.length < 6) {
    MessagePlugin.warning(t('setup.errPassword'))
    return
  }
  if (password.value !== confirm.value) {
    MessagePlugin.warning(t('setup.errConfirm'))
    return
  }
  loading.value = true
  try {
    await authApi.setup(username.value, password.value)
    setToken(`${username.value}:${password.value}`) // 初始化完成即登录
    MessagePlugin.success(t('setup.done'))
    router.replace('/')
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.setup-wrap {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--td-bg-color-page);
}
.setup-card {
  width: 360px;
  padding: 32px;
  background: var(--td-bg-color-container);
  border-radius: 8px;
  box-shadow: var(--td-shadow-2);
}
.setup-card h2 {
  margin: 0 0 8px;
  text-align: center;
  color: var(--td-brand-color);
}
.hint {
  text-align: center;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  margin: 0 0 24px;
}
</style>
