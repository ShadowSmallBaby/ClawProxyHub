<template>
  <div class="page">
    <div class="page-header">

      <t-button variant="outline" @click="load">{{ $t('common.refresh') }}</t-button>
    </div>
    <t-table row-key="ID" :data="logs" :columns="columns" :loading="loading">
      <template #key="{ row }">
        <span v-if="row.key_name">{{ row.key_name }}</span>
        <span v-else class="dim">-</span>
      </template>
      <template #status="{ row }">
        <!-- 错误信息并入状态 tooltip -->
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
        <!-- Token 明细合并：输入/输出/缓存 tooltip + 总数 -->
        <t-tooltip placement="top-left" :overlay-style="{ minWidth: '220px' }">
          <span class="tokens">
            <span class="tok-in">↓ {{ fmt(row.InputTokens) }}</span>
            <span class="tok-out">↑ {{ fmt(row.OutputTokens) }}</span>
            <t-tooltip v-if="row.CachedTokens" :content="`${$t('logs.cached')} ${fmt(row.CachedTokens)}`" placement="top">
              <span class="tok-cache">✎ {{ fmtCache(row.CachedTokens) }}</span>
            </t-tooltip>
          </span>
          <template #content>
            <div class="tok-detail">
              <div class="tok-detail-title">{{ $t('logs.tokenDetail') }}</div>
              <div class="tok-detail-row"><span>{{ $t('logs.inputTokens') }}</span><b>{{ fmt(row.InputTokens) }}</b></div>
              <div class="tok-detail-row"><span>{{ $t('logs.outputTokens') }}</span><b>{{ fmt(row.OutputTokens) }}</b></div>
              <div class="tok-detail-row" v-if="row.CachedTokens">
                <span>{{ $t('logs.cached') }}</span><b>{{ fmt(row.CachedTokens) }}</b>
              </div>
              <div class="tok-detail-total"><span>{{ $t('logs.totalTokens') }}</span><b>{{ fmt(total(row)) }}</b></div>
            </div>
          </template>
        </t-tooltip>
      </template>
      <template #latency="{ row }">
        <!-- 首字/总耗时合并：绿条 + tooltip -->
        <t-tooltip placement="top-left">
          <div class="latency">
            <span class="latency-bar"></span>
            <div class="latency-nums">
              <div>{{ $t('logs.firstToken') }} <b>{{ fmtMs(row.FirstTokenMs) }}</b></div>
              <div>{{ $t('logs.totalTime') }} <b>{{ fmtMs(row.LatencyMs) }}</b></div>
            </div>
          </div>
          <template #content>
            <div class="tok-detail">
              <div class="tok-detail-title">{{ $t('logs.latencyTitle') }}</div>
              <div class="tok-detail-row"><span>{{ $t('logs.firstToken') }}</span><b>{{ fmtMs(row.FirstTokenMs) }}</b></div>
              <div class="tok-detail-row"><span>{{ $t('logs.totalTime') }}</span><b>{{ fmtMs(row.LatencyMs) }}</b></div>
            </div>
          </template>
        </t-tooltip>
      </template>
      <template #ua="{ row }">
        <t-tooltip v-if="row.UserAgent" :content="row.UserAgent" placement="top-left">
          <span class="ellipsis">{{ row.UserAgent }}</span>
        </t-tooltip>
        <span v-else>-</span>
      </template>
    </t-table>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import { dict, protocolDict } from '../utils/dict'
import type { RequestLog } from '../api/types'

const { t } = useI18n()

const logs = ref<RequestLog[]>([])
const loading = ref(false)
const plugins = ref<{ id: number; name: string; label?: string }[]>([])

// 插件品牌名映射
function pluginLabel(pluginID: number | null): string {
  if (!pluginID) return '-'
  const p = plugins.value.find((x) => x.id === pluginID)
  return p?.label || p?.name || `#${pluginID}`
}

const columns = computed(() => [
  { colKey: 'key', title: t('logs.key'), width: 130, ellipsis: true },
  { colKey: 'Model', title: t('logs.model'), width: 160, ellipsis: true, align: 'center' },
  { colKey: 'plugin', title: t('logs.plugin'), width: 110, cell: (_h: any, { row }: any) => pluginLabel(row.PluginID), align: 'center' },
  { colKey: 'Protocol', title: t('logs.protocol'), width: 150, cell: (_h: any, { row }: any) => dict(protocolDict, row.Protocol), align: 'center' },
  { colKey: 'status', title: t('common.colStatus'), width: 80, align: 'center' },
  { colKey: 'tokens', title: 'Token', width: 190, align: 'center' },
  { colKey: 'latency', title: t('logs.latency'), width: 130, align: 'center' },
  { colKey: 'ClientIP', title: 'IP', width: 120, align: 'center' },
  { colKey: 'ua', title: t('logs.client'), width: 140, align: 'center' },
  { colKey: 'CreatedAt', title: t('common.colTime'), width: 170, cell: (_h: any, { row }: any) => row.CreatedAt?.replace('T', ' ').slice(0, 19) ?? '-', align: 'center' },
])

// 大数缩写：如 35.9K / 110.2K
function fmt(n: number): string {
  if (!n) return '0'
  if (n < 1000) return String(n)
  if (n < 1000000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}K`
  return `${(n / 1000000).toFixed(2)}M`
}

// 缓存 token 取整 K 展示（如 5m 对齐输入输出）
function fmtCache(n: number): string {
  return fmt(n)
}

// 毫秒展示：<1s 展示 ms，否则秒（如 3.50s）
function fmtMs(ms: number): string {
  if (!ms) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function total(row: RequestLog): number {
  return (row.InputTokens || 0) + (row.OutputTokens || 0) + (row.CachedTokens || 0)
}

async function load() {
  loading.value = true
  try {
    const [resp, p] = await Promise.all([
      api.get<{ logs: RequestLog[] }>('/admin/logs?limit=200'),
      api.get<{ plugins: { id: number; name: string; label?: string }[] }>('/admin/plugins'),
    ])
    logs.value = resp.logs ?? []
    plugins.value = p.plugins ?? []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
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
/* Token 合并列：输入/输出 + 可选缓存 */
.tokens {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  white-space: nowrap;
  cursor: default;
}
.tok-in {
  color: var(--td-success-color);
}
.tok-out {
  color: var(--td-brand-color);
}
.tok-cache {
  color: var(--td-warning-color);
  cursor: default;
}
/* Token 明细 tooltip */
.tok-detail {
  min-width: 200px;
}
.tok-detail-title {
  font-weight: 700;
  margin-bottom: 8px;
}
.tok-detail-row {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 2px 0;
}
.tok-detail-row b {
  font-variant-numeric: tabular-nums;
}
.tok-detail-total {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px solid rgba(255, 255, 255, 0.2);
}
.tok-detail-total b {
  font-variant-numeric: tabular-nums;
}
/* 延迟合并列：绿条 + 首字/总耗时 */
.latency {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: default;
}
.latency-bar {
  width: 3px;
  height: 28px;
  border-radius: 2px;
  background: var(--td-success-color);
  flex-shrink: 0;
}
.latency-nums {
  font-size: 12px;
  line-height: 1.5;
  white-space: nowrap;
}
.latency-nums div {
  display: flex;
  justify-content: space-between;
  gap: 6px;
}
.latency-nums b {
  font-variant-numeric: tabular-nums;
}
</style>
