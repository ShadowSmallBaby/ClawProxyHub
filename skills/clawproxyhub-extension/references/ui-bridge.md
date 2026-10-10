# 页面桥接与受控动作

协议类型见 [ui.ts](../../../sdk/extension/ui.ts)，消息接收逻辑见 [ExtensionPage.vue](../../../web/src/views/extensions/ExtensionPage.vue)，数据校验见 [extensionUI.ts](../../../web/src/features/extensionUI.ts)。开发前核对目标宿主能力。

## 沙箱与会话

页面在 `sandbox="allow-scripts"` 的 iframe 中加载。资源来自已安装包，宿主 CSP 的 `connect-src 'none'` 禁止页面直接联网；脚本、样式和字体应打入包。使用相对资源路径，避免依赖 CDN、宿主路由或开发服务器。

1. 等待宿主的 `cph:init`，保存 `nonce` 和 `context`；只接受来自 `window.parent` 的消息。
2. 检查 `context.ui.version` 及实际需要的 `surfaces/fields/locations`，订阅 UI 事件，再发送 `cph:ready`。能力缺失时显示明确的兼容性提示。
3. 宿主发送一次 `activate` 事件，包含页面、可选贡献和上下文；从 `event.context.name` 读取插件入口携带的名称。
4. 后续消息携带相同 `nonce`，业务调用和 UI 请求共用递增的正整数 `id`，以 `cph:result` 对应成功结果或错误。处理超时、取消及迟到响应。
5. `cph:context` 更新语言、主题和本扩展设置。页面重载、包切换、停用、卸载或切换连接会撤销旧会话；不跨会话复用 nonce 或未完成请求。

宿主还校验来源窗口及沙箱的 `null` origin。沙箱跨来源消息使用 `postMessage(..., '*')` 时必须保留窗口与 nonce 校验，不能通过放宽 iframe 同源权限来解决通信问题。

## 采用公共桥 SDK

优先使用 [bridge.js](../../../sdk/extension/bridge.js) 与相邻 `bridge.d.ts`、`ui.ts`。构建时打入资源；统一构建器的 `frontend.sdk` 可将 JS 桥复制为包内 `cph.js`。React、Vue、Svelte 或原生 JS 均可使用，无全局宿主 SDK 对象：

| 函数或状态 | 用法 |
| --- | --- |
| `ready: Promise<HostContext>` | 获得初始化上下文 |
| `onUIEvent(listener)` | 订阅事件，返回取消订阅函数 |
| `activate()` | 发出 `cph:ready`；先订阅再调用 |
| `ui(command)` | 请求宿主更新标准 UI，返回 Promise |
| `invoke<T>(action, input, signal?)` | 按清单动作的短 ID 调用，支持取消 |
| `draft(content, name, dirty)` | 交给宿主做离开确认与会话草稿恢复 |
| `onContext(listener)` | 订阅初始与后续上下文，返回取消订阅函数；据此更新语言、主题与设置 |

SDK 保留请求关联、并发限制、超时和取消处理。通过 `onContext` 更新自己的状态，先订阅 `onUIEvent` 再调用 `activate()`。完整原生 JS 用法见 [笔记页面](../../../examples/extensions/notes/frontend/main.js)。现有 [Lua 编辑器 bridge.ts](../../../extensions/lua-editor/src/bridge.ts) 还包含编辑器专用的 Vue `appearance` 状态，不把它当作公共 SDK 的一部分。

## 标准界面与事件

标准标题、按钮、抽屉、通知交给宿主；编辑器等专用内容在扩展内部渲染。文案使用字符串或语言对象，UI 描述只包含协议支持的数据。

| `UICommand.kind` | 主要字段 |
| --- | --- |
| `page` | `page: {title, subtitle?, actions}` |
| `drawer` | `drawer: {id, title, fields, submit}`；`null` 关闭 |
| `notice` | `level: success/warning/error/info`、`message` |

例如使用本地桥描述一个按钮：

```ts
await ui({
  kind: 'page',
  page: {
    title: { zh: '插件源码', en: 'Plugin source' },
    actions: [{ id: 'refresh', label: { zh: '刷新', en: 'Refresh' }, intent: 'primary' }],
  },
})
```

在 `onUIEvent` 中处理 `action` 且 `id === 'refresh'` 的事件，再执行读取操作。其他事件包括 `activate`、`submit`（抽屉 ID 与 `values`）和 `close`（抽屉 ID）。卸载页面组件时取消订阅。

- 一条 UI 描述序列化后最多 65,536 字符；页面最多 8 个动作，抽屉最多 24 个字段。未知字段、重复 ID、HTML、CSS、组件名和事件代码均不接受。
- 按钮使用 `label/hint/intent/loading/disabled`；抽屉提交操作使用 `submit` 描述。操作期间反映加载状态，失败保留用户输入并反馈错误。
- 抽屉字段支持 `text/textarea/select/toggle/image`。同 ID 抽屉更新会保留用户输入并同步只读值；关闭后清除表单。
- 图片只由宿主选择器产生，不接收 URL 或初始 `value`；回传 `{name, mime, data}`，`data` 是 Base64。支持 PNG/JPEG/WebP，按 `context.ui.image_max_bytes` 限制大小。

## 业务动作与权限

清单动作可以别名核心动作、宿主存储操作，也可以由扩展自己的 Go 后端处理。例如清单声明 `{"id":"read","target":"core.workspace.read","title":"Read source"}` 且申请 `workspace.read` 后，桥使用短 ID：

```ts
// pluginName 来自 activate 的 event.context.name；先检查该入口确实提供了名称。
const result = await invoke<{ content: string }>('read', { name: pluginName, file: 'main.lua' })
```

宿主将其路由到 `<扩展ID>.read`，核心再调用 `core.workspace.read`。不要将 `core.workspace.read` 或 `<扩展ID>.read` 作为 `invoke()` 的 action 参数。签名清单、信任身份、安装 grants 和当前用户权限必须满足要求，UI 按钮 ID 不等于动作授权。

核心动作参数以 [动作注册](../../../internal/app/actions.go) 和 [工作区动作](../../../internal/app/extensions.go) 为准。当前工作区动作仅操作可编辑本地插件的 `main.lua`：

| 目标 | 权限 | 输入 |
| --- | --- | --- |
| `core.workspace.scaffold` | `workspace.read` | `{}`，返回 `lua` 模板 |
| `core.workspace.read` | `workspace.read` | `{name, file: 'main.lua'}`，返回 `content` |
| `core.workspace.create` | `workspace.write` | `{name, content, label?, icon?}`，`icon` 为 Base64 图片数据 |
| `core.workspace.save` | `workspace.write` | `{name, file: 'main.lua', content}` |
| `core.workspace.reload` | `workspace.execute` | `{name}` |

普通扩展可按需别名其他已注册核心动作，但不能别名其他扩展动作或需要 `extensions.manage` 的管理动作。自己的 Go 动作同样用短 ID 调用，由宿主路由到当前已提交的后端；后端与存储声明见 [Go 后端与业务存储](backend-storage.md)。清单不包含处理函数代码或任意 HTTP 代理命令。

## 编辑与草稿

编辑类扩展通过 `draft(content, name, dirty)` 更新宿主草稿状态，从初始化 `context.draft` 恢复未保存内容。宿主负责离开确认和当前会话恢复；草稿不是持久化业务存储。只有保存成功后才更新已保存基线，失败时保留脏状态。

保存源码、执行或重载分别由对应动作触发。运行已保存的内容，必要时在存在未保存修改时禁用执行；保存本身不启动插件或安装 LuaHost。完整事件处理见 [Lua 编辑器 App.vue](../../../extensions/lua-editor/src/App.vue)。
