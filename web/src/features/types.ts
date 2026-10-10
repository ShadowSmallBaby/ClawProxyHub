import type { Component } from 'vue'
import type { Requirement } from './policy'

export interface Feature extends Requirement {
  path: string
  label: string
  desc: string
  icon: Component
  sidebar?: boolean
  component: () => Promise<{ default: Component }>
}
export interface GatewayUI {
  models?: typeof import('@/api/gateway').modelsApi
  dashboard?: Component
  logs?: Component
  accountTest?: Component
  settings?: Component
  logActions?: Component
}
