<template>
  <div class="page">
    

    <!-- 统计卡片 -->
    <t-row :gutter="[16, 16]">
      <t-col v-for="c in cards" :key="c.label" :span="2">
        <t-card :bordered="false" class="stat-card">
          <div class="stat-inner">
            <div class="stat-icon" :style="{ background: c.bg, color: c.fg }">
              <component :is="c.icon" />
            </div>
            <div class="stat-meta">
              <div class="stat-value">{{ c.value }}</div>
              <div class="stat-label">{{ c.label }}</div>
            </div>
          </div>
        </t-card>
      </t-col>
    </t-row>

    <!-- 趋势 + 模型分布 -->
    <t-row :gutter="[16, 16]" class="block">
      <t-col :span="8">
        <t-card :header="$t('dashboard.trendTitle')" :bordered="false">
          <div ref="trendEl" class="chart" />
        </t-card>
      </t-col>
      <t-col :span="4">
        <t-card :header="$t('dashboard.modelTitle')" :bordered="false">
          <div v-if="modelStats.length" class="model-list">
            <div v-for="m in modelStats" :key="m.name" class="model-row">
              <span class="model-name">{{ m.name }}</span>
              <div class="model-bar-wrap">
                <div class="model-bar" :style="{ width: m.percent + '%' }" />
              </div>
              <span class="model-count">{{ m.count }}</span>
            </div>
          </div>
          <t-empty v-else :description="$t('dashboard.noModelData')" />
        </t-card>
      </t-col>
    </t-row>

    <!-- 按插件积分 -->
    <t-row :gutter="[16, 16]" class="block">
      <t-col :span="12">
        <t-card :header="$t('dashboard.channelTitle')" :bordered="false">
          <div v-if="quotaPlugins.length" class="quota-grid">
            <div v-for="p in quotaPlugins" :key="p.plugin" class="quota-card">
              <div class="quota-head">
                <span class="quota-plugin">{{ p.plugin }}</span>
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
        </t-card>
      </t-col>
    </t-row>

    <!-- 最近请求 -->
    <t-row :gutter="[16, 16]" class="block">
      <t-col :span="12">
        <t-card :header="$t('dashboard.recentTitle')" :bordered="false">
          <t-table row-key="ID" size="small" :data="recent" :columns="recentColumns">
            <template #status="{ row }">
              <t-tag :theme="row.Status < 400 ? 'success' : 'danger'" variant="light">{{ row.Status }}</t-tag>
            </template>
          </t-table>
        </t-card>
      </t-col>
    </t-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import {
  DashboardIcon, CheckCircleIcon, ChartBarIcon, UserIcon, AppIcon, LockOnIcon,
} from 'tdesign-icons-vue-next'
import { api } from '../api/client'
import { dict, protocolDict } from '../utils/dict'
import type { RequestLog, Stats } from '../api/types'

const { t } = useI18n()

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const stats = ref<Stats | null>(null)
const trend = ref<{ date: string; requests: number; success: number; tokens: number }[]>([])
const recent = ref<RequestLog[]>([])
const quotaPlugins = ref<{ plugin: string; accounts: number; quota: Record<string, number> }[]>([])
const trendEl = ref<HTMLElement>()
let chart: echarts.ECharts | null = null
// 容器尺寸变化（侧栏展开收起、窗口缩放等）时自动重载图表
let resizeObserver: ResizeObserver | null = null

const cards = computed(() => [
  { label: t('dashboard.todayRequests'), value: stats.value?.today_requests ?? '-', icon: DashboardIcon, bg: 'linear-gradient(135deg, var(--td-brand-color-4), var(--td-brand-color-6))', fg: '#fff' },
  { label: t('dashboard.successRate'), value: stats.value ? `${stats.value.success_rate}%` : '-', icon: CheckCircleIcon, bg: 'linear-gradient(135deg, var(--td-success-color-4), var(--td-success-color-6))', fg: '#fff' },
  { label: t('dashboard.totalTokens'), value: fmt(stats.value?.total_tokens ?? 0), icon: ChartBarIcon, bg: 'linear-gradient(135deg, var(--td-warning-color-4), var(--td-warning-color-6))', fg: '#fff' },
  { label: t('dashboard.activeAccounts'), value: stats.value?.active_accounts ?? '-', icon: UserIcon, bg: 'linear-gradient(135deg, var(--td-error-color-4), var(--td-error-color-6))', fg: '#fff' },
  { label: t('dashboard.runningPlugins'), value: stats.value?.running_plugins ?? '-', icon: AppIcon, bg: 'linear-gradient(135deg, #7f8dff, #5a5fd8)', fg: '#fff' },
  { label: t('dashboard.activeKeys'), value: stats.value?.active_keys ?? '-', icon: LockOnIcon, bg: 'linear-gradient(135deg, #4fc3d9, #2a8fa8)', fg: '#fff' },
])

const recentColumns = computed(() => [
  { colKey: 'Model', title: t('dashboard.model'), width: 180, ellipsis: true },
  { colKey: 'Protocol', title: t('dashboard.protocol'), width: 150, cell: (_h: any, { row }: any) => dict(protocolDict, row.Protocol), align: 'center' },
  { colKey: 'status', title: t('common.colStatus'), width: 80, align: 'center' },
  { colKey: 'InputTokens', title: t('dashboard.input'), width: 90, align: 'center' },
  { colKey: 'OutputTokens', title: t('dashboard.output'), width: 90, align: 'center' },
  { colKey: 'LatencyMs', title: t('dashboard.latency'), width: 90, cell: (_h: any, { row }: any) => `${row.LatencyMs}ms`, align: 'center' },
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
    .slice(0, 8)
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

function renderChart() {
  if (!trendEl.value) return
  chart = chart ?? echarts.init(trendEl.value)
  chart.setOption({
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
  })
}

function onResize() {
  chart?.resize()
}

// 语言切换后重绘图表（图例/系列名跟随）
watch(() => t('dashboard.legendRequests'), renderChart)

onMounted(async () => {
  const [s, t, l, q] = await Promise.all([
    api.get<Stats>('/admin/stats'),
    api.get<{ trend: typeof trend.value }>('/admin/stats/trend?days=7'),
    api.get<{ logs: RequestLog[] }>('/admin/logs?limit=200'),
    api.get<{ plugins: typeof quotaPlugins.value }>('/admin/stats/quota'),
  ])
  stats.value = s
  trend.value = t.trend ?? []
  recent.value = l.logs ?? []
  quotaPlugins.value = q.plugins ?? []
  renderChart()
  window.addEventListener('resize', onResize)
  if (trendEl.value) {
    resizeObserver = new ResizeObserver(() => chart?.resize())
    resizeObserver.observe(trendEl.value)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  resizeObserver?.disconnect()
  resizeObserver = null
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.block {
  margin-top: 16px;
}
.stat-card:hover {
  transform: translateY(-2px);
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
  box-shadow: 0 4px 10px rgba(42, 79, 196, 0.15);
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
.model-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
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
  background: linear-gradient(90deg, var(--td-brand-color-4), var(--td-brand-color-6));
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
</style>
