// 响应式断点 hook：两态判断（手机 <768 / PC 含平板 >=768）。
import { computed, onBeforeUnmount, ref } from 'vue'

// 仅两态：手机 <768，PC（含平板）>=768
const PHONE = '(max-width: 767px)'

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

export function useIsMobile() {
  const isPhone = watchQuery(PHONE)
  // PC（含平板）= 非手机
  const isMobile = computed(() => isPhone.value)
  return { isPhone, isMobile }
}
