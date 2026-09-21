// 分页 hook：页码 / 页大小 / 总数（Logs/Tasks 复用）。
import { ref } from 'vue'

export function usePagination(defaultSize = 30) {
  const page = ref(1)
  const pageSize = ref(defaultSize)
  const total = ref(0)

  // search 重置到第一页再查
  function reset() {
    page.value = 1
  }

  return { page, pageSize, total, reset }
}
