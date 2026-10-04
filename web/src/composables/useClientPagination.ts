// 全量列表的展示分页：先筛选再切页，删除末页数据后自动回到有效页。
import { computed, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { usePagination } from './usePagination'

export function useClientPagination<T>(source: MaybeRefOrGetter<readonly T[]>, defaultSize = 30) {
  const { page, pageSize, reset } = usePagination(defaultSize)
  const total = computed(() => toValue(source).length)
  const items = computed(() => toValue(source).slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

  watch(pageSize, reset, { flush: 'sync' })
  watch(total, (n) => {
    page.value = Math.min(page.value, Math.max(1, Math.ceil(n / pageSize.value)))
  }, { flush: 'sync' })

  return { page, pageSize, total, items, reset }
}
