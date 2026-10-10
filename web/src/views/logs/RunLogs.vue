<template>
  <page-layout :body-key="`${runPage}:${runPageSize}`" :scroll="false">
    <template v-if="!isPhone" #header>
      <page-header>

        <filter-bar>
          <t-select v-model="runFilters.level" :placeholder="$t('logs.runLevelAll')" clearable class="w-xs">
            <t-option value="error" :label="$t('settings.runLevelError')" />
            <t-option value="warn" :label="$t('settings.runLevelWarn')" />
            <t-option value="debug" :label="$t('settings.runLevelDebug')" />
            <t-option value="info" :label="$t('settings.runLevelInfo')" />
          </t-select>
          <t-input v-model="runFilters.module" :placeholder="$t('logs.runModulePh')" clearable class="w-sm" @enter="searchRun" />
          <t-input v-model="runFilters.keyword" :placeholder="$t('logs.runKeywordPh')" clearable class="w-md" @enter="searchRun" />
          <t-button theme="primary" @click="searchRun">{{ $t('logs.search') }}</t-button>
          <t-button variant="outline" @click="resetRun">{{ $t('logs.reset') }}</t-button>
        </filter-bar>

    </page-header>
    </template>

      <!-- 运行日志 -->

        <c-table
          row-key="ID"
          :data="runLogs"
          :columns="runColumns"
          :loading="runLoading"
          fill
          mobile-cards
          :phone-cols="['level', 'Action', 'CreatedAt']"
          @row-click="openRun"
        >
          <template #level="{ row }">
            <t-tag :theme="levelTheme(row.Level)" variant="light">{{ levelText(row.Level) }}</t-tag>
          </template>
        </c-table>

    <template #footer>

    <c-pagination
      class="log-pagination"

      v-model="runPage"
      v-model:pageSize="runPageSize"
      :total="runTotal"
      @change="loadRun"
    />

    </template>
    <template #overlays>

    <!-- 手机端：筛选收进底部抽屉，页头隐藏 -->
    <c-drawer v-if="isPhone" v-model:visible="filterOpen" :header="$t('logs.search')" :footer="false">
      <div class="filters">

          <t-select v-model="runFilters.level" :placeholder="$t('logs.runLevelAll')" clearable>
            <t-option value="error" :label="$t('settings.runLevelError')" />
            <t-option value="warn" :label="$t('settings.runLevelWarn')" />
            <t-option value="debug" :label="$t('settings.runLevelDebug')" />
            <t-option value="info" :label="$t('settings.runLevelInfo')" />
          </t-select>
          <t-input v-model="runFilters.module" :placeholder="$t('logs.runModulePh')" clearable @enter="searchRun" />
          <t-input v-model="runFilters.keyword" :placeholder="$t('logs.runKeywordPh')" clearable @enter="searchRun" />
          <t-button theme="primary" block @click="searchRun(); filterOpen = false">{{ $t('logs.search') }}</t-button>
          <t-button variant="outline" block @click="resetRun(); filterOpen = false">{{ $t('logs.reset') }}</t-button>

      </div>
    </c-drawer>

    <!-- 运行日志明细抽屉 -->
    <c-drawer v-model:visible="runVisible" :header="$t('logs.runDetail')" :footer="false" width="560px" close-on-overlay-click>
      <div v-if="runRow" class="run-detail">
        <div class="run-meta">
          <t-tag :theme="levelTheme(runRow.Level)" variant="light">{{ levelText(runRow.Level) }}</t-tag>
          <span class="dim">{{ fmtTime(runRow.CreatedAt) }}</span>
        </div>
        <div class="run-line"><b>{{ $t('logs.runModule') }}:</b> {{ runRow.Module }}</div>
        <div class="run-line"><b>{{ $t('logs.runAction') }}:</b> {{ runRow.Action }}</div>
        <div class="run-line"><b>{{ $t('logs.runMessage') }}:</b> {{ runRow.Message }}</div>
        <pre v-if="runRow.Detail" class="run-raw">{{ runRow.Detail }}</pre>
        <t-button theme="primary" block style="margin-top: 12px" @click="exportRunJson">
          <template #icon><download-icon /></template>{{ $t('logs.exportJson') }}
        </t-button>
      </div>
    </c-drawer>

    <!-- 手机端：筛选悬浮按钮 -->
    <mobile-fab v-if="isPhone">
      <t-button theme="primary" shape="circle" size="large" @click="filterOpen = true">
        <template #icon><filter-icon /></template>
      </t-button>
    </mobile-fab>
    </template>
  </page-layout>
</template>

<script setup lang="ts">
import { PageLayout, PageHeader } from '@/components'
import { CTable, CPagination, MobileFab , CDrawer, FilterBar } from '@/components/base'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DownloadIcon, FilterIcon } from 'tdesign-icons-vue-next'
import { runLogsApi } from '@/api/logs'
import { usePagination, useIsMobile } from '@/composables'
import { fmtTime } from '@/utils/format'
import type { RunLog } from '@/api/types'

const { t } = useI18n()
const { isPhone } = useIsMobile()
const filterOpen = ref(false)

// ---------- 运行日志 ----------

const runLogs = ref<RunLog[]>([])
const runLoading = ref(false)
const { page: runPage, pageSize: runPageSize, total: runTotal, reset: resetRunPage } = usePagination(30)
const runFilters = reactive({ level: undefined as string | undefined, module: '', keyword: '' })
const runVisible = ref(false)
const runRow = ref<RunLog | null>(null)

// levelDict / levelTheme 级别展示（值是纯字符串文案，非 {zh,en}；直接拼 Record<string, string>）
const levelDict = computed<Record<string, string>>(() => ({
  error: t('settings.runLevelError'), warn: t('settings.runLevelWarn'),
  debug: t('settings.runLevelDebug'), info: t('settings.runLevelInfo'),
}))
function levelText(level: string): string {
  return levelDict.value[level] || level
}
function levelTheme(level: string) {
  return level === 'error' ? 'danger' : level === 'warn' ? 'warning' : level === 'debug' ? 'primary' : 'success'
}

const runColumns = computed(() => [
  { colKey: 'level', title: t('logs.runLevel'), width: 80, align: 'center' },
  { colKey: 'Module', title: t('logs.runModule'), width: 100, ellipsis: true, align: 'center' },
  { colKey: 'Action', title: t('logs.runAction'), width: 120, ellipsis: true, align: 'center' },
  { colKey: 'Message', title: t('logs.runMessage'), ellipsis: true },
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => fmtTime(row.CreatedAt), align: 'center' },
])

async function loadRun() {
  runLoading.value = true
  try {
    const resp = await runLogsApi.list(runPage.value, runPageSize.value, {
      level: runFilters.level, module: runFilters.module.trim(), keyword: runFilters.keyword.trim(),
    })
    runLogs.value = resp.logs ?? []
    runTotal.value = resp.total ?? 0
  } finally {
    runLoading.value = false
  }
}
function searchRun() {
  resetRunPage()
  loadRun()
}
function resetRun() {
  runFilters.level = undefined
  runFilters.module = ''
  runFilters.keyword = ''
  runPage.value = 1
  loadRun()
}
function openRun(ctx: { row: RunLog }) {
  runRow.value = ctx.row
  runVisible.value = true
}

// 导出当前明细为 JSON 文件（纯前端，Blob 落盘）
function exportRunJson() {
  if (!runRow.value) return
  const data = JSON.stringify(runRow.value, null, 2)
  const url = URL.createObjectURL(new Blob([data], { type: 'application/json' }))
  const a = Object.assign(document.createElement('a'), { href: url, download: `runlog-${runRow.value.ID}.json` })
  a.click()
  URL.revokeObjectURL(url)
}

onMounted(loadRun)
</script>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
  align-items: center;
}
/* 手机端筛选项撑满整行，不再用写死宽度横向溢出 */
@media (max-width: 767px) {
  .filters > * {
    width: 100% !important;
    flex: 1 1 100%;
  }
}
.dim {
  color: var(--td-text-color-placeholder);
}
.run-detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.run-meta {
  display: flex;
  gap: 10px;
  align-items: center;
}
.run-line {
  word-break: break-all;
  line-height: 1.7;
  font-size: 13px;
}
.run-raw {
  margin: 0;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container-hover);
  border-radius: 6px;
  max-height: 320px;
  overflow: auto;
}
</style>
