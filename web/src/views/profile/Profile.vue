<template>
  <div class="page">
    <c-card :title="$t('profile.title')" class="card" :bordered="false">
      <t-descriptions :column="1" bordered size="small">
        <t-descriptions-item :label="$t('profile.username')">{{ me?.username || '-' }}</t-descriptions-item>
        <t-descriptions-item :label="$t('profile.role')">{{ roleLabel }}</t-descriptions-item>
        <t-descriptions-item :label="$t('profile.createdAt')">{{ fmtTime(me?.created_at) }}</t-descriptions-item>
        <t-descriptions-item :label="$t('profile.menus')">
          <t-space size="small" break-line>
            <t-tag v-for="m in me?.menus ?? []" :key="m" size="small" variant="light">{{ $t('menu.' + m) }}</t-tag>
          </t-space>
        </t-descriptions-item>
      </t-descriptions>
      <div style="margin-top: 16px">
        <t-button theme="primary" variant="outline" @click="openPw">{{ $t('settings.changePassword') }}</t-button>
      </div>
    </c-card>

    <!-- 修改密码：弹窗确认 -->
    <c-dialog v-model:visible="pwVisible" :header="$t('settings.changePassword')" :confirm-btn="{ loading: savingPw }" @confirm="savePw">
      <t-form label-width="110px">
        <t-form-item :label="$t('settings.oldPassword')" mark>
          <t-input v-model="pwForm.old" type="password" />
        </t-form-item>
        <t-form-item :label="$t('settings.newPassword')" mark>
          <t-input v-model="pwForm.password" type="password" :placeholder="$t('settings.passwordPh')" />
        </t-form-item>
        <t-form-item :label="$t('settings.confirmPassword')" mark>
          <t-input v-model="pwForm.confirm" type="password" />
        </t-form-item>
      </t-form>
    </c-dialog>
  </div>
</template>

<script setup lang="ts">
import { CDialog } from '../../components/base'
import { CCard, CTable } from '../../components/base'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { clearToken } from '../../api/client'
import { authApi, type Me } from '../../api/auth'

const { t } = useI18n()
const router = useRouter()

const me = ref<Me | null>(null)
const roleLabel = computed(() => t(me.value?.role === 'guest' ? 'common.guest' : 'common.admin'))

const pwVisible = ref(false)
const pwForm = reactive({ old: '', password: '', confirm: '' })
const savingPw = ref(false)

function fmtTime(s?: string): string {
  return s ? s.replace('T', ' ').slice(0, 19) : '-'
}

function openPw() {
  pwForm.old = ''
  pwForm.password = ''
  pwForm.confirm = ''
  pwVisible.value = true
}

async function savePw() {
  if (!pwForm.old) {
    MessagePlugin.warning(t('settings.errOldPassword'))
    return
  }
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
    await authApi.changePassword(pwForm.old, pwForm.password)
    MessagePlugin.success(t('settings.passwordChanged'))
    pwVisible.value = false
    // 密码已变：清会话强制重新登录
    clearToken()
    router.push('/login')
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    savingPw.value = false
  }
}

onMounted(async () => {
  me.value = await authApi.me()
})
</script>

<style scoped>
.card { max-width: 720px; }
</style>
