<template>
  <div class="page">
    <page-header>
      <template v-if="tab === 'requests'">
        <!-- 过滤栏：模糊搜索 + 下拉 + 时间区间，窄屏自动换行 -->
        <div class="filters">
          <t-input v-model="filters.key" :placeholder="$t('logs.searchKey')" clearable style="width: 280px" @enter="search" />
          <t-input v-model="filters.model" :placeholder="$t('logs.searchModel')" clearable style="width: 280px" @enter="search" />
          <t-input v-model="filters.route" :placeholder="$t('logs.searchRoute')" clearable style="width: 280px" @enter="search" />
          <t-select v-model="filters.plugin_id" :placeholder="$t('logs.pluginAll')" clearable style="width: 130px">
            <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
          </t-select>
          <t-select v-model="filters.protocol" :placeholder="$t('logs.protocolAll')" clearable style="width: 160px">
            <t-option v-for="(v, k) in protocolDict" :key="k" :value="k" :label="dict(protocolDict, k)" />
          </t-select>
          <t-select v-model="filters.status_class" :placeholder="$t('logs.statusAll')" clearable style="width: 120px">
            <t-option value="success" :label="$t('logs.statusSuccess')" />
            <t-option value="client_error" :label="$t('logs.statusClientErr')" />
            <t-option value="server_error" :label="$t('logs.statusServerErr')" />
          </t-select>
          <t-date-range-picker
            v-model="filters.range"
            allow-input
            clearable
            :presets="presets"
            presets-placement="bottom"
            :placeholder="[$t('logs.timeFrom'), $t('logs.timeTo')]"
            style="width: 300px"
          />
          <t-button theme="primary" @click="search">{{ $t('logs.search') }}</t-button>
          <t-button variant="outline" @click="reset">{{ $t('logs.reset') }}</t-button>
        </div>
      </template>
      <template v-else>
        <div class="filters">
          <t-select v-model="runFilters.level" :placeholder="$t('logs.runLevelAll')" clearable style="width: 130px">
            <t-option value="error" :label="$t('settings.runLevelError')" />
            <t-option value="warn" :label="$t('settings.runLevelWarn')" />
            <t-option value="debug" :label="$t('settings.runLevelDebug')" />
            <t-option value="info" :label="$t('settings.runLevelInfo')" />
          </t-select>
          <t-input v-model="runFilters.module" :placeholder="$t('logs.runModulePh')" clearable style="width: 160px" @enter="searchRun" />
          <t-input v-model="runFilters.keyword" :placeholder="$t('logs.runKeywordPh')" clearable style="width: 280px" @enter="searchRun" />
          <t-button theme="primary" @click="searchRun">{{ $t('logs.search') }}</t-button>
          <t-button variant="outline" @click="resetRun">{{ $t('logs.reset') }}</t-button>
        </div>
      </template>
    </page-header>

    <c-tabs v-model="tab" size="medium" class="log-tabs">
      <!-- 调用日志 -->
      <t-tab-panel value="requests" :label="$t('logs.reqTab')">
        <c-table
          row-key="ID"
          :data="logs"
          :columns="columns"
          :loading="loading"
          height="100%"
          resizable
        >
          <template #key="{ row }">
            <span v-if="row.key_name">{{ row.key_name }}</span>
            <span v-else class="dim">-</span>
          </template>
          <template #model="{ row }">
            <span :title="modelLabel(row)">{{ modelLabel(row) }}</span>
          </template>
          <template #stream="{ row }">
            <t-tag :theme="row.Stream ? 'primary' : 'default'" variant="light">{{ row.Stream ? $t('logs.stream') : $t('logs.sync') }}</t-tag>
          </template>
          <template #status="{ row }">
            <t-tooltip
              v-if="row.Status >= 400 && row.ErrorBrief"
              :content="`${row.Status} · ${row.ErrorBrief}`"
              placement="top-left"
              :overlay-style="{ maxWidth: '640px', whiteSpace: 'pre-wrap' }"
            >
              <t-tag :theme="row.Status < 400 ? 'success' : 'danger'" variant="light">{{ row.Status }}</t-tag>
            </t-tooltip>
            <t-tag v-else :theme="row.Status < 400 ? 'success' : 'danger'" variant="light">{{ row.Status }}</t-tag>
          </template>
          <template #tokens="{ row }">
            <log-cells kind="tokens" :row="row" />
          </template>
          <template #latency="{ row }">
            <log-cells kind="latency" :row="row" />
          </template>
          <template #ua="{ row }">
            <ellipsis-cell :content="row.UserAgent" />
          </template>
        </c-table>
      </t-tab-panel>

      <!-- 运行日志 -->
      <t-tab-panel value="runs" :label="$t('logs.runTab')">
        <c-table
          row-key="ID"
          :data="runLogs"
          :columns="runColumns"
          :loading="runLoading"
          height="100%"
          @row-click="openRun"
        >
          <template #level="{ row }">
            <t-tag :theme="levelTheme(row.Level)" variant="light">{{ levelText(row.Level) }}</t-tag>
          </template>
        </c-table>
      </t-tab-panel>
    </c-tabs>

    <!-- 分页（任务页同款：外置于 tabs 下方，按当前 tab 切换数据源） -->
    <t-pagination
      class="log-pagination"
      v-if="tab === 'requests'"
      v-model="page"
      v-model:pageSize="pageSize"
      :total="total"
      :page-size-options="[10, 30, 50, 100, 200]"
      show-jumper
      @change="load"
    />
    <t-pagination
      class="log-pagination"
      v-else
      v-model="runPage"
      v-model:pageSize="runPageSize"
      :total="runTotal"
      :page-size-options="[10, 30, 50, 100, 200]"
      show-jumper
      @change="loadRun"
    />

    <!-- 运行日志明细抽屉 -->
    <t-drawer v-model:visible="runVisible" :header="$t('logs.runDetail')" size="560px">
      <template #footer>
        <t-button theme="primary" :disabled="!runRow" @click="exportRunJson">
          <template #icon><download-icon /></template>{{ $t('logs.exportJson') }}
        </t-button>
      </template>
      <div v-if="runRow" class="run-detail">
        <div class="run-meta">
          <t-tag :theme="levelTheme(runRow.Level)" variant="light">{{ levelText(runRow.Level) }}</t-tag>
          <span class="dim">{{ runRow.CreatedAt?.replace('T', ' ').slice(0, 19) }}</span>
        </div>
        <div class="run-line"><b>{{ $t('logs.runModule') }}:</b> {{ runRow.Module }}</div>
        <div class="run-line"><b>{{ $t('logs.runAction') }}:</b> {{ runRow.Action }}</div>
        <div class="run-line"><b>{{ $t('logs.runMessage') }}:</b> {{ runRow.Message }}</div>
        <pre v-if="runRow.Detail" class="run-raw">{{ runRow.Detail }}</pre>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { CTable, CTabs } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DownloadIcon } from 'tdesign-icons-vue-next'
import { logsApi, runLogsApi } from '../../api/logs'
import { pluginApi } from '../../api/entities'
import LogCells from '../../components/LogCells.vue'
import EllipsisCell from '../../components/EllipsisCell.vue'
import { pluginLabelOf } from '../../utils/lookup'
import { usePagination } from '../../composables'
import { dict, protocolDict } from '../../utils/dict'
import { modelLabel } from '../../utils/logfmt'
import type { RequestLog, RunLog } from '../../api/types'

const { t } = useI18n()
const tab = ref('requests')

// 时间快捷区间：原生 Date 计算，只到日期（不含时间），返回 [起, 止]（避免引入 dayjs）
function fmtDate(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}
// 以周一为一周起点，返回该周周一日期
function weekStartDate(base: Date, offsetWeeks = 0): Date {
  const x = new Date(base); x.setHours(0, 0, 0, 0)
  const dow = (x.getDay() + 6) % 7 // 周一=0
  x.setDate(x.getDate() - dow + offsetWeeks * 7)
  return x
}
function rangeStr(from: Date, to: Date): string[] { return [fmtDate(from), fmtDate(to)] }
const presets = computed<Record<string, string[]>>(() => {
  const now = new Date()
  const dayMs = 86400000
  const yesterday = new Date(now.getTime() - dayMs)
  const lastWeekMon = weekStartDate(now, -1)
  const lastWeekSun = new Date(weekStartDate(now).getTime() - dayMs)
  return {
    [t('logs.presetToday')]: rangeStr(now, now),
    [t('logs.presetYesterday')]: rangeStr(yesterday, yesterday),
    [t('logs.presetThisWeek')]: rangeStr(weekStartDate(now), now),
    [t('logs.presetLastWeek')]: rangeStr(lastWeekMon, lastWeekSun),
    [t('logs.presetLast7')]: rangeStr(new Date(now.getTime() - 6 * dayMs), now),
    [t('logs.presetLast30')]: rangeStr(new Date(now.getTime() - 29 * dayMs), now),
  }
})

const logs = ref<RequestLog[]>([])
const loading = ref(false)
const plugins = ref<{ id: number; name: string; label?: string }[]>([])
const { page, pageSize, total, reset: resetPage } = usePagination(30)

const filters = reactive({
  key: '',
  model: '',
  route: '',
  plugin_id: undefined as number | undefined,
  protocol: undefined as string | undefined,
  status_class: undefined as string | undefined,
  range: [] as string[],
})

const pluginLabel = (pluginID: number | null) => pluginLabelOf(plugins.value, pluginID)
const columns = computed(() => [
  { colKey: 'key', title: t('logs.key'), width: 120, ellipsis: true },
  { colKey: 'model', title: t('logs.model'), width: 260, ellipsis: true, align: 'center' },
  { colKey: 'instance', title: t('accounts.instance'), width: 110, ellipsis: true, cell: (_h: any, { row }: any) => row.instance_name || pluginLabel(row.PluginID), align: 'center' },
  { colKey: 'Protocol', title: t('logs.protocol'), width: 150, cell: (_h: any, { row }: any) => dict(protocolDict, row.Protocol), align: 'center' },
  { colKey: 'stream', title: t('logs.streamType'), width: 80, align: 'center' },
  { colKey: 'status', title: t('common.colStatus'), width: 80, align: 'center' },
  { colKey: 'tokens', title: 'Token', width: 190, align: 'center' },
  { colKey: 'latency', title: t('logs.latency'), width: 130, align: 'center' },
  { colKey: 'ClientIP', title: 'IP', width: 120, align: 'center' },
  { colKey: 'ua', title: t('logs.client'), width: 140, align: 'center' },
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => row.CreatedAt?.replace('T', ' ').slice(0, 19) ?? '-', align: 'center' },
])

async function load() {
  loading.value = true
  try {
    // 结束日期为纯日期（长度 10）时补当天末刻，含当天全部记录
    const to = filters.range?.[1] ? (filters.range[1].length === 10 ? `${filters.range[1]} 23:59:59` : filters.range[1]) : undefined
    const [resp, p] = await Promise.all([
      logsApi.list(page.value, pageSize.value, {
        key: filters.key.trim(), model: filters.model.trim(), route: filters.route.trim(),
        plugin_id: filters.plugin_id, protocol: filters.protocol, status_class: filters.status_class,
        from: filters.range?.[0], to,
      }),
      pluginApi.list(),
    ])
    logs.value = resp.logs ?? []
    total.value = resp.total ?? 0
    plugins.value = p.plugins ?? []
  } finally {
    loading.value = false
  }
}

// search 重置到第一页再查
function search() {
  resetPage()
  load()
}

function reset() {
  filters.key = ''; filters.model = ''; filters.route = ''
  filters.plugin_id = undefined; filters.protocol = undefined; filters.status_class = undefined
  filters.range = []
  page.value = 1
  load()
}

onMounted(load)

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
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => row.CreatedAt?.replace('T', ' ').slice(0, 19) ?? '-', align: 'center' },
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

watch(tab, (v) => {
  if (v === 'runs' && !runLogs.value.length) loadRun()
})
</script>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
  align-items: center;
}
.page {
  /* 撑满内容区：页头/分页固定，表格吃掉中间剩余高度并自适应窗口 */
  height: 100%;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
}
.page-header,
.log-pagination {
  flex-shrink: 0;
}
.log-tabs {
  /* 占满剩余空间；min-height:0 允许收缩以触发表格内部滚动 */
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
:deep(.t-tabs__content) {
  flex: 1;
  min-height: 0;
}
:deep(.t-tab-panel),
:deep(.log-tabs .t-table) {
  /* 把 height:100% 的高度链一路传到表格滚动容器 */
  height: 100%;
}
.log-pagination {
  margin-top: 12px;
  justify-content: flex-end;
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
