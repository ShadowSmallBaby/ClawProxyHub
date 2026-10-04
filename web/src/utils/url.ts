// 与后台实例地址规则一致：允许内网和基础路径，凭据单独配置。
export function validBaseURL(raw: string): boolean {
  try {
    const u = new URL(raw)
    if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password || u.search || u.hash) return false
    const authority = raw.replace(/^https?:\/\//, '').split(/[/?#]/)[0]
    if (authority.endsWith(':')) return false
    if (u.port && (+u.port < 1 || +u.port > 65535)) return false
    if (u.hostname.startsWith('[')) return true
    const host = authority.split(':')[0]
    if (/^[\d.]+$/.test(host)) return /^(\d+\.){3}\d+$/.test(host) && host.split('.').every(p => /^(0|[1-9]\d{0,2})$/.test(p) && +p <= 255)
    return host.length <= 253 && host.replace(/\.$/, '').split('.').every(p => /^[a-z\d](?:[a-z\d-]{0,61}[a-z\d])?$/i.test(p))
  } catch { return false }
}
