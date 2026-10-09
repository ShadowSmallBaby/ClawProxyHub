# Go 后端与多平台扩展包

功能扩展后端使用 `github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension`，不复用 Go/Lua 业务插件 RPC。当前后端协议版本为 `1`；桌面入口是 `CGO_ENABLED=0` 的 Go `main` 程序，Android 入口使用 NDK 编译为 `c-shared` 原生库。业务处理器、动作和声明式存储共用。

一个 `.cphext` 包含公共前端资源和 Windows amd64、Linux amd64/arm64、macOS amd64/arm64 五个平台的程序；声明 Android 时，正式包额外包含 `arm64-v8a` 原生库。本地模拟器测试可加 `x86_64`。前端不限框架，输出相对路径的 HTML/JS/CSS 等静态资源；不支持需要 Node 服务的 SSR。

## 构建

源码布局与完整配置见 [notes 示例](../../examples/extensions/notes/README.md)。在核心仓库根目录执行：

```sh
python scripts/build-extension.py --project examples/extensions/notes/build.json --out build/extension-example --development-key
```

`build.json` 使用 `format: 1`、`manifest`、可选 `frontend` 与 `backend`：

```json
{
  "format": 1,
  "manifest": "manifest.json",
  "frontend": {
    "directory": "ui",
    "install": ["pnpm", "install", "--frozen-lockfile"],
    "build": ["pnpm", "run", "build"],
    "output": "dist",
    "entry": "index.html",
    "sdk": "cph.js"
  },
  "backend": { "language": "go", "module": "server", "package": "./cmd/service", "cgo": false }
}
```

路径相对配置文件目录；`output` 相对前端目录，`entry/sdk` 相对前端产物。已有静态资源可省略 `install/build` 并使用 `output: "."`。`sdk` 可选，将公共 JS 桥复制到指定路径，拒绝覆盖同名资源。`package` 必须指向 Go 模块中的单个 `main` 包。

构建器只在开发机或 CI 执行这些命令；宿主安装只处理已构建的签名文件。产物包含 `.cphext`、`trust.json` 和 `index.json`。正式签名传入 `--key/--key-id/--trust`，或使用 `CPH_EXTENSION_PRIVATE_KEY/CPH_EXTENSION_KEY_ID/CPH_EXTENSION_TRUST_JSON`；开发签名复用 `.cache/development-signing/extension.key`。

构建器补齐 `kind: service`、`target: backend`、`activation: hot`、`execution: trusted-process`、`platforms` 与 `backend: {language: go, protocol: 1, entries}`。`entries` 将五个平台映射到 `backend/<os>-<arch>/service[.exe]`。签名覆盖原始清单及所有资源摘要，宿主只选择当前平台入口，并检查其 Go 构建信息与 CGO 模式。

## 动作与服务入口

### Android 入口

在 `backend` 构建配置中增加 `android: {"package":"./android","min_sdk":24,"ndk":"28.2.13676358"}`，设置 `ANDROID_HOME`（或 `ANDROID_NDK_HOME`）。默认只生成 `backend/android-arm64/libservice.so`；本地测试传入 `--android-test-abi x86_64`，会额外生成 `backend/android-amd64/libservice.so`，该参数也参与缓存键。CI 和发行构建不使用测试参数。

Android `main` 包通过 `github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension/android.Register` 在 `init` 中注册 `func() extension.Service`。SDK 提供 JNI 符号、socket 接管与取消；入口示例见 [Lua 编辑器](../../extensions/lua-editor/backend/android/main_android.go)。后端不需要 Android HTTP 服务或 Go 业务插件协议。

打包器写入 `backend.android: {protocol:1,min_sdk:24}` 及平台入口。宿主核对签名、Go 构建信息、CGO 模式、安装路径、ELF ABI、SDK 与文件摘要后，由不导出的私有 Service 加载库。每个扩展独占一个进程，最多同时运行 8 个；停用、卸载、连接退出或加载超时会回收进程。进程仍与 APP 共用 UID，`trusted-process` 不表示操作系统权限沙箱。

### 共享处理器

后端动作不填 `target`；必须声明权限、效果、超时、输入和输出 schema，例如：

```json
{
  "id": "list",
  "title": "List notes",
  "permission": "storage.read",
  "effect": "read",
  "timeout_ms": 5000,
  "input_schema": {"type": "object"},
  "output_schema": {"type": "object", "additionalProperties": true}
}
```

权限须存在于清单 `permissions`，Go 后端还必须申请 `service.execute`。效果为 `read/write/execute`；超时为 1–300000 ms。Schema 支持 `object/array/string/boolean/number/integer`、`properties/required/additionalProperties/items/maxLength/maxItems/enum`，对象默认拒绝未知字段；结构限制为 16 层、2048 节点和 64 KiB，输入输出最大 2 MiB。

`Serve(Service)` 管理初始化和调用，`Service.Actions` 中的名称须与所有后端动作一一对应。别名核心动作与 `host.storage.*` 不在 Go 中注册。处理器签名为：

```go
func(ctx context.Context, host *extension.Host, input json.RawMessage) (any, error)
```

宿主通过私有 stdin/stdout 管道双向 RPC，处理并发、取消、退出及停止；stdout 不写日志，诊断写 stderr。前端 `invoke('list', {})` 由宿主鉴权后转发，扩展不公开 HTTP 监听端口。读写接口见 [storage.md](storage.md)。

`Service.Start` 可选，在动作开放前执行；`Service.Stop` 可选，在停止时释放业务资源。`Host.Initial` 提供身份、版本及经过校验的设置快照。当前后端设置在启动时读取，修改设置后重新启用后端；页面设置则会实时收到上下文更新。

后台存储仅接受 `backend.background_permissions` 中明确声明且已授权的 `storage.read/write`。`Start` 的上下文持续到服务停止；其他后台任务可使用 `Host.BackgroundContext(ctx)`。后台授权独立于触发某次动作的用户权限。初始化阶段只准读取，成功提交安装状态后才开放写入；无后台需求时省略声明。

## 生命周期与信任

宿主在启用或替换前核对入口和结构，握手失败会回退原版本。停用、卸载和切换包时取消动作、撤销存储会话并停止进程；意外退出标记失败，旧进程退出通知不影响新的进程会话。主动降级安装被拒绝。

更新原本停用的扩展时仍准备并提交声明结构，保留停用状态，直到显式启用才启动 Go 程序和执行握手。

执行需要签名身份的 `native: true` 及安装时的 `service.execute` 授权。普通 Go 子进程仍具有当前操作系统用户的权限；`trusted-process` 不是 OS 沙箱，SDK 对表与权限的检查不保证原生代码不能绕过 SDK 访问文件或网络。
