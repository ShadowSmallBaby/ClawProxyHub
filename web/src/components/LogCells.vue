<template>
  <!-- 日志单元格：kind=tokens（↓输入 ↑输出 ✎缓存读 ✚缓存写 + 明细悬浮）/ latency（首字 / 总耗时） -->
  <t-tooltip v-if="kind === 'tokens'" placement="top-left" :overlay-style="{ minWidth: '220px' }">
    <span class="tokens">
      <span class="tok-in"><arrow-down-icon />{{ fmtTokens(row.InputTokens) }}</span>
      <span class="tok-out"><arrow-up-icon />{{ fmtTokens(row.OutputTokens) }}</span>
      <span class="tok-cache-w"><data-base-icon />{{ fmtTokens(row.CacheCreationTokens) }}</span>
      <span class="tok-cache"><layers-icon />{{ fmtTokens(row.CachedTokens) }}</span>
    </span>
    <template #content>
      <div class="tok-detail">
        <div class="tok-detail-title">{{ $t('logs.tokenDetail') }}</div>
        <div class="tok-detail-row"><span>{{ $t('logs.inputTokens') }}</span><b>{{ fmtTokens(row.InputTokens) }}</b></div>
        <div class="tok-detail-row"><span>{{ $t('logs.outputTokens') }}</span><b>{{ fmtTokens(row.OutputTokens) }}</b></div>
        <div class="tok-detail-row" v-if="row.CachedTokens">
          <span>{{ $t('logs.cached') }}</span><b>{{ fmtTokens(row.CachedTokens) }}</b>
        </div>
        <div class="tok-detail-row" v-if="row.CacheCreationTokens">
          <span>{{ $t('logs.cacheCreation') }}</span><b>{{ fmtTokens(row.CacheCreationTokens) }}</b>
        </div>
        <div class="tok-detail-total"><span>{{ $t('logs.totalTokens') }}</span><b>{{ fmtTokens(sumTokens(row)) }}</b></div>
      </div>
    </template>
  </t-tooltip>
  <t-tooltip v-else placement="top-left">
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

<script setup lang="ts">
import { ArrowDownIcon, ArrowUpIcon, DataBaseIcon, LayersIcon } from 'tdesign-icons-vue-next'
import type { RequestLog } from '../api/types'
import { fmtMs, fmtTokens, sumTokens } from '../utils/logfmt'

defineProps<{ kind: 'tokens' | 'latency'; row: RequestLog }>()
</script>

<style scoped>
.tokens { display: inline-grid; grid-template-columns: auto auto; column-gap: 14px; row-gap: 2px; justify-content: center; white-space: nowrap; cursor: default; font-variant-numeric: tabular-nums; }
.tokens > span { display: inline-flex; align-items: center; gap: 4px; text-align: left; }
.tokens > span :deep(svg) { flex-shrink: 0; }
.tok-in { color: var(--td-success-color); }
.tok-out { color: var(--td-brand-color); }
.tok-cache { color: var(--td-warning-color); cursor: default; }
.tok-cache-w { color: var(--td-text-color-secondary); cursor: default; }
.tok-detail { min-width: 200px; }
.tok-detail-title { font-weight: 700; margin-bottom: 8px; }
.tok-detail-row { display: flex; justify-content: space-between; gap: 24px; padding: 2px 0; }
.tok-detail-row b { font-variant-numeric: tabular-nums; }
.tok-detail-total { display: flex; justify-content: space-between; gap: 24px; margin-top: 6px; padding-top: 6px; border-top: 1px solid rgba(255, 255, 255, 0.2); }
.tok-detail-total b { font-variant-numeric: tabular-nums; }
.latency { display: flex; align-items: center; gap: 8px; cursor: default; }
.latency-bar { width: 3px; height: 28px; border-radius: 2px; background: var(--td-success-color); flex-shrink: 0; }
.latency-nums { font-size: 12px; line-height: 1.5; white-space: nowrap; }
.latency-nums div { display: flex; justify-content: space-between; gap: 6px; }
.latency-nums b { font-variant-numeric: tabular-nums; }
</style>
