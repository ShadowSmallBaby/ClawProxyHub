<template>
  <!-- 实例新建/编辑：名称 + 地址固定，其余按插件 instance_schema 动态渲染；
       新建且传入 plugins 时在弹窗内选插件（仅多实例插件） -->
  <c-drawer
    :visible="visible"
    :header="instance ? $t('instances.editTitle') : $t('instances.add')"
    :confirm-btn="{ loading: saving }"
    width="640px"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    @confirm="submit"
  >
    <t-form>
      <form-item :label="$t('instances.colPlugin')" :mark="true">
        <t-select v-if="!instance && plugins?.length" v-model="pluginId" :placeholder="$t('instances.pickPlugin')" style="width: 100%">
          <t-option v-for="p in plugins" :key="p.id" :value="p.id" :label="p.label || p.name" />
        </t-select>
        <t-input v-else :value="current?.label || current?.name || ''" disabled />
      </form-item>
      <form-item v-if="!isDefaultInstance" :label="$t('instances.colName')" :mark="true">
        <t-input v-model="form.name" :placeholder="$t('instances.namePh')" />
      </form-item>
      <form-item v-if="!isDefaultInstance" :label="$t('instances.colBaseUrl')" :mark="true">
        <t-input-adornment class="url-adornment">
          <template #prepend>
            <t-select v-model="form.scheme" auto-width :options="[{ value: 'https://', label: 'https://' }, { value: 'http://', label: 'http://' }]" />
          </template>
          <t-input v-model="form.host" placeholder="api.example.com" @blur="normalizeHost" />
        </t-input-adornment>
      </form-item>
      <template v-for="f in schemaFields" :key="f.key">
        <form-item v-if="fieldVisible(f)" :label="f.title">
          <t-switch v-if="f.type === 'boolean'" v-model="form.settings[f.key]" />
          <t-select v-else-if="f.options?.length" v-model="form.settings[f.key]" clearable :placeholder="f.description" style="width: 100%">
            <t-option v-for="o in f.options" :key="String(o.value)" :value="o.value" :label="o.label" />
          </t-select>
          <t-input-number v-else-if="f.type === 'number'" v-model="form.settings[f.key]" theme="column" :placeholder="f.description" style="width: 100%" />
          <t-input v-else v-model="form.settings[f.key]" :placeholder="f.description || (f.default ? $t('plugins.phDefault', { d: f.default }) : '')" />
        </form-item>
      </template>
    </t-form>
  </c-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { CDrawer } from './base'
import FormItem from './FormItem.vue'
import { instanceApi } from '@/api/entities'
import type { InstanceInfo, PluginInfo } from '@/api/types'

const props = defineProps<{
  visible: boolean
  plugin: PluginInfo | null // 固定插件（编辑 / 插件卡片入口）
  plugins?: PluginInfo[] // 新建时可选插件列表（实例页入口）
  instance: InstanceInfo | null // null = 新建
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void }>()

const { t } = useI18n()
const saving = ref(false)
const pluginId = ref<number | undefined>(undefined) // t-select 空值用 undefined
const form = reactive<{ name: string; scheme: string; host: string; settings: Record<string, any> }>({ name: '', scheme: 'https://', host: '', settings: {} })

// 当前生效插件：固定传入优先，否则按弹窗内选择
const current = computed(() => props.plugin ?? props.plugins?.find((p) => p.id === pluginId.value) ?? null)

// 默认实例（编辑单实例插件）：名称与站点地址系统固定，弹窗内不展示、不校验，仅保存动态设置
const isDefaultInstance = computed(() => !!props.instance && !!current.value && !current.value.multi_instance)

// 打开时回填（base_url 拆成协议 + host）；有默认值的下拉字段缺省时落默认值
watch(() => props.visible, (v) => {
  if (!v) return
  pluginId.value = props.plugin?.id ?? props.plugins?.[0]?.id
  form.name = props.instance?.name ?? ''
  const m = /^(https?:\/\/)(.*)$/.exec(props.instance?.base_url ?? '')
  form.scheme = m?.[1] ?? 'https://'
  form.host = m?.[2] ?? ''
  form.settings = { ...(props.instance?.settings ?? {}) }
  for (const f of schemaFields.value) {
    if (f.default !== '' && form.settings[f.key] === undefined) form.settings[f.key] = f.default
  }
})

// host 清洗：去空格、剥离误粘的协议头、去尾随斜线
function normalizeHost() {
  let h = form.host.replace(/\s+/g, '')
  const m = /^(https?:\/\/)(.*)$/i.exec(h)
  if (m) {
    form.scheme = m[1].toLowerCase()
    h = m[2]
  }
  form.host = h.replace(/\/+$/, '')
}
const baseURL = computed(() => form.scheme + form.host)

// 下拉项：enum（值即标签）或 oneOf（{const, title} 带标签）；x-depends 声明依赖其他字段取值时才显示
interface SchemaOption { value: unknown; label: string }
interface SchemaField { key: string; title: string; description: string; type: string; default: unknown; options: SchemaOption[]; depends: Record<string, unknown> | null }
const schemaFields = computed<SchemaField[]>(() => {
  const raw = current.value?.instance_schema
  if (!raw) return []
  try {
    const propsDef = JSON.parse(raw)?.properties ?? {}
    return Object.entries(propsDef).map(([key, def]: [string, any]) => {
      const options: SchemaOption[] = def.oneOf?.length
        ? def.oneOf.map((o: any) => ({ value: o.const, label: o.title ?? String(o.const) }))
        : (def.enum ?? []).map((v: unknown) => ({ value: v, label: String(v) }))
      return {
        key, title: def.title ?? key, description: def.description ?? '',
        type: def.type ?? 'string', default: def.default ?? '', options, depends: def['x-depends'] ?? null,
      }
    })
  } catch {
    return []
  }
})
function fieldVisible(f: SchemaField): boolean {
  if (!f.depends) return true
  return Object.entries(f.depends).every(([k, v]) => form.settings[k] === v)
}

async function submit() {
  if (!current.value) {
    MessagePlugin.warning(t('instances.pickPlugin'))
    return
  }
  // 默认实例：名称/地址系统固定，跳过校验并沿用原值，只保存动态设置
  if (!isDefaultInstance.value) {
    if (!form.name.trim()) {
      MessagePlugin.warning(t('instances.nameRequired'))
      return
    }
    normalizeHost()
    if (!form.host) {
      MessagePlugin.warning(t('instances.baseUrlRequired'))
      return
    }
    // host 格式：域名（含 .）或 IP[:port]（含 . 或为 localhost）；纯数字/单词字母拦下
    const h = form.host
    const hasPort = /:[0-9]{1,5}$/.test(h)
    const hostPart = hasPort ? h.slice(0, h.lastIndexOf(':')) : h
    const isDomain = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/.test(hostPart)
    const isIPv4 = /^(\d{1,3}\.){3}\d{1,3}$/.test(hostPart)
    const isLocalhost = hostPart === 'localhost'
    if (!isDomain && !isIPv4 && !isLocalhost) {
      MessagePlugin.warning(t('instances.baseUrlInvalid'))
      return
    }
  }
  saving.value = true
  try {
    const body = {
      plugin_id: current.value.id,
      name: isDefaultInstance.value ? (props.instance?.name ?? form.name) : form.name,
      base_url: isDefaultInstance.value ? (props.instance?.base_url ?? '') : baseURL.value,
      settings: form.settings,
    }
    if (props.instance) await instanceApi.update(props.instance.id, body)
    else await instanceApi.create(body)
    MessagePlugin.success(t('common.saved'))
    emit('update:visible', false)
    emit('saved')
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
/* 地址输入：协议下拉 + host 顶格铺满整行 */
.url-adornment { width: 100%; display: flex; }
.url-adornment :deep(.t-input-adornment__prepend) { margin: 0; }
.url-adornment :deep(.t-input) { flex: 1; }
</style>
