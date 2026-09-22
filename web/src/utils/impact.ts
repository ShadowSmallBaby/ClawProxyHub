// 级联删除后的提醒：路由/密钥引用不自动改，删完弹窗列出需要复核的项。
import { MessagePlugin } from 'tdesign-vue-next'
import type { DeleteImpact } from '../api/types'

export function notifyDeleteImpact(im: DeleteImpact | undefined, t: (key: string, named?: Record<string, unknown>) => string) {
  if (!im || !(im.routes.length || im.keys.length)) return
  MessagePlugin.warning({
    content: t('impact.afterDelete', { routes: im.routes.join('、') || '-', keys: im.keys.join('、') || '-' }),
    duration: 8000,
    closeBtn: true,
  })
}
