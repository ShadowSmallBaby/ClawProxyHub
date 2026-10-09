---
name: clawproxyhub-extension
description: "辅助开发、调试和签名打包 ClawProxyHub 功能扩展（.cphext），支持框架不限的沙箱前端、Go 后端、多平台包及宿主声明式存储。不用于 Go/Lua 业务插件（.cphplugin）、LuaHost 运行时（.cphhost）或 App 前端包（.cphui）开发。"
---

# ClawProxyHub 扩展开发

将作者的功能需求落成可独立构建、签名安装的项目扩展，交付源码、配置说明和实际验证结果。沿用用户指定的目录、前端技术与部署目标。

## 先定位工作区与可用能力

本文用 `C` 表示 ClawProxyHub 核心仓库根目录，用 `E` 表示目标扩展源码目录；这是路径记号，不是 shell 变量。`C` 含 `sdk/extension/manifest.go`，仓库为 [ClawProxyHub](https://github.com/ShadowSmallBaby/ClawProxyHub)。主库内通常 `E = C/extensions/<id>`，业务插件子模块 `C/plugins` 不存放功能扩展。

先读适用的 `AGENTS.md` 并检查已有改动，再读目标扩展的清单、构建脚本和入口。独立扩展仓库需定位匹配目标核心版本的 SDK 与打包工具，按其实际目录调整路径；安装后的 `data/extensions/versions/` 是产物目录。

按当前代码确认契约，文档与实现不一致时先核对目标宿主版本：

| 内容 | 权威来源（相对 `C`） |
| --- | --- |
| 清单、扩展 API 与设置约束 | `sdk/extension/manifest.go`、`sdk/extension/backend.go`、`sdk/extension/settings.go` |
| Go 协议、声明式存储 | `sdk/extension/service.go`、`sdk/extension/storage.go`、`internal/extservice/`、`internal/extstore/` |
| 页面 SDK、UI 数据类型与宿主能力 | `sdk/extension/bridge.js`、`sdk/extension/bridge.d.ts`、`sdk/extension/ui.ts`、`web/src/features/extensionUI.ts` |
| 消息会话、授权与动作参数 | `web/src/views/extensions/ExtensionPage.vue`、`internal/app/extension_services.go`、`internal/app/extensions.go`、`internal/app/actions.go` |
| 验签、启停与设置持久化 | `internal/extension/` |
| 构建与签名 | `scripts/build-extension.py`、`scripts/build-packages.py`、`cmd/cphext/main.go`；发行预配另看 `project.toml`、`build-config.json` |

当前扩展 API 为 `1`，独立于业务插件 protocol v2。新页面扩展通常采用 `kind: "frontend-sandbox"`、`target: "backend"`、`activation: "hot"`。`target` 表示安装位置，系统兼容性由 `platforms` 表达。

带 Go 后端的扩展使用 `service/backend/hot` 和 `trusted-process`，一个签名包包含公共前端及五个桌面平台入口。后端固定 Go、`CGO_ENABLED=0`，不能沿用业务插件 protocol v2；Android Go 功能扩展后端尚不支持。`frontend-trusted` 仍没有执行器。

`sdk/` 是多类接入 SDK 的集合。功能扩展使用 `sdk/extension`；已有业务插件保留 `sdk` 根包及协议子包的导入路径，不为开发一个扩展重排业务插件目录。

## 按任务读取参考

| 工作 | 参考 |
| --- | --- |
| 新建清单、入口、依赖、权限或持久化设置 | [清单与设置](references/manifest-settings.md) |
| 沙箱页面、统一控件、消息桥、业务调用或草稿 | [页面桥接与动作](references/ui-bridge.md) |
| Go 处理器、多张业务表、存储授权、升级和清理 | [Go 后端与业务存储](references/backend-storage.md) |
| 源码布局、组件注册、构建、签名、安装与验证 | [打包与验证](references/packaging.md) |

纯页面例子是 `C/extensions/lua-editor/`；公共页面 SDK、原生 JS 前端、Go 后端和两张业务表的例子是 `C/examples/extensions/notes/`。按需求选择，替换扩展身份、文案、动作和配置；不要顺带引入编辑器依赖或工作区权限。

## 实施流程

1. **确定功能与入口。** 确认扩展 ID、用户操作、入口位置、目标核心、所需业务动作与设置。先查已有动作及输入 schema；缺少影响实现的资料时再询问，同时推进已确定部分。
2. **形成最小清单。** 页面写入 `pages`，入口写入 `contributions`，业务调用写入 `actions` 并声明对应权限。动作可指向核心动作、`host.storage.*` 或自身 Go 处理器。核对依赖和最低核心版本；只有需要加入核心发行配置时才注册 `project.toml` 组件。
3. **实现页面与桥。** 独立构建页面和资源，使用相对资源路径和公共 JS/TS 桥 SDK。前端不限 Vue。检查 `context.ui`、订阅事件后通知宿主就绪；标准标题、按钮、抽屉和通知走 UI 协议，专用内容在沙箱中渲染。
4. **接入后端与状态。** 设置用清单 `settings`，业务记录用 `storage.tables`；宿主统一校验、建表和读写。需要 Go 逻辑时用 `extension.Serve` 和调用上下文中的 `Host.Storage/Batch`。编辑类页面按需接入草稿恢复、离开确认及取消处理。
5. **验证并交付。** 完成目标扩展的类型检查和构建，生成并验签 `.cphext`。按变更范围验证入口、授权、存储、设置与生命周期，记录实际包摘要、宿主版本及运行平台；交叉编译不等于在全部平台运行过。

## 实现边界

- 扩展只导入共享协议类型并管理自己的依赖；不导入宿主 Vue 组件、内部模块、CSS 或访问宿主 DOM。遵守工作区样式约定，语言和主题从宿主上下文获得。
- iframe 不接收 Token、原生对象或任意 HTTP 能力。业务调用通过已签名声明的动作，页面按钮或表单事件本身不授予权限。
- 主库是 `cph.db`，所有扩展业务数据共用 `cph.ext.db`；一个扩展可声明多张表。扩展只提交逻辑表、字段、条件和值，不持有 SQL、连接、数据库路径或自行执行迁移脚本。
- 新版本省略旧表时，成功安装才标记废弃，安装失败保留旧标记；清理废弃表是显式操作，不能升级时直接删除。主动降级仍被拒绝，失败回退不等于开放降级安装。
- 普通 Go 子进程具有当前用户的系统权限。`trusted-process`、签名和 SDK 权限检查不构成 OS 沙箱，不能宣称它们强制阻止原生代码绕过 SDK 访问文件或网络。
- 组件源清单、构建输出、挂载包和已安装版本分别维护；修改源码后重新构建和显式安装，不能把旧包的运行结果当作本次验证。

## 交付给作者

说明源码目录、入口与功能、声明的权限和设置、实际构建与验证结果、包路径及尚未验证的目标环境。命令标明执行目录。安装、提交或发布遵循本次任务已有授权；普通开发交付不自动改发行预配、市场清单或 Release。
