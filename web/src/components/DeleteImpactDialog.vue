<template>
  <!-- 删除确认：先拉影响面预览，确认后执行级联删除；路由/密钥引用不自动改，删完弹窗提醒 -->
  <t-dialog
    :visible="visible"
    :header="header"
    theme="warning"
    width="520px"
    :confirm-btn="{ content: t('common.delete'), theme: 'danger', loading: deleting }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    @confirm="confirm"
  >
    <t-loading :loading="loading" size="small" style="width: 100%">
      <p style="margin: 0 0 8px">{{ message }}</p>
      <template v-if="impact">
        <p v-if="cascadeLines.length" class="impact-title">{{ t('impact.cascade') }}</p>
        <ul v-if="cascadeLines.length" class="impact-list">
          <li v-for="line in cascadeLines" :key="line">{{ line }}</li>
        </ul>
        <t-alert v-if="impact.routes.length || impact.keys.length" theme="warning" style="margin-top: 8px">
          <template #message>
            <div>{{ t('impact.reviewHint') }}</div>
            <div v-if="impact.routes.length">{{ t('impact.routes') }}：{{ impact.routes.join('、') }}</div>
            <div v-if="impact.keys.length">{{ t('impact.keys') }}：{{ impact.keys.join('、') }}</div>
          </template>
        </t-alert>
      </template>
    </t-loading>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api } from '../api/client'
import type { DeleteImpact } from '../api/types'
import { notifyDeleteImpact } from '../utils/impact'

const props = defineProps<{
  visible: boolean
  header: string
  message: string // 确认文案（如「确认删除该账号？」）
  impactUrl: string // GET 预览接口
  deleteUrl?: string // DELETE 执行接口；不传则只做确认，由调用方经 confirm 事件自行执行
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'deleted'): void; (e: 'confirm'): void }>()

const { t } = useI18n()
const loading = ref(false)
const deleting = ref(false)
const impact = ref<DeleteImpact | null>(null)

// 打开时拉预览；接口失败时不阻塞删除，只是没有影响面展示
watch(() => props.visible, async (v) => {
  if (!v) return
  impact.value = null
  loading.value = true
  try {
    impact.value = await api.get<DeleteImpact>(props.impactUrl)
  } catch {
    impact.value = null
  } finally {
    loading.value = false
  }
})

// 级联移除的对象：只列数量 > 0 的项
const cascadeLines = computed(() => {
  const im = impact.value
  if (!im) return []
  const items: [number, string][] = [
    [im.instances, 'impact.instances'],
    [im.groups, 'impact.groups'],
    [im.accounts, 'impact.accounts'],
    [im.task_rules, 'impact.taskRules'],
    [im.task_runs, 'impact.taskRuns'],
  ]
  return items.filter(([n]) => n > 0).map(([n, key]) => `${t(key)} × ${n}`)
})

async function confirm() {
  if (!props.deleteUrl) {
    emit('update:visible', false)
    emit('confirm')
    return
  }
  deleting.value = true
  try {
    const resp = await api.del<{ impact?: DeleteImpact }>(props.deleteUrl)
    emit('update:visible', false)
    emit('deleted')
    notifyDeleteImpact(resp.impact, t)
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    deleting.value = false
  }
}
</script>

<style scoped>
.impact-title {
  margin: 0 0 4px;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}
.impact-list {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
}
</style>
