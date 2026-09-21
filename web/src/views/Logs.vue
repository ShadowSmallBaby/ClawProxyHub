<template>
  <div class="page">
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

    <t-table
      row-key="ID"
      :data="logs"
      :columns="columns"
      :loading="loading"
      :max-height="tableHeight"
      resizable
    >
      <template #key="{ row }">
        <span v-if="row.key_name">{{ row.key_name }}</span>
        <span v-else class="dim">-</span>
      </template>
      <template #model="{ row }">
        <span :title="modelLabel(row)">{{ modelLabel(row) }}</span>
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
        <t-tooltip v-if="row.UserAgent" :content="row.UserAgent" placement="top-left">
          <span class="ellipsis">{{ row.UserAgent }}</span>
        </t-tooltip>
        <span v-else>-</span>
      </template>
    </t-table>

    <!-- 分页：页大小 10/30/50/100/200 -->
    <div class="pager">
      <t-pagination
        v-model="page"
        v-model:pageSize="pageSize"
        :total="total"
        :page-size-options="[10, 30, 50, 100, 200]"
        show-jumper
        @change="load"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import LogCells from '../components/LogCells.vue'
import { dict, protocolDict } from '../utils/dict'
import { modelLabel } from '../utils/logfmt'
import type { RequestLog } from '../api/types'

const { t } = useI18n()

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
const page = ref(1)
const pageSize = ref(30)
const total = ref(0)
// 固定表格高度，内部滚动（视口高度减去过滤栏/分页/边距）
const tableHeight = ref(window.innerHeight - 260)

const filters = reactive({
  key: '',
  model: '',
  route: '',
  plugin_id: undefined as number | undefined,
  protocol: undefined as string | undefined,
  status_class: undefined as string | undefined,
  range: [] as string[],
})

// 插件品牌名映射
function pluginLabel(pluginID: number | null): string {
  if (!pluginID) return '-'
  const p = plugins.value.find((x) => x.id === pluginID)
  return p?.label || p?.name || `#${pluginID}`
}

const columns = computed(() => [
  { colKey: 'key', title: t('logs.key'), width: 120, ellipsis: true },
  { colKey: 'model', title: t('logs.model'), width: 260, ellipsis: true, align: 'center' },
  { colKey: 'instance', title: t('accounts.instance'), width: 110, ellipsis: true, cell: (_h: any, { row }: any) => row.instance_name || pluginLabel(row.PluginID), align: 'center' },
  { colKey: 'Protocol', title: t('logs.protocol'), width: 150, cell: (_h: any, { row }: any) => dict(protocolDict, row.Protocol), align: 'center' },
  { colKey: 'status', title: t('common.colStatus'), width: 80, align: 'center' },
  { colKey: 'tokens', title: 'Token', width: 190, align: 'center' },
  { colKey: 'latency', title: t('logs.latency'), width: 130, align: 'center' },
  { colKey: 'ClientIP', title: 'IP', width: 120, align: 'center' },
  { colKey: 'ua', title: t('logs.client'), width: 140, align: 'center' },
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => row.CreatedAt?.replace('T', ' ').slice(0, 19) ?? '-', align: 'center' },
])

// buildQuery 组装过滤参数（空值不带）
function buildQuery(): string {
  const p = new URLSearchParams()
  p.set('page', String(page.value))
  p.set('page_size', String(pageSize.value))
  if (filters.key.trim()) p.set('key', filters.key.trim())
  if (filters.model.trim()) p.set('model', filters.model.trim())
  if (filters.route.trim()) p.set('route', filters.route.trim())
  if (filters.plugin_id) p.set('plugin_id', String(filters.plugin_id))
  if (filters.protocol) p.set('protocol', filters.protocol)
  if (filters.status_class) p.set('status_class', filters.status_class)
  if (filters.range?.[0]) p.set('from', filters.range[0])
  // 结束日期为纯日期（长度 10）时补当天末刻，含当天全部记录
  if (filters.range?.[1]) {
    const to = filters.range[1]
    p.set('to', to.length === 10 ? `${to} 23:59:59` : to)
  }
  return p.toString()
}

async function load() {
  loading.value = true
  try {
    const [resp, p] = await Promise.all([
      api.get<{ logs: RequestLog[]; total: number }>(`/admin/logs?${buildQuery()}`),
      api.get<{ plugins: { id: number; name: string; label?: string }[] }>('/admin/plugins'),
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
  page.value = 1
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
</script>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
  align-items: center;
}
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
.ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
  cursor: default;
}
.dim {
  color: var(--td-text-color-placeholder);
}
</style>
