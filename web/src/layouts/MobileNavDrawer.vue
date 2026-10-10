<!-- MobileNavDrawer — 移动端导航抽屉：品牌区 + 菜单 + 功能区 footer，整页可拖拽。 -->
<template>
  <t-drawer
    :visible="visible"
    placement="left"
    :header="false"
    size="240px"
    class="mobile-drawer"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <div class="drawer-brand" @click="go('/dashboard')">
      <img class="drawer-logo" :src="brandLogo" :alt="branding.name" />
      <span class="drawer-name" :class="{ custom: brandCustom }">
        <template v-if="brandCustom">{{ branding.name }}</template>
        <template v-else>Claw<span>ProxyHub</span></template>
      </span>
    </div>
    <div class="drawer-menu">
      <t-menu :value="route.path" width="240px" @change="go">
        <t-menu-item v-for="item in items" :key="item.value" :value="item.value">
          <template #icon><component :is="item.icon" /></template>{{ $t(item.label) }}
        </t-menu-item>
      </t-menu>
    </div>
    <template #footer>
      <action-icons v-if="!isAndroidApp" compact />
    </template>
  </t-drawer>
</template>

<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { branding, brandLogo, brandCustom } from '@/utils/branding'
import ActionIcons from './ActionIcons.vue'
import { isAndroidApp } from '@/api/application'
import type { MenuItem } from './types'

defineProps<{
  visible: boolean
  items: MenuItem[]
}>()

const emit = defineEmits<{
  'update:visible': [v: boolean]
}>()

const route = useRoute()
const router = useRouter()

function go(v: string) {
  emit('update:visible', false)
  router.push(v)
}

</script>

<style scoped>
/* 抽屉结构：品牌区 + 菜单 + footer 自然排布，整页可拖拽滚动 */
.drawer-brand {
  height: 64px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  cursor: pointer;
  color: var(--td-brand-color);
  font-size: 18px;
  font-weight: 700;
  flex-shrink: 0;
  border-bottom: 1px solid var(--td-component-border);
}
.drawer-logo {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--td-brand-color-3);
  object-fit: cover;
  flex-shrink: 0;
}
.drawer-name {
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.drawer-name.custom {
  font-size: 16px;
}
.drawer-name span {
  font-weight: 300;
  opacity: 0.8;
  margin-left: 2px;
}
.drawer-menu {
  padding-bottom: 8px;
}
</style>
