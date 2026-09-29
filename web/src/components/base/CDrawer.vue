<!-- CDrawer — 统一抽屉：移动端底部弹出，其余端右侧弹出；标题在左、操作按钮在右。 -->
<template>
  <t-drawer
    v-bind="drawerAttrs"
    :visible="visible"
    :placement="isPhone ? 'bottom' : 'right'"
    :size="drawerSize"
    :footer="false"
    :close-btn="!showActions"
    close-on-overlay-click
    class="c-drawer"
    :class="{ 'c-drawer--actions': showActions }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <template #header>
      <div class="c-drawer-header">
        <span class="c-drawer-title">{{ header }}</span>
        <span v-if="showActions" class="c-drawer-actions">
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
            @click="emit('confirm')"
          >{{ confirmBtn?.content ?? t('common.ok') }}</t-button>
        </span>
      </div>
    </template>
    <slot />
  </t-drawer>
</template>

<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { useIsMobile } from '../../composables'

defineOptions({ inheritAttrs: false })

interface ConfirmBtn {
  loading?: boolean
  content?: string
  theme?: 'primary' | 'danger' | 'default'
}

// footer 用 undefined 豁免 Vue Boolean 转换，否则确认按钮不渲染
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

const drawerAttrs = computed(() => useAttrs())

const showActions = computed(() => props.footer !== false)

const drawerSize = computed(() => (isPhone.value ? '85%' : props.width || '480px'))
</script>

<style>
/* 自绘头部：撑满抽屉头部，标题在左、按钮在右 */
.c-drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}
.c-drawer-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.c-drawer-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

/* 手机端底部抽屉：body 收窄内边距，内容不贴边不溢出 */
@media (max-width: 767px) {
  .c-drawer.t-drawer__ctx {
    z-index: 2600;
  }
  .c-drawer .t-drawer__body {
    padding: 16px 12px;
  }
  .c-drawer .t-drawer__header {
    padding: 12px 16px;
  }
}
</style>
