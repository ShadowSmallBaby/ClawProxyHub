// 编辑器动作集中在此，前端只通过宿主桥访问核心与扩展后端。
import { invoke } from './bridge'

export interface Analysis {
  name: string
  labels: Record<string, string>
  sha256: string
  valid: boolean
  valid_name: boolean
  lines: number
  bytes: number
  diagnostics: { line: number; column: number; message: string }[]
}
export interface DraftRevision { revision: string; sha256: string; updated_at: string }
export interface Draft extends DraftRevision { name: string; content: string; base_sha256: string }
export interface DraftInput { name: string; content: string; base_sha256: string; revision: string }

export const editorApi = {
  scaffold: () => invoke<{ lua: string }>('scaffold', {}),
  read: (name: string) => invoke<{ content: string }>('read', { name, file: 'main.lua' }),
  save: (name: string, content: string) => invoke('save', { name, content, file: 'main.lua' }),
  create: (input: { name: string; label: string; content: string; icon: string }) => invoke<{ name: string }>('create', input),
  run: (name: string) => invoke('reload', { name }),
  analyze: (content: string, signal?: AbortSignal) => invoke<Analysis>('analyze', { content }, signal),
  readDraft: (name: string) => invoke<{ found: boolean; draft?: Draft }>('draft-read', { name }),
  saveDraft: (input: DraftInput) => invoke<DraftRevision>('draft-save', input),
  deleteDraft: (name: string, revision: string) => invoke<{ deleted: boolean }>('draft-delete', { name, revision }),
}
