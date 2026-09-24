<template>
  <div class="page">
    <page-header>
      
      <t-button theme="primary" @click="openCreate">{{ $t('routes.create') }}</t-button>
    </page-header>
    <c-table row-key="ID" :data="routes" :columns="columns" :loading="loading">
      <template #strategy="{ row }">
        <t-tag variant="light">{{ dict(strategyDict, row.Strategy) }}</t-tag>
      </template>
      <template #groups="{ row }">
        <div class="group-cell">
          <t-tag v-for="(g, i) in visibleGroups(row)" :key="i" theme="primary" variant="light-outline">
            {{ groupLabel(g) }}
          </t-tag>
          <t-link v-if="parseGroups(row.GroupsJSON).length > GROUP_FOLD" theme="primary" size="small" @click="toggleExpand(row.ID)">
            {{ expanded.has(row.ID) ? $t('common.collapse') : $t('common.moreItems', { n: parseGroups(row.GroupsJSON).length - GROUP_FOLD }) }}
          </t-link>
        </div>
      </template>
      <template #timeout="{ row }">
        {{ (row.FirstEventTimeoutSeconds > 0 ? row.FirstEventTimeoutSeconds + 's' : $t('routes.global')) }} /
        {{ (row.FirstTokenTimeoutSeconds > 0 ? row.FirstTokenTimeoutSeconds + 's' : $t('routes.global')) }}
      </template>
      <template #failover="{ row }">
        <t-tag v-if="!row.FailoverEnabled" theme="default" variant="light">{{ $t('routes.off') }}</t-tag>
        <t-tag v-else theme="warning" variant="light-outline">
          {{ failoverLabel(row) }}
        </t-tag>
      </template>
      <template #op="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="openEdit(row)">{{ $t('routes.edit') }}</t-link>
          <t-popconfirm :content="$t('routes.confirmDelete')" @confirm="remove(row.ID)">
            <t-link theme="danger">{{ $t('common.delete') }}</t-link>
          </t-popconfirm>
        </t-space>
      </template>
    </c-table>

    <c-dialog v-model:visible="dialogVisible" :header="editingID ? $t('routes.editTitle') : $t('routes.create')" width="760px" :confirm-btn="{ loading: saving }" @confirm="save">
      <t-form label-width="90px">
        <t-form-item :label="$t('routes.name')" mark>
          <t-input v-model="form.name" :placeholder="$t('routes.namePh')" />
        </t-form-item>
        <t-form-item :label="$t('routes.strategy')">
          <t-radio-group v-model="form.strategy" variant="default-filled">
            <t-radio-button value="round_robin">{{ dict(strategyDict, 'round_robin') }}</t-radio-button>
            <t-radio-button value="random">{{ dict(strategyDict, 'random') }}</t-radio-button>
            <t-radio-button value="least_used">{{ dict(strategyDict, 'least_used') }}</t-radio-button>
            <t-radio-button value="sticky">{{ dict(strategyDict, 'sticky') }}</t-radio-button>
          </t-radio-group>
        </t-form-item>
        <t-form-item :label="$t('routes.groupMapping')" mark>
          <div class="entries">
            <div v-for="(e, i) in form.groups" :key="i" class="entry">
              <bind-select v-model="e.group_id" :multiple="false" :options="groupOptions" :placeholder="$t('routes.groupPh')" style="width: 160px" @update:model-value="loadGroupModels(e.group_id)" />
              <!-- 模型：下拉取分组账号模型并集，也可手动输入 -->
              <t-select
                v-model="e.model"
                filterable
                creatable
                clearable
                :options="modelOptions(e.group_id)"
                :placeholder="$t('routes.modelPh')"
                style="flex: 1"
              />
              <t-input-number v-model="e.weight" :min="0" :max="100" theme="column" style="width: 110px" :placeholder="$t('routes.weightPh')" />
              <t-link theme="danger" @click="form.groups.splice(i, 1)">{{ $t('routes.removeEntry') }}</t-link>
            </div>
            <div class="entry-foot">
              <t-link theme="primary" @click="form.groups.push({ group_id: undefined, weight: 0, model: '' })">{{ $t('routes.addGroup') }}</t-link>
              <span class="hint" :class="{ bad: weightSum !== 100 }">{{ $t('routes.weightSum', { n: weightSum }) }}</span>
            </div>
          </div>
        </t-form-item>
        <t-form-item :label="$t('routes.firstEventTimeout')">
          <t-input-number v-model="form.first_event_timeout_seconds" :min="0" :max="3600" theme="column" style="width: 140px" />
          <span class="hint">{{ $t('routes.timeoutHint') }}</span>
        </t-form-item>
        <t-form-item :label="$t('routes.firstTokenTimeout')">
          <t-input-number v-model="form.first_token_timeout_seconds" :min="0" :max="3600" theme="column" style="width: 140px" />
          <span class="hint">{{ $t('routes.timeoutHint') }}</span>
        </t-form-item>
        <t-form-item :label="$t('routes.userAgent')" :help="$t('routes.userAgentHint')">
          <t-input v-model="form.user_agent" :placeholder="$t('routes.userAgentPh')" />
        </t-form-item>
        <t-form-item :label="$t('routes.failover')">
          <t-switch v-model="form.failover_enabled" />
          <span class="hint">{{ $t('routes.failoverHint') }}</span>
        </t-form-item>
        <template v-if="form.failover_enabled">
          <t-form-item :label="$t('routes.triggerCond')" mark>
            <t-checkbox-group v-model="form.failover_codes">
              <t-checkbox value="4xx">{{ $t('routes.cond4xx') }}</t-checkbox>
              <t-checkbox value="5xx">{{ $t('routes.cond5xx') }}</t-checkbox>
            </t-checkbox-group>
          </t-form-item>
          <t-form-item :label="$t('routes.failoverGroup')" mark>
            <bind-select v-model="form.failover_group_id" :multiple="false" :options="groupOptions" :placeholder="$t('routes.pickGroup')" style="width: 240px" />
          </t-form-item>
          <t-form-item :label="$t('routes.failoverModel')" mark>
            <t-input v-model="form.failover_model" :placeholder="$t('routes.failoverModelPh')" style="width: 360px" />
          </t-form-item>
        </template>
      </t-form>
    </c-dialog>
  </div>
</template>

<script setup lang="ts">
import { CDialog } from '../../components/base'
import { CCard, CTable } from '../../components/base'
import PageHeader from '../../components/PageHeader.vue'
import { useAsync } from '../../composables'
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { groupApi, routeApi } from '../../api/entities'
import BindSelect from '../../components/BindSelect.vue'
import { dict, strategyDict } from '../../utils/dict'
import type { GroupInfo, RouteGroupEntry, RouteInfo } from '../../api/types'

const { t } = useI18n()

const routes = ref<RouteInfo[]>([])
const groups = ref<GroupInfo[]>([])
const dialogVisible = ref(false)
const saving = ref(false)
const editingID = ref(0)
const form = reactive({
  name: '',
  strategy: 'round_robin',
  groups: [{ group_id: undefined, weight: 100, model: '' }] as RouteGroupEntry[],
  first_event_timeout_seconds: 0,
  first_token_timeout_seconds: 0,
  user_agent: '',
  failover_enabled: false,
  failover_codes: [] as string[],
  failover_group_id: null as number | null,
  failover_model: '',
})

const columns = computed(() => [
  { colKey: 'ID', title: t('common.colId'), width: 70 },
  { colKey: 'Name', title: t('routes.colName'), align: 'center' },
  { colKey: 'strategy', title: t('routes.colStrategy'), width: 110, align: 'center' },
  { colKey: 'groups', title: t('routes.colGroups'), align: 'center' },
  { colKey: 'timeout', title: t('routes.colTimeout'), width: 90, align: 'center' },
  { colKey: 'failover', title: t('routes.colFailover'), width: 220, align: 'center' },
  { colKey: 'op', title: t('common.colOp'), width: 130, align: 'center' },
])

const groupOptions = computed(() =>
  groups.value.map((g) => ({ value: g.id, label: `${g.name} (${g.plugin_label || g.plugin})` })),
)

// 分组 → 账号模型并集（按需拉取并缓存）
const groupModelsCache = ref<Record<number, string[]>>({})
async function loadGroupModels(groupID: number | null | undefined) {
  if (!groupID || groupModelsCache.value[groupID]) return
  const resp = await groupApi.models(groupID).catch(() => ({ models: [] }))
  groupModelsCache.value = { ...groupModelsCache.value, [groupID]: resp.models ?? [] }
}
function modelOptions(groupID: number | null | undefined) {
  return (groupID ? groupModelsCache.value[groupID] ?? [] : []).map((m) => ({ value: m, label: m }))
}

const weightSum = computed(() => form.groups.reduce((s, e) => s + (Number(e.weight) || 0), 0))

function parseGroups(json: string): RouteGroupEntry[] {
  try { return JSON.parse(json) } catch { return [] }
}

// 分组列默认只展示前 GROUP_FOLD 条，其余点「更多」展开
const GROUP_FOLD = 3
const expanded = ref(new Set<number>())

function visibleGroups(row: RouteInfo): RouteGroupEntry[] {
  const all = parseGroups(row.GroupsJSON)
  return expanded.value.has(row.ID) ? all : all.slice(0, GROUP_FOLD)
}

function toggleExpand(id: number) {
  const next = new Set(expanded.value)
  next.has(id) ? next.delete(id) : next.add(id)
  expanded.value = next
}

function groupLabel(e: RouteGroupEntry): string {
  const g = groups.value.find((x) => x.id === e.group_id)
  return `${g?.name ?? e.group_id} → ${e.model} (${e.weight})`
}

function pluginName(g: GroupInfo): string {
  return g.plugin_label || g.plugin || `#${g.plugin_id}`
}

function failoverLabel(row: RouteInfo): string {
  const codes = [row.FailoverOn4xx && '4xx', row.FailoverOn5xx && '5xx'].filter(Boolean).join('/')
  const g = groups.value.find((x) => x.id === row.FailoverGroupID)
  return `${codes} → ${g?.name ?? row.FailoverGroupID}/${row.FailoverModel}`
}

function openCreate() {
  editingID.value = 0
  Object.assign(form, {
    name: '', strategy: 'round_robin',
    groups: [{ group_id: undefined, weight: 100, model: '' }],
    first_event_timeout_seconds: 0, first_token_timeout_seconds: 0, user_agent: '', failover_enabled: false, failover_codes: [], failover_group_id: null, failover_model: '',
  })
  dialogVisible.value = true
}

function openEdit(row: RouteInfo) {
  editingID.value = row.ID
  const parsed = parseGroups(row.GroupsJSON)
  Object.assign(form, {
    name: row.Name,
    strategy: row.Strategy,
    groups: parsed.length ? parsed : [{ group_id: undefined, weight: 100, model: '' }],
    first_event_timeout_seconds: row.FirstEventTimeoutSeconds,
    first_token_timeout_seconds: row.FirstTokenTimeoutSeconds,
    user_agent: row.UserAgent ?? '',
    failover_enabled: row.FailoverEnabled,
    failover_codes: [row.FailoverOn4xx && '4xx', row.FailoverOn5xx && '5xx'].filter(Boolean) as string[],
    failover_group_id: row.FailoverGroupID,
    failover_model: row.FailoverModel,
  })
  parsed.forEach((e) => loadGroupModels(e.group_id))
  dialogVisible.value = true
}

function validate(): boolean {
  if (!form.name || !form.groups.length || form.groups.some((e) => !e.group_id || !e.model)) {
    MessagePlugin.warning(t('routes.errForm'))
    return false
  }
  if (form.groups.some((e) => e.weight < 0 || e.weight > 100) || weightSum.value !== 100) {
    MessagePlugin.warning(t('routes.errWeight', { n: weightSum.value }))
    return false
  }
  if (form.failover_enabled) {
    if (!form.failover_codes.length) {
      MessagePlugin.warning(t('routes.errCodes'))
      return false
    }
    if (!form.failover_group_id || !form.failover_model) {
      MessagePlugin.warning(t('routes.errFailover'))
      return false
    }
  }
  return true
}

async function save() {
  if (!validate()) return
  saving.value = true
  const body = {
    name: form.name,
    strategy: form.strategy,
    groups: form.groups,
    first_event_timeout_seconds: form.first_event_timeout_seconds,
    first_token_timeout_seconds: form.first_token_timeout_seconds,
    user_agent: form.user_agent.trim(),
    failover_enabled: form.failover_enabled,
    failover_on_4xx: form.failover_codes.includes('4xx'),
    failover_on_5xx: form.failover_codes.includes('5xx'),
    failover_group_id: form.failover_enabled ? form.failover_group_id : null,
    failover_model: form.failover_enabled ? form.failover_model : '',
  }
  try {
    if (editingID.value) {
      await routeApi.update(editingID.value, body)
      MessagePlugin.success(t('common.saved'))
    } else {
      await routeApi.create(body)
      MessagePlugin.success(t('common.created'))
    }
    dialogVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e.message)
  } finally {
    saving.value = false
  }
}

const { loading, run } = useAsync()

async function load() {
  await run(async () => {
    const [r, g] = await Promise.all([routeApi.list(), groupApi.list()])
    routes.value = r.routes ?? []
    groups.value = g.groups ?? []
  })
}

async function remove(id: number) {
  await routeApi.remove(id)
  await load()
}

onMounted(load)
</script>

<style scoped>
.entries { width: 100% }
.entry { display: flex; gap: 8px; align-items: center; margin-bottom: 8px }
.entry-foot { display: flex; align-items: center; gap: 12px }
.hint { margin-left: 8px; color: var(--td-text-color-placeholder); font-size: 12px }
.hint.bad { color: var(--td-error-color) }
.group-cell { display: flex; flex-direction: column; align-items: center; gap: 4px }
</style>
