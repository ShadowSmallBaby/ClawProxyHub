import { api, resourceURL } from './client'

export interface MCPTool { id: string; owner?: string; title: string; permission: string; effect: 'read' | 'write' | 'execute' }
export interface MCPState { enabled: boolean; has_key: boolean; allowed_actions: string[]; updated_at: string }
export interface MCPConfig { config: MCPState; tools: MCPTool[]; endpoint: string; execution: string }
export const mcpApi = {
  config: () => api.get<MCPConfig>('/admin/mcp/config'),
  save: (enabled: boolean, allowed_actions: string[]) => api.put('/admin/mcp/config', { enabled, allowed_actions }),
  rotate: () => api.post<{ key: string }>('/admin/mcp/key', {}),
  revoke: () => api.del('/admin/mcp/key'),
  address: (path: string) => new URL(resourceURL(path), window.location.origin).href,
}
