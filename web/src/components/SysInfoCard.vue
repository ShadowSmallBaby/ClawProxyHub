<!-- SysInfoCard — 系统基本信息卡片：t-descriptions 一列展示 + 待换入备份提醒。 -->
<template>
  <div class="sys-info">
    <t-descriptions v-if="sys" :column="column" bordered size="small" class="sys-desc">
      <t-descriptions-item :label="$t('settings.sysVersion')">v{{ sys.version }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysProtocol')">v{{ sys.protocol_version }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysRuntime')">{{ sys.go_version }} · {{ sys.os }}/{{ sys.arch }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysStarted')">{{ fmtTime(sys.started_at) }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysUptime')">{{ uptime }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysDataDir')"><code>{{ sys.data_dir }}</code></t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysDbSize')">{{ fmtBytes(sys.db_size_bytes) }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysMigration')">{{ sys.migration_version }}</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysMem')">{{ fmtBytes(sys.mem_alloc_bytes) }} · {{ sys.goroutines }} goroutines</t-descriptions-item>
      <t-descriptions-item :label="$t('settings.sysCounts')">
        <span class="counts">
          <span v-for="c in countItems" :key="c.key"><b>{{ sys.counts[c.key] ?? 0 }}</b> {{ $t(c.label) }}</span>
        </span>
      </t-descriptions-item>
    </t-descriptions>
    <t-alert v-if="sys?.pending_restore" theme="warning" class="restore-alert" :message="$t('settings.pendingRestore')" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { fmtTime, fmtBytes } from '@/utils/format'
import type { SysInfo } from '@/api/settings'

const props = withDefaults(defineProps<{ sys: SysInfo | null; column?: number }>(), { column: 1 })
const { t } = useI18n()

const countItems = [
  { key: 'plugins', label: 'settings.countPlugins' }, { key: 'instances', label: 'settings.countInstances' },
  { key: 'accounts', label: 'settings.countAccounts' }, { key: 'groups', label: 'settings.countGroups' },
  { key: 'routes', label: 'settings.countRoutes' }, { key: 'keys', label: 'settings.countKeys' },
  { key: 'request_logs', label: 'settings.countLogs' }, { key: 'task_runs', label: 'settings.countRuns' },
]

const uptime = computed(() => {
  const s = props.sys?.uptime_seconds ?? 0
  return t('settings.uptimeFmt', { d: Math.floor(s / 86400), h: Math.floor((s % 86400) / 3600), m: Math.floor((s % 3600) / 60) })
})
</script>

<style scoped>
.sys-desc code {
  font-size: 12px;
  word-break: break-all;
}
/* 长值（如数据目录）溢出换行：表格 fixed 布局 + 单元格换行 */
.sys-desc :deep(.t-descriptions__body) {
  table-layout: fixed;
  width: 100%;
}
.sys-desc :deep(.t-descriptions__content) {
  word-break: break-all;
  min-width: 0;
}
.sys-desc :deep(.t-descriptions__label) {
  width: 96px;
}
.counts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.counts b {
  color: var(--td-text-color-primary);
  font-variant-numeric: tabular-nums;
}
.sys-info {
  margin-bottom: 20px;
}
.restore-alert {
  margin-bottom: 16px;
}
</style>
