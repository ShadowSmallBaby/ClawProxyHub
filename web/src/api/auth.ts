// 认证与账号体系 API（登录 / 当前用户 / 密码 / 通知 / 版本）。
import { api } from './client'

// ---------- 认证 ----------

export interface LoginTokenResp { token: string; role: string }
export interface SetupStatus { initialized: boolean }

export interface Me { username: string; role: string; menus: string[]; created_at?: string }

export const authApi = {
  login: (username: string, password: string) =>
    api.post<LoginTokenResp>('/admin/login', { username, password }),
  setupStatus: () => api.get<SetupStatus>('/admin/setup-status'),
  setup: (username: string, password: string) =>
    api.post('/admin/setup', { username, password }),
  me: () => api.get<Me>('/admin/me'),
  changePassword: (old_password: string, password: string) =>
    api.post('/admin/password', { old_password, password }),
}

// ---------- 版本 ----------

export interface Changelog { title?: Record<string, string>; items?: Record<string, string>[] }
export interface VersionInfo {
  version: string
  latest?: string
  update_available?: boolean
  release_url?: string
  changelog?: Changelog
}

export const versionApi = {
  get: () => api.get<VersionInfo>('/admin/version'),
}

// ---------- 站内通知 ----------

export interface Notification {
  id: number
  title: string
  content: string
  level: string
  read: boolean
  created_at: string
}

export const notificationApi = {
  list: () => api.get<{ notifications: Notification[]; unread: number }>('/admin/notifications?limit=50'),
  read: (id: number) => api.post(`/admin/notifications/${id}/read`),
  readAll: () => api.post('/admin/notifications/read-all'),
  clear: () => api.del('/admin/notifications'),
}
