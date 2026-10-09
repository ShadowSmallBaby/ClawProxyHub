// 扩展 UI 协议：仅描述数据与事件，不依赖宿主框架、组件或样式。
export type LocalizedText = string | Record<string, string>
export type ContributionLocation = 'plugins.toolbar' | 'plugins.item.actions'
export type ExtensionEnvironment = 'app' | 'web-desktop' | 'web-mobile'

export interface UICapabilities {
  version: 1
  surfaces: readonly ('page' | 'drawer' | 'notice')[]
  fields: readonly UIField['kind'][]
  locations: readonly ContributionLocation[]
  image_max_bytes: number
}

export interface UIControl {
  label: LocalizedText
  hint?: LocalizedText
  intent?: 'primary' | 'default' | 'danger'
  disabled?: boolean
  loading?: boolean
}

export interface UIAction extends UIControl { id: string }

export interface UIPage {
  title: LocalizedText
  subtitle?: LocalizedText
  actions: UIAction[]
}

interface UIFieldBase {
  id: string
  label: LocalizedText
  hint?: LocalizedText
  required?: boolean
  readonly?: boolean
}

export type UIField = UIFieldBase & (
  | { kind: 'text' | 'textarea'; value?: string; max_length?: number }
  | { kind: 'select'; value?: string; options: { value: string; label: LocalizedText }[] }
  | { kind: 'toggle'; value?: boolean }
  | { kind: 'image' }
)

// 持久化设置使用同一套基础控件；图片通过业务数据管理，不保存到配置。
export type SettingField = UIFieldBase & (
  | { kind: 'text' | 'textarea'; default?: string; max_length?: number }
  | { kind: 'select'; default?: string; options: { value: string; label: LocalizedText }[] }
  | { kind: 'toggle'; default?: boolean }
)
export type SettingsValues = Record<string, string | boolean>

export interface UIDrawer {
  id: string
  title: LocalizedText
  fields: UIField[]
  submit: UIControl
}

// 图片仅由宿主文件选择器产生，不接受扩展传入的 URL 或初始图片。
export interface UIImage { name: string; mime: 'image/png' | 'image/jpeg' | 'image/webp'; data: string }
export type UIValue = string | boolean | UIImage

export type UICommand =
  | { kind: 'page'; page: UIPage }
  | { kind: 'drawer'; drawer: UIDrawer | null }
  | { kind: 'notice'; level: 'success' | 'warning' | 'error' | 'info'; message: LocalizedText }

export type UIEvent =
  | { kind: 'activate'; page: string; context: { name?: string }; contribution?: { id: string; location: ContributionLocation } }
  | { kind: 'action'; id: string }
  | { kind: 'submit'; id: string; values: Record<string, UIValue> }
  | { kind: 'close'; id: string }

export interface HostContext {
  name: string
  locale: string
  dark: boolean
  environment: ExtensionEnvironment
  settings?: SettingsValues
  draft?: { content: string; name: string; dirty: boolean }
  ui?: UICapabilities
}
