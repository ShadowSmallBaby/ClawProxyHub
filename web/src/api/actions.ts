// GUI、CLI 与 MCP 复用核心的动作权限和审计。
import { request } from './client'
export const actionApi = {
  invoke: <T = unknown>(id: string, input: unknown = {}, signal?: AbortSignal) =>
    request<T>('POST', `/admin/actions/${encodeURIComponent(id)}`, input, signal),
}
