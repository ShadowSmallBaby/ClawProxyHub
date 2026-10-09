<template>
  <div class="panel mcp-settings">
    <t-loading :loading="loading">
      <div v-if="state" class="s-form">
        <form-item :label="t('mcp.status')">
          <t-space align="center">
            <t-switch :value="state.enabled" :disabled="loading" @change="toggle" />
            <span>{{ t(state.enabled ? 'mcp.running' : 'mcp.stopped') }}</span>
          </t-space>
        </form-item>
        <template v-if="state.enabled">
          <form-item :label="t('mcp.connection')" :tip="t('mcp.connectionHint')">
            <div class="mcp-field-row">
              <t-input :value="endpoint" class="mcp-readonly" readonly :aria-label="t('mcp.connection')" />
              <t-button variant="text" shape="square" :aria-label="t('mcp.copyAddress')" @click="copy(endpoint)"><template #icon><copy-icon /></template></t-button>
            </div>
          </form-item>
          <form-item :label="t('mcp.authentication')" :tip="t('mcp.keyHint')">
            <div class="mcp-field-row">
              <t-input :value="maskedKey" class="mcp-readonly mcp-key" readonly :placeholder="t(state.has_key ? 'mcp.keyStored' : 'mcp.noKey')" :aria-label="t('mcp.authentication')" />
              <t-button :disabled="!issuedKey" variant="text" shape="square" :aria-label="t('mcp.copyKey')" @click="copy(issuedKey)"><template #icon><copy-icon /></template></t-button>
              <t-popconfirm v-if="state.has_key" :content="t('mcp.rotateHint')" @confirm="rotate">
                <t-button :disabled="loading" variant="outline" class="mcp-key-action">{{ t('mcp.rotate') }}</t-button>
              </t-popconfirm>
              <t-button v-else :disabled="loading" variant="outline" class="mcp-key-action" @click="rotate">{{ t('mcp.create') }}</t-button>
            </div>
            <div v-if="issuedKey" class="mcp-hint">{{ t('mcp.once') }}</div>
          </form-item>
          <form-item :label="t('mcp.authorization')" :tip="t('mcp.authorizationHint')" class="mcp-authorization">
            <div class="mcp-tools" role="group" :aria-label="t('mcp.authorization')">
              <t-checkbox-group v-model="allowed">
                <t-checkbox v-for="tool in tools" :key="tool.id" :value="tool.id" :disabled="loading">
                  <span class="mcp-tool-title">{{ toolTitle(tool) }}</span>
                  <span class="mcp-tool-detail">{{ toolModule(tool) }} · {{ t('mcp.' + tool.effect) }}</span>
                  <span class="mcp-tool-detail">{{ toolDescription(tool) }}</span>
                </t-checkbox>
              </t-checkbox-group>
              <t-empty v-if="!tools.length" :description="t('mcp.noTools')" />
            </div>
          </form-item>
          <div class="save-row"><t-button theme="primary" :loading="loading" @click="save">{{ t('common.save') }}</t-button></div>
        </template>
      </div>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { CopyIcon } from 'tdesign-icons-vue-next'
import { FormItem } from '@/components'
import { useAsync } from '@/composables/useAsync'
import { mcpApi, type MCPState, type MCPTool } from '@/api/mcp'
import { copyText } from '@/utils/common'

const { t, te } = useI18n(), { run, loading } = useAsync()
const state = ref<MCPState>(), tools = ref<MCPTool[]>([]), allowed = ref<string[]>([]), endpoint = ref('')
const issuedKey = ref('')
const maskedKey = computed(() => issuedKey.value ? `${issuedKey.value.slice(0, 8)}••••••••${issuedKey.value.slice(-4)}` : '')
async function load() {
  const response = await mcpApi.config()
  state.value = response.config; tools.value = response.tools
  allowed.value = response.config.allowed_actions.filter(id => response.tools.some(tool => tool.id === id))
  endpoint.value = mcpApi.address(response.endpoint)
}
function toggle(enabled: boolean | string | number) { return run(async () => {
  await mcpApi.save(enabled === true, state.value!.allowed_actions)
  state.value!.enabled = enabled === true
  if (!state.value!.enabled) issuedKey.value = ''
}) }
function save() { return run(async () => {
  if (!state.value) return
  await mcpApi.save(state.value.enabled, allowed.value)
  state.value.allowed_actions = [...allowed.value]
  MessagePlugin.success(t('mcp.saved'))
}) }
function rotate() { return run(async () => { issuedKey.value = (await mcpApi.rotate()).key; state.value!.has_key = true }) }
async function copy(value: string) { if (await copyText(value)) MessagePlugin.success(t('mcp.copied')); else MessagePlugin.error(t('mcp.copyFailed')) }
function toolText(tool: MCPTool, part: string) { return `mcp.tools.${tool.id.replaceAll('.', '_')}.${part}` }
function toolTitle(tool: MCPTool) { const key = toolText(tool, 'title'); return te(key) ? t(key) : tool.title }
function toolDescription(tool: MCPTool) { const key = toolText(tool, 'description'); return te(key) ? t(key) : tool.id }
function toolModule(tool: MCPTool) { const key = `mcp.modules.${tool.permission.split('.')[0]}`; return te(key) ? t(key) : tool.owner || tool.permission.split('.')[0] }
onBeforeUnmount(() => { issuedKey.value = '' })
onMounted(() => run(load))
defineExpose({ save, loading })
</script>

<style scoped>
.panel { padding: 24px 28px 32px; }
.mcp-field-row { display: flex; align-items: center; gap: 8px; max-width: 640px; }
.mcp-field-row :deep(.t-input__wrap) { flex: 1; min-width: 0; }
.mcp-field-row :deep(.t-button), .mcp-field-row :deep(.t-popup__reference) { flex: none; }
.mcp-key-action { width: 108px; }
.mcp-readonly :deep(.t-input) { background: var(--td-bg-color-secondarycontainer); border-color: transparent; }
.mcp-hint { margin-top: 6px; color: var(--td-text-color-secondary); font-size: 12px; }
.mcp-authorization { align-items: flex-start; }
.mcp-tools { height: 374px; max-width: 640px; overflow-y: auto; border: 1px solid var(--td-component-stroke); border-radius: 8px; padding: 12px; box-sizing: border-box; }
.mcp-tools :deep(.t-checkbox-group) { display: flex; flex-direction: column; gap: 16px; }
.mcp-tools :deep(.t-checkbox) { align-items: flex-start; margin-right: 0; }
.mcp-tools :deep(.t-checkbox__label) { min-width: 0; white-space: normal; }
.mcp-tool-title { display: block; font-weight: 500; }
.mcp-tool-detail { display: block; font-size: 12px; color: var(--td-text-color-secondary); overflow-wrap: anywhere; }
.save-row { padding-top: 16px; }
@media (max-width: 767px) { .panel { padding: 16px 12px 112px; } .save-row { display: none; } }
</style>
