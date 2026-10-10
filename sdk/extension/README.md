# 功能扩展 SDK

`sdk/extension` 提供 `.cphext` 清单、页面桥、Go 后端协议和声明式存储类型；与业务插件的 `sdk` 根包及 protocol v2 独立。

- [Go 后端与多平台构建](backend.md)：`Service/Host`、动作 schema、桌面和 Android JNI 入口、构建命令。
- [声明式业务存储](storage.md)：共享 `cph.ext.db`、多表、结构化请求、权限和清理规则。
- [原生 JS + Go 笔记示例](../../examples/extensions/notes/README.md)：公共前端、两张业务表与单个签名包。

页面使用 [bridge.js](bridge.js)、[bridge.d.ts](bridge.d.ts) 和 [ui.ts](ui.ts)。SDK 无 Vue 依赖；`ready/onContext/onUIEvent/activate/invoke/ui/draft` 负责会话、上下文、动作与标准界面。将桥随前端一起构建，或通过 `build.json` 的 `frontend.sdk` 复制到产物。先订阅事件，再在 `ready` 后调用 `activate()`。

## 扩展 UI 协议

扩展通过纯数据声明界面，通过事件处理用户操作。宿主负责注入位置、标准控件、抽屉、通知、语言和主题；扩展可在独立沙箱中渲染编辑器等专用内容。扩展不导入宿主的 Vue 组件、内部模块或样式。类型定义见 [ui.ts](ui.ts)，示例见 [Lua 编辑器](../../extensions/lua-editor/src/App.vue)。

## 会话与能力

签名清单可用 `environments` 声明 `app`、`web-desktop`、`web-mobile`，省略表示不限制界面；显式空数组、未知值与重复值会被拒绝。Lua 编辑器声明支持三种界面。

App 与 Web 分别检查安装范围；不匹配的包在目录中显示“不支持当前界面”，不能从该界面安装。`web-desktop` 与 `web-mobile` 只约束功能入口与页面，两种 Web 界面都能安装、设置和管理 Web 扩展，共用核心上的后端与启用状态。Web 使用项目统一的 768px 断点，App 身份不随窗口宽度变化。直接访问页面同样受界面范围限制；切换到不支持的界面会撤销沙箱会话和未完成调用，保留草稿，不停用核心上的扩展。

`context.environment` 提供当前界面，窗口跨越 Web 断点后通过 `cph:context` 更新。`platforms` 仍表示后端操作系统与 CPU 架构，与界面范围无关。

宿主为已安装、可用的签名页面创建 `sandbox="allow-scripts"` 的 iframe，在加载后发送 `cph:init`，携带当前会话的 `nonce` 和 `context`。`context.ui` 声明 UI 协议版本、可用界面、字段类型、注入点及图片大小上限。扩展检查能力、订阅事件后发送 `cph:ready`；宿主随后发送一次 `activate` 事件。`cph:context` 推送语言、主题和本扩展设置变化，无需重载页面。

## 持久化设置

扩展在签名 `manifest.json` 的 `settings` 数组声明配置。字段使用 `id`、`kind`、`label`、`hint?`、`default?`、`required?`、`readonly?`；文本可设置 `max_length`，选择项使用 `options: [{value, label}]`。`label/hint` 为语言对象。设置支持 `text/textarea/select/toggle`，最多 24 项，持久化 JSON 上限 64 KiB；开关的 `false` 是有效值。

宿主提供设置入口和统一表单，配置按扩展 ID 保存，更新与卸载保留，显式清理数据后删除。只读项必须声明默认值，服务端禁止修改。新版本删除的字段或不再支持的选项不会传入当前扩展；缺省值取自当前签名清单。设置修改请求携带包摘要，防止旧表单覆盖已更新包的配置。

扩展从 `context.settings` 读取自身配置，不能读取其他扩展或系统配置。`context.dark` 始终表示工作台当前主题；扩展可根据自己的主题设置决定专用内容的外观，宿主渲染的抽屉和按钮遵循工作台主题。

所有后续消息都携带该 `nonce`。宿主同时检查消息来源窗口和沙箱来源；页面重载、停用、卸载、切换连接或包版本变化会撤销旧会话与未完成调用。扩展不接收 Token、宿主 DOM 或原生对象。

## 注入点

签名清单的 `contributions` 声明 `id`、`location`、`page`、`label`，可通过 `labels` 提供多语言文案。

`pages` 定义可加载页面，`contributions` 决定入口位置；扩展管理卡片不自动列出页面入口。

`pages` 与 `actions` 可通过 `labels: {"zh":"名称","en":"Name"}` 声明展示名称，`title` 保留默认文案。宿主在依赖清单中按当前语言展示，操作 ID 仍用于调用和授权。

| 位置 | 宿主呈现 | 激活上下文 |
| --- | --- | --- |
| `plugins.toolbar` | 工具栏按钮 | 声明的贡献 ID |
| `plugins.item.actions` | 插件卡片操作链接 | 贡献 ID、插件 `name` |

点击后，宿主打开清单声明的页面，再通过 `activate` 事件传入 `contribution: {id, location}` 和上下文。当前事件仅交给该页面的活跃会话；扩展通过 `onUIEvent` 订阅，无需访问插件页内部代码。

## 界面请求

扩展发送 `{type: 'cph:ui', nonce, id, command}`。`id` 是本会话内递增的正整数；宿主以 `cph:result` 返回同一个 `id` 的成功结果或错误。业务调用与 UI 请求使用同一个请求编号序列。

| `command.kind` | 内容 | 宿主行为 |
| --- | --- | --- |
| `page` | `page: {title, subtitle?, actions}` | 渲染标题栏与按钮；返回按钮由宿主管理 |
| `drawer` | `drawer: {id, title, fields, submit}` | 打开或更新统一表单抽屉 |
| `drawer` | `drawer: null` | 关闭当前扩展抽屉 |
| `notice` | `level`、`message` | 显示标准通知 |

表单支持 `text`、`textarea`、`select`、`toggle`、`image`。字段可声明标题、提示、必填、只读和初始值，图片只接受用户在宿主选择的 PNG/JPEG/WebP 文件。图片通过 `{name, mime, data}` 回传，`data` 为 Base64；上限取自 `context.ui.image_max_bytes`。同 ID 抽屉更新时保留用户输入、同步只读字段，关闭后清除表单。

按钮使用 `id`、`label`、`hint?`、`intent?`、`loading?`、`disabled?`；标题和文案可为字符串或语言对象，例如 `{"zh":"保存","en":"Save"}`。宿主选择具体组件与样式，不接受 HTML、CSS、组件名、事件代码或图片 URL。单条描述最多 65,536 个字符、8 个页面动作、24 个表单字段；未知字段、重复 ID 和不支持的控件会被拒绝。

```ts
await ui({
  kind: 'page',
  page: {
    title: { zh: '编辑插件', en: 'Edit plugin' },
    actions: [{ id: 'save', label: { zh: '保存', en: 'Save' }, intent: 'primary' }],
  },
})

onUIEvent(event => {
  if (event.kind === 'action' && event.id === 'save') saveSource()
})
```

## 操作事件与授权

宿主发送 `{type: 'cph:ui-event', nonce, event}`：

| `event.kind` | 内容 |
| --- | --- |
| `activate` | 页面 ID、上下文和可选的贡献信息 |
| `action` | 用户点击的页面动作 `id` |
| `submit` | 抽屉 `id` 与表单 `values` |
| `close` | 用户关闭的抽屉 `id` |

事件只表示用户操作，不授予额外权限。扩展需用 `cph:invoke` 调用签名清单中声明的业务动作；宿主与核心继续校验动作、权限及扩展可用状态。保存草稿使用 `cph:draft`，宿主负责离开确认和会话内恢复；保存源码与运行插件是两个独立动作。
