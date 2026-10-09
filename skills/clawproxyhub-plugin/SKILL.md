---
name: clawproxyhub-plugin
description: "辅助编写、适配、调试和打包 ClawProxyHub 的 Go / Lua 业务插件（.cphplugin）。用于接入新上游，实现登录与凭据、模型、对话流、多实例设置、持久化状态和任务能力；不用于开发 .cphext 功能扩展、.cphhost 运行时或普通网关配置。"
---

# ClawProxyHub 插件开发

将作者提供的上游接口资料、请求样本或已有实现，落成符合当前 ClawProxyHub 契约的业务插件，交付源码、配置说明和实际验证结果。按用户选择保留上游、语言和目标目录。

## 先定位工作区与契约

本文用 `C` 表示核心仓库根目录，用 `P` 表示插件仓库根目录。这些是路径记号，不是 shell 变量：

- `C` 含 `sdk/proto/cph.proto`，仓库为 [ClawProxyHub](https://github.com/ShadowSmallBaby/ClawProxyHub)。
- `P` 的 Go module 为 `github.com/ShadowSmallBaby/ClawProxyHubPlugins`，含 `tools/pack/`。主库检出时通常 `P = C/plugins`；独立开发时 `P` 就是当前插件仓库，不能再多拼一层 `plugins/`。
- Go 源码在 `P/plugins-go/<name>/`，Lua 源码在 `P/plugins-lua/<name>/`。安装后的插件目录是部署产物，不默认作为仓库源码目录。

读取目标工作区适用的 `AGENTS.md`，检查已有改动，再读目标插件的清单与实现。主库与插件子模块分别检查 Git 状态；只修改任务涉及的仓库和文件。

按以下来源核对接口，避免照搬过时示例：

| 要确认的内容 | 权威来源 |
| --- | --- |
| RPC、字段、能力与协议版本 | 目标 SDK 的 `sdk/proto/cph.proto`、`sdk/sdk.go` |
| Go 出站与协议转换 | 目标 SDK 的 `sdk/http.go`、`sdk/sse.go`、三个上游适配器 |
| Lua 真正可调用的函数与字段 | 目标 LuaHost 的 `cph.go`、`host.go`、`http.go`、`rpc.go`、`proto.go`、`result.go`、`stream.go` |
| 包内文件、版本与构建入口 | `P/tools/pack/main.go`、`P/PACKAGING.md`、`P/android/README.md` |

插件锁定的 SDK 可能落后于核心工作区。需要定位实际依赖时，在 `P` 执行：

```sh
go list -m -f '{{.Version}} {{.Dir}}' github.com/ShadowSmallBaby/ClawProxyHub
```

沿用 `P/go.mod` 锁定的依赖与该仓库的升级约定；新增接口先确认依赖中存在。普通插件构建无需添加本地 `replace`、Go workspace 或复制 SDK。当前新插件使用 protocol v2；`cphv1` 是生成代码包名，不能据此把协议写成 1。

## 选择运行时并按需读参考

优先延续已有插件或用户指定的运行时。新插件尚未指定语言时，按实际所需能力判断：

| 场景 | 选择与参考 |
| --- | --- |
| 需要 Go SDK 的其他适配器、WebSocket 或复杂上游协议 | Go，读 [Go 实现](references/go.md) |
| HTTP / OpenAI 兼容上游，需要多实例设置或持久化小状态，希望脚本分发 | Lua 可用 `cph.settings` 与 `cph.store`，读 [Lua 实现](references/lua.md) |
| 在项目 Lua 编辑器内编写或修改自建插件 | Lua，另读 Lua 参考中的“本地编辑器场景” |
| 修改能力声明、授权、凭据、Chat 或任务 | 读 [公共契约](references/contract.md) |
| 测试、生成安装包、开发安装或准备发布 | 读 [验证与打包](references/validation.md) |

LuaHost 通过 `cph.settings` 与 `cph.store` 支持多实例设置和持久化小状态；使用新增回调时确认目标核心和 LuaHost 已更新。其余能力以宿主注册的接口为准。用户指定 Lua 而需求超出当前宿主能力时，说明具体缺口与可行选择；不要捏造 API 或顺手扩展核心。

## 实施流程

1. **形成接入说明。** 从用户资料与代码确认插件名、上游地址及路径、认证方式、模型来源、上游协议、目标入口、是否多站点及任务能力。缺少影响实现的真实接口或样本时再询问，同时推进已确定部分；不把猜测的端点、签名算法或模型能力写成事实。
2. **选择最接近的实现。** Go 多实例参考 `P/plugins-go/newapi/`，Lua HTTP 接入参考 `P/plugins-lua/autoclaw/`，Lua 任务参考 `C/examples/luatask/main.lua`。只提取当前所需结构；其他上游的请求头、签名常量、内容改写和重试策略不自动成为新插件要求。
3. **实现已声明能力。** 先打通清单与握手，再完成所需授权、账号、模型、Chat 或任务。已有插件保持凭据兼容；新增必填字段时处理旧 blob。仅任务插件无需实现 Chat，单纯模型插件无需伪造登录能力。
4. **验证后产出。** 选择与变更相关的测试和目标插件打包命令，检查实际产物。需要联调时加载本次产物，避免拿旧二进制或旧 VM 的结果当作新实现证据。

## 实现边界

- 业务插件实现 `ClawPlugin` 契约。核心负责路由、账号持久化、任务调度和客户端协议输出；新增插件通常不需要给核心或前端加插件名分支。
- 包清单负责身份与分发，运行时 Handshake 负责能力、授权方式和 schema。声明必须与实现相符，不靠 `Unimplemented` 或空成功结果掩盖缺失功能。
- 账号凭据经 RPC 结果交回核心持久化；实例、账号与代理上下文来自请求信封。按实例和账号隔离配置、缓存及会话。
- Chat 接收统一信封并输出统一事件；优先复用适配器。保留工具调用、内容块和用量语义，不能把复杂输入统一拼成字符串。
- 日志使用宿主日志接口，排障结合 `request_logs`。不向插件进程 stdout 写调试信息，也不把令牌、Cookie、完整凭据或授权回调写进日志及测试夹具。

## 交付给作者

说明插件位置与运行时、已实现能力、配置与登录方式、已运行的验证、包路径及尚未验证的外部条件。命令带清晰的执行目录。开发完成默认交付源码和本地产物；安装、提交或发布按本次任务已有授权执行，不自动改市场索引或发布 Release。
