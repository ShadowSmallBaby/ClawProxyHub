// 布局共享类型：菜单/页面描述项。
import type { Component } from 'vue'

export interface MenuItem {
  value: string
  label: string // i18n key（menu.xxx）
  desc: string // i18n key（menuDesc.xxx）
  icon: Component
}
