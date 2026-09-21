<!-- LangSwitch — 头部语言切换：zh / en 下拉选择。 -->
<template>
  <t-popup trigger="click">
    <t-button variant="text" shape="square" theme="default">
      <translate-icon />
    </t-button>
    <template #content>
      <div class="lang-menu">
        <div
          v-for="l in langs"
          :key="l"
          class="lang-menu-item"
          :class="{ active: locale === l }"
          @click="setLocale(l)"
        >
          {{ $t('common.' + l) }}
          <check-icon v-if="locale === l" />
        </div>
      </div>
    </template>
  </t-popup>
</template>

<script setup lang="ts">
import { CheckIcon, TranslateIcon } from 'tdesign-icons-vue-next'
import { useLocale, type Locale } from '../../composables'

const { locale, setLocale } = useLocale()
const langs: Locale[] = ['zh', 'en']
</script>

<style scoped>
/* 语言菜单 */
.lang-menu {
  min-width: 140px;
}
.lang-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  white-space: nowrap;
  border-radius: 6px;
  transition: background-color 0.2s ease-out;
}
.lang-menu-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.lang-menu-item.active {
  color: var(--td-brand-color);
}
</style>
