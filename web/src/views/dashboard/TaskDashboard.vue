<template>
  <page-layout class="page">
    <stat-cards :cards="cards" />
    <c-card :header="$t('dashboard.channelTitle')" :bordered="false" class="task-quota">
      <quota-carousel :items="quota" :fmt="fmtNum" />
    </c-card>
  </page-layout>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AppIcon, UserIcon } from 'tdesign-icons-vue-next'
import { PageLayout, CCard } from '@/components'
import { statsApi, type QuotaPlugin } from '@/api/stats'
import type { Stats } from '@/api/types'
import { fmtNum } from '@/utils/format'
import { useAsync } from '@/composables'
import StatCards from './parts/StatCards.vue'
import QuotaCarousel from './parts/QuotaCarousel.vue'
const { t } = useI18n()
const { run } = useAsync()
const stats = ref<Stats>()
const quota = ref<QuotaPlugin[]>([])
const cards = computed(() => [
  { label: t('dashboard.activeAccounts'), value: stats.value?.active_accounts ?? '-', icon: UserIcon, bg: 'var(--td-brand-color-1)', fg: 'var(--td-brand-color)' },
  { label: t('dashboard.runningPlugins'), value: stats.value?.running_plugins ?? '-', icon: AppIcon, bg: 'var(--td-brand-color-1)', fg: 'var(--td-brand-color)' },
])
onMounted(() => run(async () => {
  const [s, q] = await Promise.all([statsApi.get(), statsApi.quota()])
  stats.value = s
  quota.value = q.plugins ?? []
}))
</script>
<style scoped>
.task-quota { margin-top: 16px; }
</style>
