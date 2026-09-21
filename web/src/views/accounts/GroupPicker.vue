<!-- GroupPicker — 行内分组选择器：已选 tag 可关，＋ 弹层勾选增减（Accounts 表格复用）。 -->
<template>
  <div class="group-tags" @click.stop>
    <t-tag
      v-for="gid in modelValue ?? []"
      :key="gid"
      size="small"
      variant="light"
      closable
      @close="() => toggle(gid, false)"
    >
      {{ nameOf(gid) }}
    </t-tag>
    <t-popup trigger="click">
      <t-tag size="small" theme="default" variant="light" class="group-add">＋</t-tag>
      <template #content>
        <div class="group-picker">
          <div v-if="!groups.length" class="group-picker-empty">{{ $t('accounts.noGroups') }}</div>
          <div
            v-for="g in groups"
            :key="g.id"
            class="group-picker-item"
            :class="{ active: (modelValue ?? []).includes(g.id) }"
            @click="() => toggle(g.id, !(modelValue ?? []).includes(g.id))"
          >
            {{ g.name }}
            <check-icon v-if="(modelValue ?? []).includes(g.id)" />
          </div>
        </div>
      </template>
    </t-popup>
  </div>
</template>

<script setup lang="ts">
import { CheckIcon } from 'tdesign-icons-vue-next'
import type { GroupInfo } from '../../api/types'

const props = defineProps<{
  modelValue: number[] | null // 账号 group_ids
  groups: GroupInfo[] // 该账号所属实例的可选分组
  nameOf: (id: number) => string // 分组名映射
}>()

const emit = defineEmits<{ (e: 'update:modelValue', v: number[]): void }>()

// 单个分组增减（tag 关闭 / 弹层勾选），乐观更新由父层负责
function toggle(groupId: number, add: boolean) {
  const cur = props.modelValue ?? []
  const next = add ? [...cur, groupId] : cur.filter((id) => id !== groupId)
  emit('update:modelValue', next)
}
</script>

<style scoped>
/* 行内分组 tag：动态增减 */
.group-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}
.group-add {
  cursor: pointer;
  min-width: 22px;
  text-align: center;
}
.group-picker {
  min-width: 160px;
  max-height: 240px;
  overflow-y: auto;
}
.group-picker-empty {
  padding: 8px 12px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.group-picker-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 12px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  white-space: nowrap;
  transition: background-color 0.2s ease-out;
}
.group-picker-item:hover {
  background: var(--td-bg-color-secondarycontainer);
}
.group-picker-item.active {
  color: var(--td-brand-color);
}
</style>
