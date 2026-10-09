# 接入 SDK

`sdk/` 是 ClawProxyHub 对外接入契约的集合。业务插件与功能扩展使用独立的协议和生命周期。

| 路径 | 用途 |
| --- | --- |
| `sdk` 根 Go 包 | 现有 Go 业务插件入口、宿主回调、HTTP 与流处理工具 |
| `sdk/proto/cphv1`、`sdk/transport`、`sdk/androidplugin` | 业务插件 RPC、会话和 Android 连接适配 |
| `sdk/anthropicup`、`sdk/openaiup`、`sdk/responsesup`、`sdk/requestutil`、`sdk/streamutil` | 业务插件的上游协议处理工具 |
| [`sdk/extension`](extension/README.md) | `.cphext` 功能扩展清单、Go 后端协议、声明式存储及框架无关的页面桥 |

业务插件继续导入 `github.com/ShadowSmallBaby/ClawProxyHub/sdk`；功能扩展 Go 后端导入 `github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension`。前端可使用该子目录的 `bridge.js`、`bridge.d.ts` 与 `ui.ts`，无需依赖宿主 Vue。

保留现有业务插件导入路径，避免目录整理要求核心、LuaHost 与独立插件仓库同步迁移。新功能扩展契约放入 `extension/`；两者不共用协议版本、授权或存储接口。SQLite 连接、迁移和实际执行留在 `internal/extstore`，不属于对外 SDK。
