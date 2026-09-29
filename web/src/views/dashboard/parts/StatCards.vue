<!-- StatCards — 概览顶部统计卡。手机端每行两个，桌面/平板一行六个。 -->
<template>
  <t-row :gutter="[16, 16]">
    <t-col v-for="c in cards" :key="c.label" class="stat-col" :span="isPhone ? 6 : 2">
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
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { CCard } from '@/components/base'
import { useIsMobile } from '@/composables'

export interface StatCard {
  label: string
  value: string | number
  icon: Component
  bg: string
  fg: string
}

defineProps<{ cards: StatCard[] }>()

const { isPhone } = useIsMobile()
</script>

<style scoped>
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
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

/* 手机端：卡内元素整体缩小，两列排布不拥挤 */
@media (max-width: 767px) {
  .stat-inner {
    gap: 10px;
  }
  .stat-icon {
    width: 34px;
    height: 34px;
    border-radius: 9px;
    font-size: 16px;
  }
  .stat-value {
    font-size: 19px;
  }
  .stat-label {
    margin-top: 1px;
    font-size: 11px;
  }
}
</style>
