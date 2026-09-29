<!-- FormItem — 表单行：label + 必填角标 + 提示角标（悬浮 tooltip），内容走默认插槽。 -->
<template>
  <div class="s-form-item">
    <div class="s-form-label">
      <span>{{ label }}</span>
      <span v-if="mark" class="s-form-mark">*</span>
      <t-tooltip v-if="tip" :content="tip" placement="top-left" :show-arrow="false">
        <help-circle-icon class="s-form-tip" />
      </t-tooltip>
    </div>
    <div class="s-form-control">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { HelpCircleIcon } from 'tdesign-icons-vue-next'

defineProps<{
  label: string
  tip?: string
  // 必填角标
  mark?: boolean
}>()
</script>

<style scoped>
.s-form-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 0;
}
.s-form-label {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 150px;
  flex-shrink: 0;
  font-size: 14px;
  color: var(--td-text-color-primary);
}
.s-form-tip {
  font-size: 14px;
  color: var(--td-text-color-placeholder);
  cursor: help;
}
.s-form-mark {
  color: var(--td-error-color);
  margin-left: -2px;
}
.s-form-control {
  flex: 1;
  min-width: 0;
}
/* 手机端：标签置顶 + 内容撑满 */
@media (max-width: 767px) {
  .s-form-item {
    flex-direction: column;
    align-items: stretch;
    gap: 6px;
  }
  .s-form-label {
    width: auto;
  }
  .s-form-control :deep(.t-input),
  .s-form-control :deep(.t-select),
  .s-form-control :deep(.t-input-number),
  .s-form-control :deep(.t-input__wrap),
  .s-form-control :deep(.t-textarea) {
    width: 100% !important;
    max-width: 100% !important;
  }
}
</style>
