<template>
  <div class="connection-page page">
    <c-card>
      <page-header><template #left><h2>{{ $t(supportsConnections ? 'connections.title' : 'connections.serviceUnavailable') }}</h2></template></page-header>
      <p>{{ $t(supportsConnections ? 'connections.backendUnavailable' : 'connections.serviceUnavailableHint') }}</p>
      <t-alert v-if="applicationError" theme="error" :message="applicationError" />
      <t-button variant="outline" @click="$router.replace('/')">{{ $t('common.refresh') }}</t-button>
    </c-card>
  </div>
</template>
<script setup lang="ts">
import { CCard, PageHeader } from '@/components'
import { supportsConnections } from '@/api/connections'
import { applicationError } from '@/api/application'
</script>
<style scoped>
.connection-page { display: flex; justify-content: center; align-items: center; height: 100%; padding: 20px; box-sizing: border-box; }
.connection-page p { color: var(--td-text-color-secondary); }
</style>
