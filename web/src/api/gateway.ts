// 网关专属账号操作随对应前端功能编入。
import { api } from './client'
import type { ModelInfo } from './types'
export const modelsApi = {
  list: (id: number, refresh = false) => api.get<{ models: ModelInfo[] | null }>(`/admin/accounts/${id}/models${refresh ? '?refresh=1' : ''}`),
  save: (id: number, models: { id: string }[]) => api.put(`/admin/accounts/${id}/models`, { models }),
}
export const accountTestApi = {
  test: (id: number, body: { endpoint: string; model: string; question: string }) => api.post<{ text: string; logs: string[]; request?: string; events?: string[] }>(`/admin/accounts/${id}/test`, body),
}
