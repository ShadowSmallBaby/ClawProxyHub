<template>
  <page-layout :scroll="false" class="editor-page">
    <template #header>
    <div class="editor-topbar">
      <t-button variant="text" shape="square" @click="goBack">
        <template #icon><arrow-left-icon /></template>
      </t-button>
      <span class="editor-title">{{ title }}</span>
      <span class="editor-sub">main.lua</span>
      <t-space style="margin-left: auto">
        <t-button theme="primary" :loading="saving" @click="save">{{ $t('plugins.editorSave') }}</t-button>
      </t-space>
    </div>
    </template>
    <code-editor v-model="lua" height="100%" />
    <template #overlays>

    <!-- 保存表单：新建时填写插件名/显示名/icon，确认后创建 -->
    <c-drawer
      v-model:visible="formVisible"
      :header="t('plugins.editorCreate')"
      :confirm-btn="{ loading: creating }"
      @confirm="confirmCreate"
    >
      <t-form label-width="110px">
        <form-item :label="t('plugins.editorName')" :mark="true">
          <t-input :value="luaName" readonly :status="luaName ? undefined : 'error'" :placeholder="t('plugins.editorNameFromCode')" />
        </form-item>
        <form-item :label="t('plugins.editorLabel')">
          <t-input :value="luaLabel" readonly :placeholder="t('plugins.editorLabelFromCode')" />
        </form-item>
        <form-item :label="t('plugins.editorIcon')">
          <div class="icon-upload" @click="pickIcon" @dragover.prevent @drop.prevent="onDropIcon">
            <img v-if="iconUrl" :src="iconUrl" class="icon-preview" />
            <t-icon v-else name="image-add" class="icon-plus" />
            <span class="icon-hint">{{ iconFile ? iconFile.name : t('plugins.editorIconHint') }}</span>
          </div>
          <input ref="iconInputRef" type="file" accept="image/png,image/jpeg,image/webp" style="display: none" @change="onPickIcon" />
        </form-item>
      </t-form>
      <t-alert theme="info" :message="t('plugins.editorCreateHint')" style="margin-top: 12px" />
    </c-drawer>
    </template>
  </page-layout>
</template>

<script setup lang="ts">
import { PageLayout } from '@/components'
// 全屏 Lua 插件编辑页：编辑态保存即重载；新建态保存弹建档表单（名称/显示名由代码侧
// PLUGIN_NAME/PLUGIN_LABEL 回显只读，仅 icon 可编辑）后创建。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { ArrowLeftIcon } from 'tdesign-icons-vue-next'
import { CDrawer } from '@/components/base'
import FormItem from '@/components/FormItem.vue'
import CodeEditor from '@/components/CodeEditor.vue'
import { pluginApi } from '@/api/entities'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()

// route.params.name 存在 = 编辑态；缺省 = 新建态（先写码后建档）
const name = computed(() => (route.params.name as string) || '')
const title = computed(() => (name.value ? `${t('plugins.editorEdit')} · ${name.value}` : t('plugins.editorNew')))
const isEdit = computed(() => !!name.value)

const lua = ref('')
const saving = ref(false)
const formVisible = ref(false)
const creating = ref(false) // 建档请求进行中
const iconFile = ref<File | null>(null)
const iconUrl = ref('')
const iconInputRef = ref<HTMLInputElement>()

// 代码侧身份提取：main.lua 顶部 PLUGIN_NAME / PLUGIN_LABEL_ZH / PLUGIN_LABEL_EN
const luaName = computed(() => extractIdentity(lua.value, 'PLUGIN_NAME'))
// 显示名按当前界面语言取对应变量，缺省回退另一语言 / 插件名
const luaLabel = computed(() => {
  const zh = extractIdentity(lua.value, 'PLUGIN_LABEL_ZH')
  const en = extractIdentity(lua.value, 'PLUGIN_LABEL_EN')
  const primary = locale.value.startsWith('en') ? en : zh
  return primary || zh || en || luaName.value
})

// extractIdentity 从内容里提取 `local <key> = "value"` 的引号内值。
function extractIdentity(content: string, key: string): string {
  for (const line of content.split('\n')) {
    const m = new RegExp(`^local\\s+${key}\\s*=\\s*"([^"]*)"`)
    const hit = m.exec(line.trim())
    if (hit) return hit[1]
  }
  return ''
}

watch(
  () => route.params.name,
  async () => {
    lua.value = ''
    if (name.value) {
      try {
        const resp = await pluginApi.source(name.value, 'main.lua')
        lua.value = resp.content
      } catch (e: any) {
        MessagePlugin.error(e.message)
      }
    } else {
      try {
        const sc = await pluginApi.scaffold()
        lua.value = sc.lua
      } catch { /* 脚手架拉取失败：空白编辑 */ }
    }
  },
  { immediate: true },
)

function goBack() {
  router.push('/plugins')
}

function save() {
  if (isEdit.value) {
    doSaveSource()
    return
  }
  iconFile.value = null
  iconUrl.value = ''
  formVisible.value = true
}

// 编辑态：保存 main.lua 并重载
async function doSaveSource() {
  saving.value = true
  try {
    await pluginApi.saveSource(name.value, 'main.lua', lua.value)
    MessagePlugin.success(t('plugins.editorSaved'))
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    saving.value = false
  }
}

// 新建态：建档并创建（名称/显示名取代码侧提取值，用户编辑的 main.lua 一并落盘）
async function confirmCreate() {
  const n = luaName.value.trim()
  if (!/^[A-Za-z0-9_-]{1,32}$/.test(n)) {
    MessagePlugin.warning(t('plugins.editorNameFromCodeMissing'))
    return
  }
  // PLUGIN_LABEL_ZH 为约定必填项，缺失弹窗提示（EN 可空，缺省回退插件名）
  if (!extractIdentity(lua.value, 'PLUGIN_LABEL_ZH').trim()) {
    MessagePlugin.warning(t('plugins.editorLabelZhMissing'))
    return
  }
  creating.value = true
  try {
    await pluginApi.createLocal(n, luaLabel.value.trim(), lua.value, iconFile.value ?? undefined)
    MessagePlugin.success(t('plugins.editorCreated', { name: n }))
    formVisible.value = false
    router.replace(`/plugins/editor/${n}`)
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    creating.value = false
  }
}

function pickIcon() {
  iconInputRef.value?.click()
}

function setIcon(file: File) {
  if (!/^image\/(png|jpeg|webp)$/.test(file.type)) {
    MessagePlugin.warning(t('plugins.editorIconRule'))
    return
  }
  iconFile.value = file
  iconUrl.value = URL.createObjectURL(file)
}

function onPickIcon(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) setIcon(f)
}

function onDropIcon(e: DragEvent) {
  const f = e.dataTransfer?.files?.[0]
  if (f) setIcon(f)
}
</script>

<style scoped>
.editor-page { padding: 0; }
.editor-topbar {
  display: flex; align-items: center; gap: 10px; padding: 10px 16px;
  border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-container);
}
.editor-title { font-weight: 600; }
.editor-sub { font-size: 12px; color: var(--td-text-color-secondary); font-family: ui-monospace, monospace; }
.icon-upload {
  display: flex; align-items: center; gap: 10px; padding: 0 14px; height: 60px; min-width: 240px;
  border: 1px dashed var(--td-component-stroke); border-radius: 10px; cursor: pointer;
  color: var(--td-text-color-placeholder);
}
.icon-upload:hover { border-color: var(--td-brand-color); color: var(--td-brand-color); }
.icon-preview { width: 40px; height: 40px; flex: none; object-fit: cover; border-radius: 8px; }
.icon-plus { font-size: 24px; flex: none; }
.icon-hint { font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
