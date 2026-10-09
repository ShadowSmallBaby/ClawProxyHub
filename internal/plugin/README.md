# 插件运行适配

Manager 负责插件名称与协议校验、数据库快照、重复启动保护和断连重连。Runtime/Session 承接平台连接与生命周期，账号、网关和任务使用同一 protobuf 客户端。

- 桌面使用 go-plugin，接受 SDK 声明的协议范围，通过 Go 或 Lua 子进程运行插件。
- `WithRuntime(NewServiceRuntime(connector))` 接入平台校验的请求与回调连接；平台负责发现、身份校验、协议协商和解除绑定。
- Android 使用主 APP 的 Service 适配器加载同证书签名的 `.cphplugin`；依赖图排除桌面 go-plugin，拒绝启动桌面二进制。
- 生命周期变更串行执行，重复 Start 复用存活实例，并发断连只触发一次重连；失败握手与停止释放连接。

市场安装校验 Release manifest 并选择当前平台；未声明平台清单的索引使用兼容包。已声明清单的校验失败不会降级下载。

Lua Host 由扩展管理器安装并提供受信入口；桌面启动独立进程，Android 通过私有 Lua 服务加载已安装库。运行时替换与插件启动串行，存在使用者时拒绝替换或停用。Lua 创建和保存只修改源码，运行由独立工作区动作触发。

## 测试

`go test -race ./internal/plugin ./internal/app ./internal/task` 覆盖双向 gRPC、回调、断连与并发重连。桌面二进制兼容测试可设置 `CPH_TEST_EXISTING_PLUGIN_DIR` 指向旧版插件安装目录，再运行 `go test ./internal/plugin -run TestExistingDesktopBinary -count=1`。
