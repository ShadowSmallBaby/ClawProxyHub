// 多语言 hook：当前 locale + 切换（Layout 下拉复用）。
import { computed } from 'vue'
import i18n, { setLocale as applyLocale, type Locale } from '../i18n'

export function useLocale() {
  const locale = computed<Locale>(() => i18n.global.locale.value as Locale)

  function setLocale(l: Locale) {
    applyLocale(l)
  }

  return { locale, setLocale }
}

// 按当前语言取远端双语字段（缺当前语言回退另一语言）
export function useLocalizedText() {
  const { locale } = useLocale()
  return (m: Record<string, string> | undefined): string => {
    if (!m) return ''
    return m[locale.value] || m.zh || m.en || ''
  }
}
