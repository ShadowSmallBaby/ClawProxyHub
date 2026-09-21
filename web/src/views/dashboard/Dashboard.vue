<template>
  <div class="page">
    

    <!-- 统计卡片 -->
    <t-row :gutter="[16, 16]">
      <t-col v-for="c in cards" :key="c.label" :span="2">
        <c-card :bordered="false" class="stat-card">
          <div class="stat-inner">
            <div class="stat-icon" :style="{ background: c.bg, color: c.fg }">
              <component :is="c.icon" />
            </div>
            <div class="stat-meta">
              <div class="stat-value">{{ c.value }}</div>
              <div class="stat-label">{{ c.label }}</div>
            </div>
          </div>
        </c-card>
      </t-col>
    </t-row>

    <!-- 趋势 + 模型分布 -->
    <t-row :gutter="[16, 16]" class="block">
      <t-col :span="8">
        <c-card :header="$t('dashboard.trendTitle')" :bordered="false">
          <div ref="trendEl" class="chart" />
        </c-card>
      </t-col>
      <t-col :span="4">
        <c-card :header="$t('dashboard.modelTitle')" :bordered="false">
          <!-- 与左侧趋势图等高（.chart 280px），条目多时卡片内滚动 -->
          <div v-if="modelStats.length" class="model-list">
            <div v-for="m in modelStats" :key="m.name" class="model-row">
              <span class="model-name">{{ m.name }}</span>
              <div class="model-bar-wrap">
                <div class="model-bar" :style="{ width: m.percent + '%' }" />
              </div>
              <span class="model-count">{{ m.count }}</span>
            </div>
          </div>
          <div v-else class="model-empty">
            <t-empty :description="$t('dashboard.noModelData')" />
          </div>
        </c-card>
      </t-col>
    </t-row>

    <!-- 按插件积分 -->
    <t-row :gutter="[16, 16]" class="block">
      <t-col :span="12">
        <c-card :header="$t('dashboard.channelTitle')" :bordered="false">
          <div v-if="quotaPlugins.length" class="quota-grid">
            <div v-for="p in quotaPlugins" :key="p.plugin + '/' + p.instance" class="quota-card">
              <div class="quota-head">
                <span class="quota-plugin">{{ p.label || p.plugin }}</span>
                <span class="quota-accounts">{{ $t('dashboard.accountsN', { n: p.accounts }) }}</span>
              </div>
              <div class="quota-row">
                <span class="quota-key">{{ $t('dashboard.usedCredits') }}</span>
                <span class="quota-value">{{ fmtThousands(p.quota.used_credits) }}</span>
              </div>
              <div class="quota-row">
                <span class="quota-key">{{ $t('dashboard.remainingCredits') }}</span>
                <span class="quota-value">{{ fmtThousands(p.quota.credits) }}</span>
              </div>
              <div class="quota-row">
                <span class="quota-key">{{ $t('dashboard.totalCredits') }}</span>
                <span class="quota-value">{{ fmtThousands(p.quota.total_credits) }}</span>
              </div>
            </div>
          </div>
          <t-empty v-else :description="$t('dashboard.noQuotaData')" />
        </c-card>
      </t-col>
    </t-row>

    <!-- 最近请求 -->
    <t-row :gutter="[16, 16]" class="block">
      <t-col :span="12">
        <c-card :header="$t('dashboard.recentTitle')" :bordered="false">
          <c-table row-key="ID" size="small" :data="recent" :columns="recentColumns" max-height="45vh">
            <template #model="{ row }">
              <span :title="modelLabel(row)">{{ modelLabel(row) }}</span>
            </template>
            <template #status="{ row }">
              <t-tag :theme="row.Status < 400 ? 'success' : 'danger'" variant="light">{{ row.Status }}</t-tag>
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
          </c-table>
        </c-card>
      </t-col>
    </t-row>
  </div>
</template>

<script setup lang="ts">
import { CCard, CTable } from '../../components/base'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import {
  DashboardIcon, CheckCircleIcon, ChartBarIcon, UserIcon, AppIcon, LockOnIcon,
} from 'tdesign-icons-vue-next'
import { statsApi, type QuotaPlugin, type TrendPoint } from '../../api/stats'
import { useChart } from '../../composables'
import LogCells from '../../components/LogCells.vue'
import { dict, protocolDict } from '../../utils/dict'
import { modelLabel } from '../../utils/logfmt'
import type { RequestLog, Stats } from '../../api/types'

const { t } = useI18n()

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const stats = ref<Stats | null>(null)
const trend = ref<TrendPoint[]>([])
const recent = ref<RequestLog[]>([])
const quotaPlugins = ref<QuotaPlugin[]>([])
// 图表容器（useChart 管实例生命周期 + 容器尺寸自适应）
const { el: trendEl, render: renderChart } = useChart(() => chartOption.value)

const cards = computed(() => [
  // 高亮色只作图标底淡色 + 图标色，不作渐变大背景
  { label: t('dashboard.todayRequests'), value: stats.value?.today_requests ?? '-', icon: DashboardIcon, bg: 'var(--td-brand-color-1)', fg: 'var(--td-brand-color)' },
  { label: t('dashboard.successRate'), value: stats.value ? `${stats.value.success_rate}%` : '-', icon: CheckCircleIcon, bg: 'var(--td-success-color-1)', fg: 'var(--td-success-color)' },
  { label: t('dashboard.totalTokens'), value: fmt(stats.value?.total_tokens ?? 0), icon: ChartBarIcon, bg: 'var(--td-warning-color-1)', fg: 'var(--td-warning-color)' },
  { label: t('dashboard.activeAccounts'), value: stats.value?.active_accounts ?? '-', icon: UserIcon, bg: 'var(--td-error-color-1)', fg: 'var(--td-error-color)' },
  { label: t('dashboard.runningPlugins'), value: stats.value?.running_plugins ?? '-', icon: AppIcon, bg: 'var(--td-brand-color-1)', fg: 'var(--td-brand-color)' },
  { label: t('dashboard.activeKeys'), value: stats.value?.active_keys ?? '-', icon: LockOnIcon, bg: 'var(--td-brand-color-1)', fg: 'var(--td-brand-color)' },
])

const recentColumns = computed(() => [
  { colKey: 'model', title: t('dashboard.model'), width: 260, ellipsis: true },
  { colKey: 'Protocol', title: t('dashboard.protocol'), width: 150, cell: (_h: any, { row }: any) => dict(protocolDict, row.Protocol), align: 'center' },
  { colKey: 'status', title: t('common.colStatus'), width: 80, align: 'center' },
  { colKey: 'tokens', title: 'Token', width: 190, align: 'center' },
  { colKey: 'latency', title: t('dashboard.latency'), width: 130, align: 'center' },
  { colKey: 'ua', title: t('logs.client'), width: 140, align: 'center' },
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => row.CreatedAt?.replace('T', ' ').slice(0, 19) ?? '-', align: 'center' },
])

// 模型调用分布（最近 200 条聚合）
const modelStats = computed(() => {
  const counts = new Map<string, number>()
  for (const log of recent.value) {
    counts.set(log.Model, (counts.get(log.Model) ?? 0) + 1)
  }
  const rows = [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 20)
  const max = rows[0]?.count ?? 1
  return rows.map((r) => ({ ...r, percent: Math.max(4, (r.count / max) * 100) }))
})

function fmt(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}

// 积分千分位格式；未采集显示 -
function fmtThousands(n: number | undefined): string {
  if (n === undefined || n === null) return '-'
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

// 图表配置（语言切换后图例/系列名跟随重绘）
const chartOption = computed<echarts.EChartsCoreOption>(() => ({
  grid: { left: 40, right: 40, top: 32, bottom: 28 },
  tooltip: { trigger: 'axis' },
  legend: { data: [t('dashboard.legendRequests'), t('dashboard.legendSuccess')], right: 0, top: 0 },
  xAxis: { type: 'category', data: trend.value.map((p) => p.date.slice(5)), axisLine: { lineStyle: { opacity: 0.3 } }, axisTick: { show: false } },
  yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { opacity: 0.15 } } },
  series: [
    { name: t('dashboard.legendRequests'), type: 'line', smooth: true, data: trend.value.map((p) => p.requests),
      lineStyle: { width: 2.5 }, itemStyle: { color: '#4c7dff' },
      areaStyle: { opacity: 0.18, color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
        { offset: 0, color: 'rgba(76, 125, 255, 0.35)' },
        { offset: 1, color: 'rgba(76, 125, 255, 0)' },
      ]) } },
    { name: t('dashboard.legendSuccess'), type: 'line', smooth: true, data: trend.value.map((p) => p.success),
      lineStyle: { width: 2 }, itemStyle: { color: '#2ba471' } },
  ],
}))

// 语言切换后重绘图表（图例/系列名跟随）
watch(() => t('dashboard.legendRequests'), () => renderChart())

onMounted(async () => {
  const [s, t, l, q] = await Promise.all([
    statsApi.get(),
    statsApi.trend(7),
    statsApi.recentLogs(100),
    statsApi.quota(),
  ])
  stats.value = s
  trend.value = t.trend ?? []
  recent.value = l.logs ?? []
  quotaPlugins.value = q.plugins ?? []
  renderChart()
})
</script>

<style scoped>
.block {
  margin-top: 16px;
}
.stat-inner {
  display: flex;
  align-items: center;
  gap: 14px;
}
.stat-icon {
  width: 46px;
  height: 46px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
}
.stat-value {
  font-size: 26px;
  font-weight: 700;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.stat-label {
  margin-top: 3px;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
.chart {
  height: 280px;
  width: 100%;
}
/* 模型分布与趋势图等高；超出条目内部滚动 */
.model-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 280px;
  overflow-y: auto;
}
.model-empty {
  height: 280px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.model-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.model-name {
  width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-family: ui-monospace, monospace;
}
.model-bar-wrap {
  flex: 1;
  height: 8px;
  border-radius: 4px;
  background: var(--td-gray-color-2);
  overflow: hidden;
}
.model-bar {
  height: 100%;
  border-radius: 4px;
  background: var(--td-brand-color);
  transition: width 0.5s ease;
}
.model-count {
  width: 36px;
  text-align: right;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.quota-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
@media (max-width: 1200px) {
  .quota-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
.quota-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  padding: 12px 14px;
}
.quota-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.quota-plugin {
  font-weight: 600;
  font-size: 14px;
}
.quota-accounts {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.quota-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 3px 0;
  font-size: 13px;
}
.quota-key {
  color: var(--td-text-color-secondary);
}
.quota-value {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.ellipsis { display: inline-block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; vertical-align: bottom; }
</style>
