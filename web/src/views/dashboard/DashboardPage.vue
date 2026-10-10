<template>
  <component :is="gateway.dashboard" v-if="gateway.dashboard && hasCapability('gateway')" />
  <task-dashboard v-else />
</template>
<script setup lang="ts">
import { gateway } from '@profile'
import { hasCapability } from '@/features/access'
import TaskDashboard from './TaskDashboard.vue'
</script>
