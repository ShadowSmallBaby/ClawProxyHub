import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useIsMobile } from './useIsMobile'
import { extensionEnvironment, supportsExtensionInstallation, supportsExtensionUI, type EnvironmentManifest } from '@/features/extensionEnvironment'

export function useExtensionEnvironment() {
  const { isMobile } = useIsMobile(), { t } = useI18n()
  const environment = computed(() => extensionEnvironment(__CPH_PROFILE__, isMobile.value))
  const acceptsPackage = (manifest?: EnvironmentManifest) => supportsExtensionInstallation(manifest, environment.value)
  const acceptsUI = (manifest?: EnvironmentManifest) => supportsExtensionUI(manifest, environment.value)
  const environmentLabel = (manifest?: EnvironmentManifest) => manifest?.environments?.map(value => t('extensions.environments.' + value)).join(' / ') || t('extensions.allEnvironments')
  return { environment, acceptsPackage, acceptsUI, environmentLabel }
}
