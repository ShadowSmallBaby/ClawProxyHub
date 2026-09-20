<!-- BindSelect — 通用可搜索多选绑定组件（账号↔分组/代理、密钥↔路由、分组↔代理复用）。 -->
<template>
  <t-select
    :value="modelValue"
    :multiple="multiple"
    filterable
    clearable
    :placeholder="placeholder"
    :min-collapsed-num="minCollapsed"
    @change="(v: any) => emit('update:modelValue', v)"
  >
    <t-option v-for="o in options" :key="o.value" :value="o.value" :label="o.label" />
  </t-select>
</template>

<script setup lang="ts">
interface Option {
  value: number | string
  label: string
}

withDefaults(
  defineProps<{
    modelValue: Array<number | string> | number | string | null | undefined
    options: Option[]
    placeholder?: string
    multiple?: boolean
    minCollapsed?: number
  }>(),
  { multiple: true, minCollapsed: 0, placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: any): void }>()
</script>
