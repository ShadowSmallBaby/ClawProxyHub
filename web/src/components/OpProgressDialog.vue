<!-- OpProgressDialog — 操作进度弹窗：安装 / 升级 / 重启 / 卸载共用。
     顶部步骤条概览（待办 → 进行中 → 完成 / 失败），下方带时间戳的日志流。 -->
<template>
  <c-dialog
    :visible="visible"
    :header="header"
    width="480px"
    :close-on-overlay-click="false"
    :close-on-esc-keydown="false"
    :close-btn="!running"
    :footer="!running || cancelable"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <!-- 步骤条概览 -->
    <div class="op-steps">
      <div v-for="s in steps" :key="s.key" class="op-step" :class="'is-' + s.status">
        <span class="op-step-dot">
          <t-icon v-if="s.status === 'done'" name="check" />
          <t-icon v-else-if="s.status === 'error'" name="close" />
          <t-loading v-else-if="s.status === 'active'" size="12px" />
          <i v-else class="op-step-pending" />
        </span>
        <span class="op-step-label">{{ s.label }}</span>
      </div>
    </div>

    <!-- 时间戳日志流 -->
    <div v-if="logs.length" ref="logBox" class="op-log">
      <div v-for="(l, i) in logs" :key="i" class="op-log-line" :class="{ 'is-error': l.level === 'error' }">
        <span class="op-log-time">{{ l.time }}</span>
        <span class="op-log-text">{{ l.text }}</span>
      </div>
    </div>

    <!-- 进行中且可取消（含下载）显示取消；否则完成/失败显示关闭 -->
    <template #footer>
      <t-button v-if="running" theme="danger" variant="outline" :disabled="!cancelable" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </t-button>
      <t-button v-else theme="default" @click="emit('update:visible', false)">{{ t('common.close') }}</t-button>
    </template>
  </c-dialog>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CDialog } from './base'

// 一步：pending 待办 / active 进行中 / done 完成 / error 失败
export interface OpStep { key: string; label: string; status: 'pending' | 'active' | 'done' | 'error' }
// 一行日志：HH:MM:SS 时间戳 + 文本；level 决定配色
export interface OpLog { time: string; text: string; level?: 'info' | 'error' }

const props = defineProps<{ visible: boolean; header: string; steps: OpStep[]; logs: OpLog[]; running: boolean; cancelable?: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'cancel'): void }>()
const { t } = useI18n()

// 新日志到达时滚到底
const logBox = ref<HTMLElement>()
watch(() => props.logs.length, () => nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight }))
</script>

<style scoped>
/* 步骤条：横向圆点 + 连接线，当前步高亮 */
.op-steps { display: flex; align-items: center; padding: 4px 2px 14px; }
.op-step { display: flex; align-items: center; gap: 6px; color: var(--td-text-color-placeholder); font-size: 13px; }
.op-step:not(:last-child)::after {
  content: ''; width: 28px; height: 1px; margin: 0 8px;
  background: var(--td-component-stroke);
}
.op-step-dot {
  width: 20px; height: 20px; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;
  background: var(--td-bg-color-component); color: var(--td-text-color-placeholder); font-size: 12px; flex: none;
}
.op-step-pending { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.op-step.is-active { color: var(--td-brand-color); font-weight: 600; }
.op-step.is-active .op-step-dot { background: var(--td-brand-color-light); color: var(--td-brand-color); }
.op-step.is-done { color: var(--td-text-color-secondary); }
.op-step.is-done .op-step-dot { background: var(--td-success-color-light); color: var(--td-success-color); }
.op-step.is-error { color: var(--td-error-color); font-weight: 600; }
.op-step.is-error .op-step-dot { background: var(--td-error-color-light); color: var(--td-error-color); }

/* 日志流：等宽、深底、时间戳次要色 */
.op-log {
  max-height: 200px; overflow-y: auto;
  background: var(--td-bg-color-page); border: 1px solid var(--td-component-stroke); border-radius: 8px;
  padding: 8px 10px; font-family: ui-monospace, "SFMono-Regular", Consolas, monospace; font-size: 12px; line-height: 1.9;
}
.op-log-line { display: flex; gap: 8px; }
.op-log-time { color: var(--td-text-color-placeholder); flex: none; font-variant-numeric: tabular-nums; }
.op-log-text { color: var(--td-text-color-primary); word-break: break-all; }
.op-log-line.is-error .op-log-text { color: var(--td-error-color); }
</style>
