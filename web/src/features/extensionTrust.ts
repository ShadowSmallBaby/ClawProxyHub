import type { ExtensionTrustConfig } from '../api/extensions'

const identifier = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/
const validID = (value: unknown): value is string => typeof value === 'string' && value.length <= 96 && identifier.test(value)
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)
const fail = (key: string): never => { throw new Error(`extensions.trustValidation.${key}`) }

function base64(value: unknown) {
  if (typeof value !== 'string' || !value || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return ''
  try { return atob(value) } catch { return '' }
}

function identity(value: unknown) {
  if (!object(value)) return fail('identity')
  if ('key_id' in value && !object(value.key_id)) {
    const { key_id: code, ...config } = value
    return { code, config }
  }
  if (Object.keys(value).length !== 1) return fail('identity')
  const code = Object.keys(value)[0]!
  return { code, config: value[code] }
}

export function extractTrustCode(text: string): string {
  try {
    const { code } = identity(JSON.parse(text))
    return typeof code === 'string' ? code : ''
  } catch { return '' }
}

// 签名 ID 使用 key_id，同时兼容 trust.json 的单身份映射；证书有效性由服务端最终校验。
export function parseTrustJSON(text: string): { code: string; config: ExtensionTrustConfig } {
  let value: unknown
  try { value = JSON.parse(text) } catch { return fail('json') }
  const { code, config } = identity(value)
  if (!validID(code)) return fail('code')
  if (!object(config)) return fail('object')
  const fields = ['public_key', 'certificate', 'publisher', 'ids', 'permissions', 'native']
  if (Object.keys(config).some(key => !fields.includes(key))) return fail('fields')
  if (typeof config.publisher !== 'string' || !config.publisher.trim() || config.publisher.length > 200) return fail('publisher')
  if ((!!config.public_key) === (!!config.certificate)) return fail('key')
  if ('public_key' in config && (typeof config.public_key !== 'string' || (config.public_key && base64(config.public_key).length !== 32))) return fail('publicKey')
  if ('certificate' in config && (typeof config.certificate !== 'string' || (config.certificate && !base64(config.certificate)))) return fail('certificate')
  if (!Array.isArray(config.ids) || config.ids.length < 1 || config.ids.length > 256 ||
      config.ids.some(id => !validID(id)) || new Set(config.ids).size !== config.ids.length) return fail('ids')
  if (!Array.isArray(config.permissions) || config.permissions.length > 64 ||
      config.permissions.some(permission => !validID(permission)) || new Set(config.permissions).size !== config.permissions.length) return fail('permissions')
  if ('native' in config && typeof config.native !== 'boolean') return fail('native')
  return { code, config: config as unknown as ExtensionTrustConfig }
}
