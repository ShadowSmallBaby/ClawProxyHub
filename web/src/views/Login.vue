<template>
  <div class="login-wrap">
    <!-- 左侧：品牌宣发区 -->
    <div class="login-brand">
      <div class="brand-header">
        <img class="brand-logo" src="/logo.png" alt="ClawProxyHub" />
        <h1>Claw<span>ProxyHub</span></h1>
      </div>
      <p class="brand-slogan">{{ $t('login.slogan') }}</p>
      <ul class="brand-features">
        <li v-for="f in features" :key="f.title">
          <span class="feature-icon">{{ f.icon }}</span>
          <div>
            <strong>{{ f.title }}</strong>
            <p>{{ f.desc }}</p>
          </div>
        </li>
      </ul>
      <p class="brand-footer">Open Source · Self-hosted · API Compatible</p>
    </div>

    <!-- 右侧：登录区域 -->
    <div class="login-panel">
      <div class="login-card">
        <img class="login-logo" src="/logo.png" alt="ClawProxyHub" />
        <h2>{{ $t('login.welcome') }}</h2>
        <p class="login-sub">{{ $t('login.sub') }}</p>
        <t-form @submit="onLogin">
          <t-form-item :label="$t('login.username')">
            <t-input v-model="username" :placeholder="$t('login.usernamePh')" clearable @enter="onLogin" />
          </t-form-item>
          <t-form-item :label="$t('login.password')">
            <t-input
              v-model="password"
              type="password"
              :placeholder="$t('login.passwordPh')"
              @enter="onLogin"
            />
          </t-form-item>
          <t-form-item>
            <t-button theme="primary" block size="large" :loading="loading" @click="onLogin">
              {{ $t('login.submit') }}
            </t-button>
          </t-form-item>
        </t-form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api, setToken } from '../api/client'

const { t } = useI18n()
const router = useRouter()
const username = ref('')
const password = ref('')
const loading = ref(false)

const features = computed(() => [
  { icon: '🔀', title: t('login.featRouting'), desc: t('login.featRoutingDesc') },
  { icon: '🔑', title: t('login.featKeys'), desc: t('login.featKeysDesc') },
  { icon: '📊', title: t('login.featMonitor'), desc: t('login.featMonitorDesc') },
  { icon: '🧩', title: t('login.featPlugins'), desc: t('login.featPluginsDesc') },
])

onMounted(async () => {
  // 未初始化 → 引导页
  try {
    const status = await api.get<{ initialized: boolean }>('/admin/setup-status')
    if (!status.initialized) router.replace('/setup')
  } catch { /* 探测失败不阻塞 */ }
})

async function onLogin() {
  if (!username.value || !password.value) return
  loading.value = true
  try {
    // 登录换 JWT（后续请求带 Bearer <jwt>，服务端解析 role 免每请求 bcrypt）
    const r = await api.post<{ token: string; role: string }>('/admin/login', {
      username: username.value,
      password: password.value,
    })
    setToken(r.token)
    router.push('/')
  } catch (e: any) {
    MessagePlugin.error(e.message === 'unauthorized' ? t('login.errCredentials') : e.message)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-wrap {
  height: 100%;
  display: flex;
}

/* ---------- 左侧品牌区 ---------- */
.login-brand {
  flex: 1 1 55%;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 48px 8%;
  color: #fff;
  /* 深蓝渐变 + 品牌色光斑，明暗主题通用 */
  background:
    radial-gradient(ellipse 60% 40% at 15% 20%, rgba(127, 167, 255, 0.25), transparent),
    radial-gradient(ellipse 50% 45% at 85% 80%, rgba(76, 125, 255, 0.35), transparent),
    linear-gradient(160deg, #162d75 0%, #0f1f52 55%, #0a1440 100%);
}

.brand-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 20px;
}

.brand-logo {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.12);
  padding: 4px;
}

.brand-header h1 {
  margin: 0;
  font-size: 28px;
  letter-spacing: 0.5px;
}

.brand-header h1 span {
  font-weight: 300;
  opacity: 0.8;
}

.brand-slogan {
  margin: 0 0 40px;
  font-size: 16px;
  opacity: 0.85;
  line-height: 1.6;
}

.brand-features {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 22px;
  max-width: 460px;
}

.brand-features li {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}

.feature-icon {
  flex: none;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(4px);
}

.brand-features strong {
  display: block;
  font-size: 15px;
  margin-bottom: 2px;
}

.brand-features p {
  margin: 0;
  font-size: 13px;
  opacity: 0.7;
  line-height: 1.5;
}

.brand-footer {
  margin: 48px 0 0;
  font-size: 12px;
  letter-spacing: 1px;
  text-transform: uppercase;
  opacity: 0.45;
}

/* ---------- 右侧登录区 ---------- */
.login-panel {
  flex: 1 1 45%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--td-bg-color-page);
}

.login-card {
  width: 100%;
  max-width: 380px;
  padding: 40px 36px;
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: var(--td-shadow-2);
}

.login-logo {
  display: block;
  width: 56px;
  height: 56px;
  margin: 0 auto 16px;
  border-radius: 14px;
}

.login-card h2 {
  margin: 0 0 4px;
  text-align: center;
  color: var(--td-text-color-primary);
}

.login-sub {
  margin: 0 0 28px;
  text-align: center;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

/* ---------- 窄屏：隐藏品牌区 ---------- */
@media (max-width: 768px) {
  .login-brand {
    display: none;
  }
  .login-panel {
    flex: 1 1 100%;
  }
}
</style>
