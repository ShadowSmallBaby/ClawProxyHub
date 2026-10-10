<template>
  <div class="logs-page">
    <c-tabs v-if="gateway.logs && hasCapability('gateway')" v-model="tab">
      <t-tab-panel value="requests" :label="$t('logs.reqTab')" />
      <t-tab-panel value="runs" :label="$t('logs.runTab')" />
    </c-tabs>
    <div class="logs-body">
      <component :is="gateway.logs" v-if="tab === 'requests' && gateway.logs && hasCapability('gateway')" />
      <run-logs v-else />
    </div>
  </div>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { CTabs } from '@/components'
import { gateway } from '@profile'
import { hasCapability } from '@/features/access'
import RunLogs from './RunLogs.vue'
const tab = ref(gateway.logs && hasCapability('gateway') ? 'requests' : 'runs')
</script>
<style scoped>
.logs-page { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.logs-body { flex: 1; min-height: 0; }
</style>
