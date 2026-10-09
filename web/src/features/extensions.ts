import { shallowRef } from 'vue'
import { extensionApi, type ExtensionState } from '@/api/extensions'
import { onConnectionReset } from '@/api/connections'
import { canUse } from './access'
export const extensionStates=shallowRef<ExtensionState[]>([])
export const runtimeManagement=shallowRef<'core'|'native'>('core')
function reset(){extensionStates.value=[];runtimeManagement.value='core'}
onConnectionReset(reset)
export async function refreshExtensions(){if(!canUse('extensions')){reset();return};const result=await extensionApi.list();extensionStates.value=result.extensions;runtimeManagement.value=result.runtime_management||'core'}
