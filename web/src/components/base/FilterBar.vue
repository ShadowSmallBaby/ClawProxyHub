<!-- FilterBar — PC 筛选条：flex-wrap 自动换行，窄视口不溢出（移动端筛选走各自页面的 c-drawer）。 -->
<template>
  <div class="filter-bar">
    <slot />
  </div>
</template>

<script setup lang="ts"></script>

<style scoped>
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
  /* 压住 min-width:auto：允许收缩触发内部折行 */
  min-width: 0;
  flex: 1;
}
/* 子项默认弹性收窄：写死宽度可换行，未写宽度的输入自动伸缩 */
.filter-bar > :deep(.t-input),
.filter-bar > :deep(.t-select),
.filter-bar > :deep(.t-date-range-picker) {
  max-width: 100%;
}
/* 筛选条内固定宽窄视口转弹性：1024px 以下一行放不下时自动伸缩 */
@media (max-width: 1024px) {
  .filter-bar > .w-xs, .filter-bar > .w-sm, .filter-bar > .w-md,
  .filter-bar > .w-lg, .filter-bar > .w-xl {
    width: auto !important;
    flex: 1 1 180px;
  }
}
</style>
