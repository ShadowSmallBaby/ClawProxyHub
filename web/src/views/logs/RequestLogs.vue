<template>
  <page-layout :body-key="`${page}:${pageSize}`" :scroll="false">
    <template v-if="!isPhone" #header>
      <page-header>

          <!-- 过滤栏：模糊搜索 + 下拉 + 时间区间，窄屏自动换行 -->
          <filter-bar>
            <t-input v-model="filters.key" :placeholder="$t('logs.searchKey')" clearable class="w-md" @enter="search" />
            <t-input v-model="filters.model" :placeholder="$t('logs.searchModel')" clearable class="w-md" @enter="search" />
            <t-input v-model="filters.route" :placeholder="$t('logs.searchRoute')" clearable class="w-md" @enter="search" />
            <t-select v-model="filters.plugin_id" :placeholder="$t('logs.pluginAll')" clearable class="w-xs">
              <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
            </t-select>
            <t-select v-model="filters.protocol" :placeholder="$t('logs.protocolAll')" clearable class="w-sm">
              <t-option v-for="(_, k) in protocolDict" :key="k" :value="k" :label="dict(protocolDict, k)" />
            </t-select>
            <t-select v-model="filters.status_class" :placeholder="$t('logs.statusAll')" clearable class="w-xs">
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
              class="w-lg"
            />
            <t-button theme="primary" @click="search">{{ $t('logs.search') }}</t-button>
            <t-button variant="outline" @click="reset">{{ $t('logs.reset') }}</t-button>
          </filter-bar>

    </page-header>
    </template>

      <!-- 调用日志 -->

        <c-table
          row-key="ID"
          :data="logs"
          :columns="columns"
          :loading="loading"
          fill
          resizable
          mobile-cards
          :phone-cols="['model', 'status', 'CreatedAt']"
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

    <template #footer>
    <c-pagination
      class="log-pagination"

      v-model="page"
      v-model:pageSize="pageSize"
      :total="total"
      @change="load"
    />

    </template>
    <template #overlays>

    <!-- 手机端：筛选收进底部抽屉，页头隐藏 -->
    <c-drawer v-if="isPhone" v-model:visible="filterOpen" :header="$t('logs.search')" :footer="false">
      <div class="filters">

          <t-input v-model="filters.key" :placeholder="$t('logs.searchKey')" clearable @enter="search" />
          <t-input v-model="filters.model" :placeholder="$t('logs.searchModel')" clearable @enter="search" />
          <t-input v-model="filters.route" :placeholder="$t('logs.searchRoute')" clearable @enter="search" />
          <t-select v-model="filters.plugin_id" :placeholder="$t('logs.pluginAll')" clearable>
            <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
          </t-select>
          <t-select v-model="filters.protocol" :placeholder="$t('logs.protocolAll')" clearable>
            <t-option v-for="(_, k) in protocolDict" :key="k" :value="k" :label="dict(protocolDict, k)" />
          </t-select>
          <t-select v-model="filters.status_class" :placeholder="$t('logs.statusAll')" clearable>
            <t-option value="success" :label="$t('logs.statusSuccess')" />
            <t-option value="client_error" :label="$t('logs.statusClientErr')" />
            <t-option value="server_error" :label="$t('logs.statusServerErr')" />
          </t-select>
          <t-button theme="primary" block @click="search(); filterOpen = false">{{ $t('logs.search') }}</t-button>
          <t-button variant="outline" block @click="reset(); filterOpen = false">{{ $t('logs.reset') }}</t-button>

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
import { FilterIcon } from 'tdesign-icons-vue-next'
import { logsApi } from '@/api/logs'
import { pluginApi } from '@/api/entities'
import LogCells from '@/components/LogCells.vue'
import EllipsisCell from '@/components/EllipsisCell.vue'
import { pluginLabelOf } from '@/utils/lookup'
import { usePagination, useIsMobile } from '@/composables'
import { dict, protocolDict } from '@/utils/dict'
import { modelLabel } from '@/utils/logfmt'
import { fmtTime } from '@/utils/format'
import type { RequestLog } from '@/api/types'

const { t } = useI18n()
const { isPhone } = useIsMobile()
const filterOpen = ref(false)

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
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => fmtTime(row.CreatedAt), align: 'center' },
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
