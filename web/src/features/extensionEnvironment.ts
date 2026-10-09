import type { ExtensionEnvironment } from '../../../sdk/extension/ui'

export interface EnvironmentManifest { environments?: readonly ExtensionEnvironment[] }

export function extensionEnvironment(profile: string, mobile: boolean): ExtensionEnvironment {
  return profile.startsWith('app-') ? 'app' : mobile ? 'web-mobile' : 'web-desktop'
}

// App 与 Web 分开安装；Web 的屏幕适配范围只影响前端入口。
export function supportsExtensionInstallation(manifest: EnvironmentManifest | undefined, environment: ExtensionEnvironment): boolean {
  if (!manifest) return false
  if (manifest.environments === undefined) return true
  return manifest.environments.some(value => environment === 'app' ? value === 'app' : value === 'web-desktop' || value === 'web-mobile')
}

export function supportsExtensionUI(manifest: EnvironmentManifest | undefined, environment: ExtensionEnvironment): boolean {
  return !!manifest && (manifest.environments === undefined || manifest.environments.includes(environment))
}
