# 核心装配与平台宿主

CLI 读取配置与处理信号，公共启动和关闭由 `app` 与 `module.Registry` 管理。模块声明依赖及能力，装配前拒绝缺失依赖、循环、重复 ID 和能力冲突；启动按依赖顺序执行，失败和退出按逆序清理。

## 运行组合

核心统一使用 full，包含数据库迁移、网关、路由、账号、插件和任务。桌面发行内嵌 Web；Android 等原生宿主通过 `WithoutWeb()` 禁用 Web 模块，由宿主提供界面。发行锁及 `CPH_PROFILE` 仅接受 full。

`go build ./cmd/cph` 和正式桌面发行均内嵌 Web；原生宿主用 `cph_no_web` 排除这些资源。统一脚本将核心、CLI 与原始签名包装配成 full ZIP，详见[统一发行](../distribution/README.md)。Web 与 App 前端均保留网关与任务入口，并按后端能力和用户权限显示。

`GET /admin/capabilities` 需要登录，返回组合和模块 installed/enabled/available 状态。内置模块通过重启变更；第三方功能通过扩展管理器独立启停。

## 插件与扩展

业务插件管理器先注册，扩展管理器再恢复已安装状态并导入挂载包，最后启动业务插件。Lua Host 来自已安装且启用的 `.cphhost`，Lua 编辑器来自 `.cphext`；两者共用验签、平台兼容、依赖和生命周期。运行中的 Lua 插件阻止运行时替换、停用或卸载。

`WithPluginRuntime` 注入平台适配器，桌面默认使用独立进程。Android 复用相同核心生命周期、SQLite 与任务服务，通过 JNI、私有 Service 和系统调度提供平台能力；`WithRuntimeValidator` 在切换运行时前执行平台验签、协议握手与 Lua VM 检查。模块接口不作为第三方扩展 ABI。

GUI、`cli` 与认证 MCP 通过动作注册器共享任务和工作区操作。MCP 默认停止，通过独立管理员密钥、在线工具授权和服务开关管理，详见 [MCP](../mcpserver/README.md)。

## 系统调度

`WithExternalScheduler()` 禁用常驻扫描，平台通过 `ExecuteDue(ctx)` 执行有界任务。数据库规则领取使用唯一锁、随机执行令牌、五分钟租约和活动续约；旧调度快照不能重复领取。启动只恢复过期记录，不影响另一活动核心的执行。

过期执行记录为结果未知，不自动重放外部副作用。执行令牌透传插件 context，供插件实现幂等；取消与停机等待运行记录落盘后再释放领取。Android JobScheduler 使用持久化任务及取消令牌，受系统调度和省电策略约束。

## 开发检查

```sh
go test ./...
go test -race ./internal/module ./internal/app ./internal/plugin ./internal/task
go -C hosts/luahost test ./...
```

任务 worker、账号刷新和日志清理退出后才关闭数据库；HTTP 监听失败使用同一清理路径。
