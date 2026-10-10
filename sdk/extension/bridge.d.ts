import type { HostContext, UICommand, UIEvent } from './ui'
export const ready: Promise<HostContext>
export function invoke<T = unknown>(action: string, input: unknown, signal?: AbortSignal): Promise<T>
export function ui(command: UICommand): Promise<void>
export function activate(): void
export function onUIEvent(listener: (event: UIEvent) => void): () => void
export function onContext(listener: (context: HostContext) => void): () => void
export function draft(content: string, name: string, dirty: boolean): void
