// i18n 实例：全局作用域（legacy: false，组合式 API），locale 持久化 localStorage
import { createI18n } from 'vue-i18n'
import zh from './zh'
import en from './en'

export type Locale = 'zh' | 'en'

export function initialLocale(): Locale {
  return (localStorage.getItem('cph-locale') as Locale) || 'zh'
}

const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: initialLocale(),
  fallbackLocale: 'zh',
  messages: { zh, en },
})

// 切换语言（持久化 + 同步 <html lang>）
export function setLocale(locale: Locale) {
  i18n.global.locale.value = locale
  localStorage.setItem('cph-locale', locale)
  document.documentElement.setAttribute('lang', locale)
}

export default i18n
