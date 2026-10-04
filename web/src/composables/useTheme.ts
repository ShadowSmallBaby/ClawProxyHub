// 主题状态在布局与切换按钮间共享。
import { ref, watch } from 'vue'

const KEY = 'cph-theme'
const dark = ref(localStorage.getItem(KEY) === 'dark')

watch(dark, (v) => {
  const mode = v ? 'dark' : 'light'
  document.documentElement.setAttribute('theme-mode', mode)
  localStorage.setItem(KEY, mode)
})

export function useTheme() {
  document.documentElement.setAttribute('theme-mode', dark.value ? 'dark' : 'light')
  return { dark }
}
