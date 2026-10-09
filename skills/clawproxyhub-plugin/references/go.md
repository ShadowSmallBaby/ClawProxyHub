# Go 插件实现

先读目标 SDK 和 `P/plugins/newapi/` 中与本次能力对应的文件。`C/examples/stub/main.go` 演示基本契约；新插件的工程结构以可导入业务包和独立桌面入口为准。

## 结构与启动

```text
P/plugins/myplugin/
  manifest.json
  plugin.go        业务工厂、握手、宿主与配置
  cmd/main.go      桌面入口
  auth.go          仅需要复杂登录时拆分
  chat.go          仅实现对话能力时添加
  *_test.go        与实际行为对应的测试
```

业务根目录使用 `package myplugin`，提供 `New(version string) sdk.Plugin`。实现结构内嵌 `pb.UnimplementedClawPluginServer`；版本保存在实例字段，空版本可回退 `dev`。需要宿主回调时实现 `SetHost(*sdk.Host)` 并保存注入对象。

桌面入口的版本变量与打包器保持一致：

```go
//go:build !android

package main

import (
    "github.com/ShadowSmallBaby/ClawProxyHub/sdk"
    "github.com/ShadowSmallBaby/ClawProxyHubPlugins/plugins-go/myplugin"
)

var version = "dev"

func main() { sdk.Serve(myplugin.New(version)) }
```

独立模块开发时调整业务包导入路径。`Handshake` 检查 `req.ProtocolVersion` 与所实现的 `sdk.ProtocolVersion`，不匹配返回结构化错误，成功返回身份、版本和实际能力。不要把某次发行版本同时硬编码进工厂、握手与清单。

业务层保持可导入，Android 入口也调用同一工厂。普通桌面打包使用 `CGO_ENABLED=0`；引入依赖前确认可交叉编译。Android 相关操作见[验证与打包](validation.md)。

## 宿主、配置与凭据

| API | 用途 |
| --- | --- |
| `host.InstanceSettings(pluginName, instanceID)` | 返回插件设置、实例设置、实例 `base_url` 依次覆盖的 JSON；0 表示插件级设置 |
| `host.Settings(pluginName)` | 只读插件级设置 |
| `host.StoreGet(key)` / `host.StorePut(key, value)` | 插件范围内持久化小状态；账号凭据仍通过 RPC 返回 |
| `host.Log` / `host.LogFields` | 统一运行日志 |
| `sdk.ProxyURL(cred.GetProxy())` | 将请求信封中的出站代理转换为 URL |

需要区分回调失败或跟随 RPC 取消时，在已支持的 SDK 版本使用 `SettingsContext/InstanceSettingsContext/StoreGetContext/StorePutContext`；传入当前 RPC context，并处理返回 error。旧的便捷方法签名保留，但可能把读取失败降级为默认值。

实例设置可短期缓存，但缓存键包含实例 ID；账号会话、模型映射或登录状态再按需要包含账号 ID。存储命名空间只隔离到插件，业务键需自行区分实例和账号。共享 map、连接及刷新状态按并发 RPC 设计，不使用一个“当前账号”全局变量。

`Login` 从 `req.InstanceId` 取实例；账号 RPC 从凭据信封取实例。重新序列化时只保存插件凭据，不把核心注入的代理密码与路由上下文混入 blob。改旧插件时先看现有 `credFrom` 与已保存字段，保持旧凭据可读取。

## 出站与流处理

使用 SDK / 插件仓库已有助手，按上游协议选请求体构造器和解析器，完整字段要求见[公共契约](contract.md)。

- 普通请求可用 `host.HTTPPost(ctx, sdk.HTTPRequest{...}, client)`，按 `Method` 设置实际方法。标准 SSE 用 `host.StreamSSE`；非标准流可用 `host.StreamRaw` 或已有上游封装。
- `HTTPRequest.Proxy` 在 `client=nil` 时用于构造代理客户端；传入自建 client 时由调用方确保代理配置正确。已有 HTTP 流用 `sdk.ScanSSE(resp.Body, parser)`，已约定的大帧按需用 `ScanSSEWithLimit`。
- HTTP 请求绑定 RPC context；Chat 取 `stream.Context()`，处理取消、超时和响应体关闭。日志通过 `Sensitive` 增补上游特有敏感头，不记录完整响应凭据。
- `StreamSSE` 遇非 200 会返回 `HTTPResponse` 且可能 `err=nil`，不会自动发失败事件；调用方必须检查状态码。读流错误可能已经由解析器发 `TaskFailed`，不能再无条件补一个失败。
- SDK parser 的 emit 回调没有 error 返回值。封装时保存首次 `stream.Send` 错误并取消出站请求，最终把发送错误返回；不要照抄示例里忽略发送错误的写法。
- 自己读 SSE 时必须处理多行 `data:`、帧上限、读错误和 `Finish/FinishWithError`。普通上游优先交给 `sdk.ScanSSE`，避免逐行 Scanner 循环遗漏收尾。

只有 SDK 解析器无法覆盖上游时才实现自定义转换。复杂协议可参考对应插件：`P/plugins/puter/` 的 NDJSON、`P/plugins/todofor/` 的 WebSocket；不要继承与新上游无关的认证或内容改写行为。

## 最小行为验证

测试工厂注入的版本与握手一致，声明的能力有真实实现，凭据能够往返且不携带临时信封字段。用假上游或固定帧验证请求、事件和失败语义；涉及多实例时用两站点、两账号证明配置和状态不串用。模型目录的 ID 应可被 Chat 的映射逻辑识别，工具能力标志须与实际转换一致。

具体命令、包内容和联调步骤见[验证与打包](validation.md)。
