// 枚举值 → 双语文案字典；dict()/label() 均跟随当前语言（i18n locale 响应式）。
import i18n from '../i18n'

// 双语文案：zh / en
type Bi = { zh: string; en: string }

// 按当前语言取值：en 取 en（缺省回退 zh），zh 取 zh
function pick(v: Bi): string {
  return (i18n.global.locale.value === 'en' ? v.en || v.zh : v.zh) || ''
}

export const capabilityDict: Record<string, Bi> = {
  account: { zh: '账号', en: 'Account' },
  login: { zh: '登录', en: 'Login' },
  chat: { zh: '对话', en: 'Chat' },
  models: { zh: '模型', en: 'Models' },
  tasks: { zh: '任务', en: 'Tasks' },
  refresh: { zh: '刷新', en: 'Refresh' },
  refreshable: { zh: '可刷新', en: 'Refreshable' },
  auto_relogin: { zh: '自动续登', en: 'Auto Re-login' },
  profile: { zh: '资料', en: 'Profile' },
  instances: { zh: '多实例', en: 'Multi-instance' },
}

export const strategyDict: Record<string, Bi> = {
  round_robin: { zh: '轮询', en: 'Round Robin' },
  random: { zh: '随机', en: 'Random' },
  least_used: { zh: '最少使用', en: 'Least Used' },
  sticky: { zh: '会话粘性', en: 'Sticky Session' },
}

export const accountStatusDict: Record<string, Bi> = {
  active: { zh: '正常', en: 'Active' },
  expired: { zh: '已过期', en: 'Expired' },
  disabled: { zh: '已停用', en: 'Disabled' },
  paused: { zh: '已暂停', en: 'Paused' },
}

export const triggerDict: Record<string, Bi> = {
  interval: { zh: '固定间隔', en: 'Interval' },
  cron: { zh: 'Cron 表达式', en: 'Cron Expression' },
  daily: { zh: '每天定时', en: 'Daily' },
  once: { zh: '单次', en: 'Once' },
}

export const runStatusDict: Record<string, Bi> = {
  running: { zh: '执行中', en: 'Running' },
  success: { zh: '成功', en: 'Success' },
  failed: { zh: '失败', en: 'Failed' },
}

export const protocolDict: Record<string, Bi> = {
  messages: { zh: 'Anthropic Messages', en: 'Anthropic Messages' },
  chat_completions: { zh: 'OpenAI Chat', en: 'OpenAI Chat' },
  responses: { zh: 'OpenAI Responses', en: 'OpenAI Responses' },
}

// dict 查字典，未收录原样返回枚举值
export function dict(map: Record<string, Bi>, key: string | null | undefined): string {
  if (!key) return ''
  const v = map[key]
  return v ? pick(v) : key
}

// label 多语言展示名：当前语言优先，回退另一语言，再回退 fallback
export function label(map: Record<string, string> | null | undefined, fallback: string): string {
  if (!map) return fallback
  const l = i18n.global.locale.value === 'en' ? 'en' : 'zh'
  return map[l] || map.zh || map.en || fallback
}
