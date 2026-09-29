<!-- CTable — 统一二次封装表格：行悬停背景提亮 + 斑马纹关闭，集中管理边框/圆角。
     移动端：传 mobile-cards 时按列定义渲染为卡片列表（复用各列 slot），桌面仍是表格。 -->
<template>
  <div v-if="mobileCards && isMobile" class="c-cards" :class="{ 'is-loading': loading, 'is-scroll': cardScroll }">
    <div v-if="loading" class="c-cards-loading"><t-loading size="small" /></div>
    <div v-else-if="!data?.length" class="c-cards-empty"><t-empty /></div>
    <div v-for="(row, i) in data" v-else :key="rowKeyOf(row, i)" class="c-card-row">
      <div v-for="col in shownCols(row)" :key="col.colKey" class="c-card-field" :class="{ 'is-op': col.colKey === opKey }">
        <span v-if="col.colKey !== opKey" class="c-card-label">{{ col.title }}</span>
        <span class="c-card-value">
          <slot :name="col.colKey" :row="row">
            {{ cellText(col, row) }}
          </slot>
        </span>
      </div>
      <t-link
        v-if="foldedCount(row)"
        theme="primary"
        size="small"
        class="c-card-more"
        @click="toggleFold(row, i)"
      >
        {{ expanded.has(rowKeyOf(row, i)) ? $t('common.collapse') : `${$t('common.more')} (${foldedCount(row)})` }}
      </t-link>
    </div>
  </div>
  <t-table v-else v-bind="$attrs" :data="data" :columns="columns" :loading="loading" :row-key="rowKey" class="c-table">
    <template v-for="(_, name) in $slots" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
  </t-table>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useIsMobile } from '@/composables'

defineOptions({ inheritAttrs: false })

interface Column {
  colKey: string
  title?: string
  // 函数 = 自定义取值；字符串 = TDesign 插槽名，卡片里退回原始字段
  cell?: string | ((h: unknown, ctx: { row: Record<string, unknown> }) => unknown)
}

const props = defineProps<{
  data?: Record<string, any>[]
  columns?: Column[]
  // 行唯一键：字段名，或按行计算（动态列表无稳定 id 时用）
  rowKey?: string | ((row: Record<string, any>, index: number) => string | number)
  loading?: boolean
  // 移动端改卡片渲染（桌面不受影响）
  mobileCards?: boolean
  // 手机端只展示这些列（colKey），其余折叠；不传则全展示。平板始终全展示
  phoneCols?: string[]
  // 操作列 colKey，卡片里单独放底部
  opKey?: string
}>()

const { isMobile, isPhone } = useIsMobile()
const cardScroll = computed(() => props.mobileCards && isPhone.value)

const rowKey = computed(() => props.rowKey ?? 'id')

function rowKeyOf(row: Record<string, any>, index: number): string | number {
  const k = rowKey.value
  return typeof k === 'function' ? k(row, index) : row[k]
}
const opKey = computed(() => props.opKey ?? 'op')

// 展开状态：按行 key 记录哪些卡片展开了折叠列
const expanded = ref(new Set<string | number>())

function toggleFold(row: Record<string, any>, index: number) {
  const k = rowKeyOf(row, index)
  const next = new Set(expanded.value)
  next.has(k) ? next.delete(k) : next.add(k)
  expanded.value = next
}

// 手机端按 phoneCols 折叠，平板/未配置则全展示
function isFolded(col: Column): boolean {
  if (!isPhone.value || !props.phoneCols?.length) return false
  if (col.colKey === opKey.value) return false
  return !props.phoneCols.includes(col.colKey)
}

function shownCols(row: Record<string, any>): Column[] {
  const open = expanded.value.has(rowKeyOf(row, dataIndex(row)))
  return visibleCols.value.filter((c) => open || !isFolded(c))
}

function dataIndex(row: Record<string, any>): number {
  return (props.data ?? []).indexOf(row)
}

function foldedCount(row: Record<string, any>): number {
  if (expanded.value.has(rowKeyOf(row, dataIndex(row)))) return 0
  return visibleCols.value.filter(isFolded).length
}

// 操作列沉底，其余按原顺序
const visibleCols = computed(() => {
  const cols = props.columns ?? []
  const op = cols.filter((c) => c.colKey === opKey.value)
  const rest = cols.filter((c) => c.colKey !== opKey.value)
  return [...rest, ...op]
})

function cellText(col: Column, row: Record<string, any>): string {
  if (typeof col.cell === 'function') {
    const v = col.cell(null, { row })
    if (v == null || typeof v === 'object') return String(row[col.colKey] ?? '-')
    return String(v)
  }
  const v = row[col.colKey]
  return v == null || v === '' ? '-' : String(v)
}
</script>

<style>
/* 统一表格质感：单元格边框降为弱描边，行悬停仅背景提亮 */
.c-table .t-table__cell {
  transition: background-color 0.2s ease-out;
}

/* 移动端卡片列表 */
.c-cards {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
/* 手机端：卡片区占满剩余高度并内部滚动，页头/分页钉住 */
.c-cards.is-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.c-cards-loading,
.c-cards-empty {
  padding: 32px 0;
  text-align: center;
  color: var(--td-text-color-placeholder);
}
.c-card-row {
  border: 1px solid var(--td-component-border);
  border-radius: 10px;
  padding: 10px 12px;
  background: var(--td-bg-color-container);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.c-card-field {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}
.c-card-label {
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
}
.c-card-value {
  text-align: right;
  min-width: 0;
  word-break: break-all;
}
.c-card-field.is-op {
  border-top: 1px solid var(--td-component-border);
  padding-top: 8px;
  margin-top: 2px;
  justify-content: flex-end;
}
.c-card-more {
  align-self: flex-start;
  margin-top: 2px;
}
</style>
