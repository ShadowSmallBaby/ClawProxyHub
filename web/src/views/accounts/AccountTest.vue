<template>
  <c-drawer v-model:visible="testVisible" :header="$t('accounts.testTitle')" :footer="false" width="560px" close-on-overlay-click>
    <t-space v-if="testRow" direction="vertical" style="width: 100%" size="large">
      <t-form>
        <form-item :label="$t('accounts.testEndpoint')">
          <bind-select v-model="testEndpoint" :multiple="false" :options="endpointOptions" />
        </form-item>
        <form-item :label="$t('accounts.testModel')">
          <bind-select v-model="testModel" :multiple="false" :options="testModelOptions" :placeholder="$t('accounts.testModelPh')" />
        </form-item>
        <form-item :label="$t('accounts.testQuestion')">
          <t-input v-model="testQuestion" :placeholder="$t('accounts.testQuestionPh')" />
        </form-item>
      </t-form>
      <t-button theme="primary" block :loading="testing" :disabled="!testModel" @click="runTest">{{ $t('accounts.testRun') }}</t-button>
      <div v-if="testText" class="test-answer">{{ testText }}</div>
      <div v-if="testLogs.length" class="test-logs">
        <div v-for="(l, i) in testLogs" :key="i" class="test-log-line">{{ l }}</div>
      </div>
      <template v-if="testRequest || testEvents.length">
        <t-collapse>
          <t-collapse-panel v-if="testRequest" :header="$t('accounts.testRequest')">
            <pre class="test-raw">{{ testRequest }}</pre>
          </t-collapse-panel>
          <t-collapse-panel v-if="testEvents.length" :header="$t('accounts.testEvents')">
            <pre class="test-raw">{{ testEvents.join('\n') }}</pre>
          </t-collapse-panel>
        </t-collapse>
        <t-button variant="outline" block @click="exportTest">{{ $t('accounts.testExport') }}</t-button>
      </template>
    </t-space>
  </c-drawer>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { CDrawer, FormItem, BindSelect } from '@/components'
import { accountApi } from '@/api/entities'
import { accountTestApi } from '@/api/gateway'
import type { Account, ModelInfo } from '@/api/types'
const props = defineProps<{ row: Account }>()
const emit = defineEmits<{ close: [] }>()
const testModels = ref<ModelInfo[]>([])
// 在线测试抽屉
const testVisible = ref(false)
const testRow = ref<Account | null>(null)
const testEndpoint = ref('chat_completions')
const testModel = ref('')
const testQuestion = ref('')
const testText = ref('')
const testLogs = ref<string[]>([])
const testRequest = ref('')
const testEvents = ref<string[]>([])
const testing = ref(false)

const endpointOptions = [
  { value: 'chat_completions', label: 'chat/completions' },
  { value: 'messages', label: 'messages' },
  { value: 'responses', label: 'responses' },
]
const testModelOptions = computed(() => testModels.value.map((m) => ({ value: m.id, label: m.id })))

// openTest 打开在线测试抽屉，模型候选取账号已存模型
async function openTest(row: Account) {
  testRow.value = row
  testEndpoint.value = 'chat_completions'
  testQuestion.value = ''
  testText.value = ''
  testLogs.value = []
  testRequest.value = ''
  testEvents.value = []
  testModel.value = ''
  testVisible.value = true
  const detail = await accountApi.detail(row.id).catch(() => null)
  if (testRow.value?.id !== row.id) return
  testModels.value = detail?.models ?? []
  if (testModels.value.length) testModel.value = testModels.value[0].id
}

// runTest 直调插件 Chat（绕路由/key），输出响应与日志
async function runTest() {
  if (!testRow.value || !testModel.value) return
  testing.value = true
  testText.value = ''
  testLogs.value = []
  testRequest.value = ''
  testEvents.value = []
  try {
    const resp = await accountTestApi.test(testRow.value.id, {
      endpoint: testEndpoint.value, model: testModel.value, question: testQuestion.value,
    })
    testText.value = resp.text ?? ''
    testLogs.value = resp.logs ?? []
    testRequest.value = resp.request ?? ''
    testEvents.value = resp.events ?? []
  } catch (e: any) {
    testLogs.value = ['✗ ' + (e.message || 'error')]
  } finally {
    testing.value = false
  }
}

// exportTest 导出本次测试的 请求/事件/回答 为 JSON blob 下载
function exportTest() {
  const data = JSON.stringify(
    { request: JSON.parse(testRequest.value || 'null'), events: testEvents.value, text: testText.value, logs: testLogs.value },
    null, 2,
  )
  const url = URL.createObjectURL(new Blob([data], { type: 'application/json' }))
  const a = document.createElement('a')
  a.href = url
  a.download = `test-${testRow.value?.id ?? 0}-${Date.now()}.json`
  a.click()
  URL.revokeObjectURL(url)
}
onMounted(() => openTest(props.row))
watch(testVisible, visible => { if (!visible) emit('close') })
</script>
<style scoped>
.test-answer {
  padding: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
}
.test-logs {
  padding: 8px 12px;
  font-family: monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
}
.test-log-line {
  word-break: break-all;
  line-height: 1.7;
}
.test-raw {
  margin: 0;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
  max-height: 280px;
  overflow: auto;
}
</style>
