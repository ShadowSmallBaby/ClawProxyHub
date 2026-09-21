// 异步操作 hook：加载态 + 错误消息统一提示（列表加载/提交/删除复用）。
import { ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

export function useAsync() {
  const loading = ref(false)

  // run 包装异步任务：自动管理 loading；失败弹错误消息（可静默）
  async function run<T>(task: () => Promise<T>, opts?: { silent?: boolean; errorText?: string }): Promise<T | undefined> {
    loading.value = true
    try {
      return await task()
    } catch (e: any) {
      if (!opts?.silent) MessagePlugin.error(e?.message || opts?.errorText || 'error')
      return undefined
    } finally {
      loading.value = false
    }
  }

  return { loading, run }
}
