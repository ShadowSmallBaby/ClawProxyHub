// 主题 hook：明暗切换 + 持久化（AppLayout 及登录页复用）。
import { ref, watch } from 'vue'

const KEY = 'cph-theme'

export function useTheme() {
  const dark = ref(localStorage.getItem(KEY) === 'dark')

  watch(dark, (v) => {
    const mode = v ? 'dark' : 'light'
    document.documentElement.setAttribute('theme-mode', mode)
    localStorage.setItem(KEY, mode)
  })

  // 初始主题（首次调用即应用）
  document.documentElement.setAttribute('theme-mode', dark.value ? 'dark' : 'light')

  return { dark }
}
