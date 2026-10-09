// 消息会话复用公共 SDK；编辑器只负责自己的主题设置。
import { shallowRef } from 'vue'
import { onContext } from '../../../sdk/extension/bridge.js'

export { ready, invoke, ui, onUIEvent, activate, draft } from '../../../sdk/extension/bridge.js'

export const appearance = shallowRef({ locale: 'zh', dark: true })
onContext(context => {
  if (typeof context.locale !== 'string' || typeof context.dark !== 'boolean') return
  const theme = context.settings?.theme ?? 'dark'
  appearance.value = { locale: context.locale, dark: theme === 'dark' || theme !== 'light' && context.dark }
})
