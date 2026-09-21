// 弹窗可见性 hook：数字 id/对象 → boolean 双向绑定（t-dialog v-model:visible 复用）。
import { computed, ref, type Ref } from 'vue'

// editVisible 双向绑定：ref !== null 即打开，关闭时置回 null
export function useDialogVisible<T>(source: Ref<T | null>) {
  return computed<boolean>({
    get: () => source.value !== null,
    set: (v) => { if (!v) source.value = null },
  })
}

// 独立开关弹窗（createVisible 等场景）
export function useDialog() {
  const visible = ref(false)
  function open() { visible.value = true }
  function close() { visible.value = false }
  return { visible, open, close }
}
