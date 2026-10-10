// 站点品牌（名称 / 缩写 / logo）：全局单例，登录页与布局共用；设置页保存后调 refreshBranding 即时刷新。
import { computed, reactive } from 'vue'
import { resourceURL } from '@/api/client'
import { onConnectionReset } from '@/api/connections'
import { brandingApi, type Branding } from '@/api/auth'

export const DEFAULT_SITE_NAME = 'ClawProxyHub'
export const DEFAULT_LOGO = `${import.meta.env.BASE_URL}logo.png`



export const branding = reactive<Branding>({ name: DEFAULT_SITE_NAME, abbr: 'CPH', logo: '' })

/** logo 地址：自定义 data URL 优先，否则内置 */
export const brandLogo = computed(() => branding.logo ? resourceURL(branding.logo) : DEFAULT_LOGO)
/** 是否为自定义品牌名（自定义走蓝紫渐变字，默认走 Claw / ProxyHub 双段样式） */
export const brandCustom = computed(() => branding.name !== DEFAULT_SITE_NAME)

let loaded = false
export async function refreshBranding() {
  try {
    Object.assign(branding, await brandingApi.get())
    document.title = branding.name
    loaded = true
  } catch { /* 失败保持默认 */ }
}
export function ensureBranding() {
  if (!loaded) refreshBranding()
}

onConnectionReset(() => {
  loaded = false
  Object.assign(branding, { name: DEFAULT_SITE_NAME, abbr: 'CPH', logo: '' })
  document.title = DEFAULT_SITE_NAME
})
