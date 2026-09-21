<!-- PageHeader — 页头：左侧留空（标题由 Layout 头部承担），右侧放操作按钮。 -->
<template>
  <div class="page-header">
    <div class="page-header-left"><slot name="left" /></div>
    <div class="page-header-right"><slot /></div>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.page-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.page-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
</style>
