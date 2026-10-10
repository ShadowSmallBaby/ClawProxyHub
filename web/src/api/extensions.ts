// 扩展管理与动作仍使用当前连接的统一传输。
import { api, upload, resourceURL, request } from './client'
import { actionApi } from './actions'
import type { ExtensionEnvironment, SettingField, SettingsValues } from '../../../sdk/extension/ui'
export interface ExtensionManifest { id:string;version:string;name:string;label?:Record<string,string>;desc?:Record<string,string>;kind:string;target:string;activation:string;permissions:string[];environments?:ExtensionEnvironment[];dependencies?:Record<string,unknown>;settings?:SettingField[];backend?:{language:'go';protocol:number;entries:Record<string,string>;background_permissions?:string[];android?:{protocol:number;min_sdk:number}};storage?:{schema_version:number;tables:{name:string}[]};pages?:{id:string;title:string;labels?:Record<string,string>;entry:string}[];actions?:{id:string;target?:string;title:string;labels?:Record<string,string>}[];contributions?:{id:string;location:string;label:string;labels?:Record<string,string>;page:string;when?:string;order?:number}[] }
export interface ExtensionSettings {hash:string;fields:SettingField[];values:SettingsValues}
export interface ExtensionState {manifest:ExtensionManifest;hash:string;signer:string;publisher:string;enabled:boolean;available:boolean;status:string;error?:string;bytes:number}
export interface PackageEntry {path:string;sha256?:string;manifest?:ExtensionManifest;signer?:string;publisher?:string;status:string;error?:string;installed_version?:string;installed_hash?:string;enabled:boolean;available:boolean}
export interface ExtensionImpact {extensions:string[];plugins:string[];tasks:number[];pages:string[];actions:string[]}
export interface ExtensionTrustConfig {public_key?:string;certificate?:string;publisher:string;ids:string[];permissions:string[];native?:boolean}
export interface ExtensionTrust {code:string;config:ExtensionTrustConfig;read_only:boolean}
export interface OnlinePackage {id:string;version:string;display_name:string;label?:Record<string,string>;desc?:Record<string,string>;kind:string;environments?:ExtensionEnvironment[];name:string;size:number;sha256:string;status:string;installed_version?:string}
export interface OnlineCatalog {packages:OnlinePackage[];cached:boolean;checked_at:string;release_url:string}
function packageForm(file:File,grants?:string[]){const form=new FormData();form.append('package',file);if(grants)form.append('grants',JSON.stringify(grants));return form}
export const extensionApi={
 list:()=>api.get<{extensions:ExtensionState[];runtime_management?:'core'|'native'}>('/admin/extensions'),
 catalog:()=>api.get<{packages:PackageEntry[]}>('/admin/extensions/catalog'),
 trust:()=>api.get<{identities:ExtensionTrust[]}>('/admin/extension-trust'),
 saveTrust:(code:string,config:ExtensionTrustConfig)=>api.put(`/admin/extension-trust/${encodeURIComponent(code)}`,config),
 deleteTrust:(code:string)=>api.del(`/admin/extension-trust/${encodeURIComponent(code)}`),
 online:(refresh=false)=>api.get<OnlineCatalog>(`/admin/extensions/marketplace?refresh=${refresh}`),
 inspectOnline:(sha256:string)=>api.post<ExtensionState & {ticket:string}>('/admin/extensions/inspect-market',{sha256}),
 installStaged:(ticket:string,grants:string[])=>actionApi.invoke<ExtensionState>('core.extensions.install',{ticket,grants}),
 installMounted:(sha256:string,grants:string[])=>api.post<ExtensionState>('/admin/extensions/install-mounted',{sha256,grants}),
 impact:(id:string)=>actionApi.invoke<ExtensionImpact>('core.extensions.impact',{id}),
 settings:(id:string)=>api.get<ExtensionSettings>(`/admin/extensions/${encodeURIComponent(id)}/settings`),
 saveSettings:(id:string,hash:string,values:SettingsValues)=>api.put<ExtensionSettings>(`/admin/extensions/${encodeURIComponent(id)}/settings`,{hash,values}),
 inspect:(file:File)=>upload<ExtensionState>('/admin/extensions/inspect',packageForm(file)),
 install:(file:File,grants:string[])=>upload<ExtensionState>('/admin/extensions/install',packageForm(file,grants)),
 enable:(id:string,enabled:boolean)=>actionApi.invoke(`core.extensions.${enabled?'enable':'disable'}`,{id}),
 uninstall:(id:string)=>actionApi.invoke('core.extensions.uninstall',{id}),
 cleanCache:(id:string)=>actionApi.invoke('core.extensions.cache',{id}),
 cleanData:(id:string)=>actionApi.invoke('core.extensions.data',{id}),
 obsolete:(id:string)=>actionApi.invoke<{hash:string;tables:string[]}>('core.extensions.obsolete',{id}),
 cleanObsolete:(id:string,hash:string)=>actionApi.invoke('core.extensions.clean-obsolete',{id,hash}),
 asset:(s:ExtensionState,entry:string)=>resourceURL(`/extension-assets/${encodeURIComponent(s.manifest.id)}/${s.hash}/${entry.split('/').map(encodeURIComponent).join('/')}`),
 invoke:(id:string,input:unknown,signal?:AbortSignal)=>request('POST',`/admin/actions/${encodeURIComponent(id)}`,input,signal),
}
