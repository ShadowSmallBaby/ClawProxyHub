<template>
  <div class="page">
    <div class="page-header">
      
      <t-button theme="primary" @click="openCreate">{{ $t('routes.create') }}</t-button>
    </div>
    <t-table row-key="ID" :data="routes" :columns="columns">
      <template #strategy="{ row }">
        <t-tag variant="light">{{ dict(strategyDict, row.Strategy) }}</t-tag>
      </template>
      <template #groups="{ row }">
        <t-space size="small">
          <t-tag v-for="(g, i) in parseGroups(row.GroupsJSON)" :key="i" theme="primary" variant="light-outline">
            {{ groupLabel(g) }}
          </t-tag>
        </t-space>
      </template>
      <template #timeout="{ row }">
        {{ row.TimeoutSeconds > 0 ? row.TimeoutSeconds + 's' : $t('routes.global') }}
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
    </t-table>

    <t-dialog v-model:visible="dialogVisible" :header="editingID ? $t('routes.editTitle') : $t('routes.create')" width="680px" :confirm-btn="{ loading: saving }" @confirm="save">
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
              <t-select v-model="e.group_id" :placeholder="$t('routes.groupPh')" style="width: 160px">
                <t-option v-for="g in groups" :key="g.id" :value="g.id" :label="`${g.name} (${g.plugin_label || g.plugin})`" />
              </t-select>
              <t-input v-model="e.model" :placeholder="$t('routes.modelPh')" style="flex: 1" />
              <t-input-number v-model="e.weight" :min="1" :max="100" theme="column" style="width: 110px" :placeholder="$t('routes.weightPh')" />
              <t-link theme="danger" @click="form.groups.splice(i, 1)">{{ $t('routes.removeEntry') }}</t-link>
            </div>
            <t-link theme="primary" @click="form.groups.push({ group_id: 0, weight: 100, model: '' })">{{ $t('routes.addGroup') }}</t-link>
          </div>
        </t-form-item>
        <t-form-item :label="$t('routes.timeout')">
          <t-input-number v-model="form.timeout_seconds" :min="0" :max="3600" theme="column" style="width: 140px" />
          <span class="hint">{{ $t('routes.timeoutHint') }}</span>
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
            <t-select v-model="form.failover_group_id" :placeholder="$t('routes.pickGroup')" style="width: 240px" clearable>
              <t-option v-for="g in groups" :key="g.id" :value="g.id" :label="`${g.name} (${g.plugin_label || g.plugin})`" />
            </t-select>
          </t-form-item>
          <t-form-item :label="$t('routes.failoverModel')" mark>
            <t-input v-model="form.failover_model" :placeholder="$t('routes.failoverModelPh')" style="width: 360px" />
          </t-form-item>
        </template>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { api } from '../api/client'
import { dict, strategyDict } from '../utils/dict'
import type { GroupInfo, RouteGroupEntry, RouteInfo } from '../api/types'

const { t } = useI18n()

const routes = ref<RouteInfo[]>([])
const groups = ref<GroupInfo[]>([])
const dialogVisible = ref(false)
const saving = ref(false)
const editingID = ref(0)
const form = reactive({
  name: '',
  strategy: 'round_robin',
  groups: [{ group_id: 0, weight: 100, model: '' }] as RouteGroupEntry[],
  timeout_seconds: 0,
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

function parseGroups(json: string): RouteGroupEntry[] {
  try { return JSON.parse(json) } catch { return [] }
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
    groups: [{ group_id: 0, weight: 100, model: '' }],
    timeout_seconds: 0, failover_enabled: false, failover_codes: [], failover_group_id: null, failover_model: '',
  })
  dialogVisible.value = true
}

function openEdit(row: RouteInfo) {
  editingID.value = row.ID
  Object.assign(form, {
    name: row.Name,
    strategy: row.Strategy,
    groups: parseGroups(row.GroupsJSON).length ? parseGroups(row.GroupsJSON) : [{ group_id: 0, weight: 100, model: '' }],
    timeout_seconds: row.TimeoutSeconds,
    failover_enabled: row.FailoverEnabled,
    failover_codes: [row.FailoverOn4xx && '4xx', row.FailoverOn5xx && '5xx'].filter(Boolean) as string[],
    failover_group_id: row.FailoverGroupID,
    failover_model: row.FailoverModel,
  })
  dialogVisible.value = true
}

function validate(): boolean {
  if (!form.name || !form.groups.length || form.groups.some((e) => !e.group_id || !e.model)) {
    MessagePlugin.warning(t('routes.errForm'))
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
    timeout_seconds: form.timeout_seconds,
    failover_enabled: form.failover_enabled,
    failover_on_4xx: form.failover_codes.includes('4xx'),
    failover_on_5xx: form.failover_codes.includes('5xx'),
    failover_group_id: form.failover_enabled ? form.failover_group_id : null,
    failover_model: form.failover_enabled ? form.failover_model : '',
  }
  try {
    if (editingID.value) {
      await api.put(`/admin/routes/${editingID.value}`, body)
      MessagePlugin.success(t('common.saved'))
    } else {
      await api.post('/admin/routes', body)
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

async function load() {
  const [r, g] = await Promise.all([
    api.get<{ routes: RouteInfo[] }>('/admin/routes'),
    api.get<{ groups: GroupInfo[] }>('/admin/groups'),
  ])
  routes.value = r.routes ?? []
  groups.value = g.groups ?? []
}

async function remove(id: number) {
  await api.del(`/admin/routes/${id}`)
  await load()
}

onMounted(load)
</script>

<style scoped>
.entries { width: 100% }
.entry { display: flex; gap: 8px; align-items: center; margin-bottom: 8px }
.hint { margin-left: 8px; color: var(--td-text-color-placeholder); font-size: 12px }
</style>
