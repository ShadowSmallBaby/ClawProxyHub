<!-- QuotaCarousel — 渠道积分。PC 每页八个（2×4）翻页；手机端每页一个，左右滑。 -->
<template>
  <div v-if="items.length" class="carousel">
    <div class="quota-grid">
      <div v-for="p in pageItems" :key="p.plugin + '/' + p.instance" class="quota-card">
        <div class="quota-head">
          <span class="quota-plugin">{{ p.label || p.plugin }}</span>
          <span class="quota-accounts">{{ $t('dashboard.accountsN', { n: p.accounts }) }}</span>
        </div>
        <div class="quota-row">
          <span class="quota-key">{{ $t('dashboard.usedCredits') }}</span>
          <span class="quota-value">{{ fmt(p.quota.used_credits) }}</span>
        </div>
        <div class="quota-row">
          <span class="quota-key">{{ $t('dashboard.remainingCredits') }}</span>
          <span class="quota-value">{{ fmt(p.quota.credits) }}</span>
        </div>
        <div class="quota-row">
          <span class="quota-key">{{ $t('dashboard.totalCredits') }}</span>
          <span class="quota-value">{{ fmt(p.quota.total_credits) }}</span>
        </div>
      </div>
    </div>

    <div v-if="pages > 1" class="carousel-nav">
      <t-button variant="text" shape="square" :disabled="page === 0" @click="page--">
        <template #icon><chevron-left-icon /></template>
      </t-button>
      <div class="dots">
        <span v-for="i in pages" :key="i" class="dot" :class="{ on: i - 1 === page }" @click="page = i - 1" />
      </div>
      <t-button variant="text" shape="square" :disabled="page === pages - 1" @click="page++">
        <template #icon><chevron-right-icon /></template>
      </t-button>
    </div>
  </div>
  <t-empty v-else :description="$t('dashboard.noQuotaData')" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronLeftIcon, ChevronRightIcon } from 'tdesign-icons-vue-next'
import { useIsMobile } from '@/composables'

export interface QuotaItem {
  plugin: string
  instance?: string
  label?: string
  accounts: number
  quota: Record<string, number>
}

const props = defineProps<{
  items: QuotaItem[]
  fmt: (n: number) => string
}>()

const { isPhone } = useIsMobile()

// 手机每页 1 个，其余每页 8 个
const perPage = computed(() => (isPhone.value ? 1 : 8))
const pages = computed(() => Math.max(1, Math.ceil(props.items.length / perPage.value)))
const page = ref(0)

// 数据或每页数变化时把页码夹回合法范围
watch([() => props.items.length, perPage], () => {
  if (page.value > pages.value - 1) page.value = pages.value - 1
})

const pageItems = computed(() => {
  const start = page.value * perPage.value
  return props.items.slice(start, start + perPage.value)
})
</script>

<style scoped>
.quota-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
@media (max-width: 1023px) {
  .quota-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 767px) {
  .quota-grid {
    grid-template-columns: 1fr;
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
.carousel-nav {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 12px;
}
.dots {
  display: flex;
  gap: 6px;
}
.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--td-component-border);
  cursor: pointer;
}
.dot.on {
  background: var(--td-brand-color);
}
</style>
