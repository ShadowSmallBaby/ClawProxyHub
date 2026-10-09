<!-- CDialog — 居中弹窗：移动端占满视口但内容两侧留间距，桌面居中卡片。 -->
<template>
  <t-dialog
    v-bind="dialogAttrs"
    :visible="visible"
    :width="dialogWidth"
    placement="center"
    :footer="false"
    :close-btn="!showActions"
    :close-on-overlay-click="true"
    attach="body"
    class="c-dialog"
    :class="{ 'c-dialog--actions': showActions }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <template #header>
      <div class="c-dialog-header">
        <span class="c-dialog-title">{{ header }}</span>
        <span v-if="showActions" class="c-dialog-actions">
          <t-button
            v-if="cancelBtn"
            variant="outline"
            :theme="cancelBtn.theme ?? 'default'"
            :loading="cancelBtn.loading"
            @click="emit('cancel')"
          >{{ cancelBtn.content }}</t-button>
          <t-button
            :theme="confirmBtn?.theme ?? 'primary'"
            :loading="confirmBtn?.loading"
            :disabled="confirmBtn?.disabled"
            @click="emit('confirm')"
          >{{ confirmBtn?.content ?? t('common.ok') }}</t-button>
        </span>
      </div>
    </template>
    <slot />
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { useIsMobile } from '@/composables'

defineOptions({ inheritAttrs: false })

interface ConfirmBtn {
  disabled?: boolean
  loading?: boolean
  content?: string
  theme?: 'primary' | 'danger' | 'default'
}

const props = withDefaults(
  defineProps<{
    visible?: boolean
    header?: string
    footer?: boolean
    confirmBtn?: ConfirmBtn
    cancelBtn?: ConfirmBtn
        width?: string
  }>(),
  { footer: undefined },
)

const emit = defineEmits<{
  'update:visible': [v: boolean]
  confirm: []
  cancel: []
}>()

const { isPhone } = useIsMobile()
const { t } = useI18n()

const dialogAttrs = computed(() => useAttrs())
const showActions = computed(() => props.footer !== false)
const dialogWidth = computed(() => (isPhone.value ? '85%' : props.width || '480px'))
</script>

<style>
/* 自绘头部：标题在左、按钮在右 */
.c-dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}
.c-dialog-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.c-dialog-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

/* 移动端：80% 宽居中，弹层层级抬高避免被侧栏/抽屉遮挡 */
@media (max-width: 767px) {
  .c-dialog.t-dialog__ctx {
    z-index: 2600;
  }

  /* position 容器上下留白收窄，给内容更多空间 */
  .c-dialog .t-dialog__position {
    padding: 24px 0;
  }
  /* dialog 弹性限高：内容多时整卡不超过视口，body 内部滚动 */
  .c-dialog .t-dialog {
    display: flex;
    flex-direction: column;
    border-radius: 12px;
    max-height: 100%;
  }
  .c-dialog .t-dialog__header {
    flex-shrink: 0;
    padding: 10px 14px;
  }
  .c-dialog .t-dialog__body {
    flex: 1;
    min-height: 0;
    padding: 12px 14px;
    overflow-y: auto;
  }
}
</style>
