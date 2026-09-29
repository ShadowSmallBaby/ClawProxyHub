// 响应式断点 hook：三端统一判断（手机 <768 / 平板 768–1023 / 桌面 ≥1024）。
import { computed, onBeforeUnmount, ref } from 'vue'

const PHONE = '(max-width: 767px)'
const TABLET = '(min-width: 768px) and (max-width: 1023px)'

function watchQuery(query: string) {
  const matched = ref(false)
  if (typeof window === 'undefined') return matched
  const mql = window.matchMedia(query)
  matched.value = mql.matches
  const sync = () => (matched.value = mql.matches)
  mql.addEventListener('change', sync)
  onBeforeUnmount(() => mql.removeEventListener('change', sync))
  return matched
}

export function useBreakpoint() {
  const isPhone = watchQuery(PHONE)
  const isTablet = watchQuery(TABLET)
  // 手机 + 平板 = 非桌面（抽屉导航/弹窗全屏用）
  const isMobile = computed(() => isPhone.value || isTablet.value)
  return { isPhone, isTablet, isMobile }
}

// 兼容旧调用：返回 isMobile（含手机与平板）
export function useIsMobile() {
  return useBreakpoint()
}
