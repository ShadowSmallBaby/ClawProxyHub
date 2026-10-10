# 验证与打包

下列插件命令均在 `P`（插件仓库根目录）执行，`myplugin` 替换为本次目标插件名。`C` 与 `P` 的关系见技能入口，安装路径按实际部署解析。

## 按变更验证

Go 实现先对受影响包验证；准备提交时完成目标仓库 `AGENTS.md` 要求的全量检查。下面分别列出命令，不把前一步失败当成后一步通过：

```sh
go vet ./plugins-go/myplugin/...
go test ./plugins-go/myplugin/...
```

需要仓库级验证时：

```sh
go vet ./...
go test ./...
```

运行 `gofmt` 格式化改过的 Go 文件。Go 源码变更必须重编；后面的目标打包或开发安装会编译新的插件二进制。不要只运行测试后继续联调旧产物。

Lua 打包器不执行 Lua 语法或 RPC 验证。使用目标 LuaHost 加载**本次脚本目录**，检查模块返回值、handshake 和声明函数，再用假上游或开发环境调用相关能力。离线测试可参照 `C/hosts/luahost/load_test.go`，在合适的测试工作区通过 `luahost.New(dir)`、`SetHost` 和 RPC 驱动目标脚本；无需把业务插件测试强塞进核心正式源码。只跑宿主既有测试不会自动覆盖新插件。

仅修改 LuaHost 时，才在 `C/hosts/luahost` 执行其独立模块测试和构建；修改核心 / 网关则在 `C` 执行相关测试并 `go build`。普通插件开发无需构建 Web；实际涉及 Web 改动时遵守其 `AGENTS.md`，完成 `npx vue-tsc --noEmit` 和 `npx vite build`。

从以下项目选取与实现相关的用例，优先使用本地假上游和脱敏固定样本：

| 能力 | 要证明的行为 |
| --- | --- |
| 握手 | 清单名与版本一致；不支持的协议给错误；声明能力真实可调用 |
| 授权 / 刷新 | 空或坏凭据失败；多步 state 往返；刷新后 blob 能再次使用；旧凭据格式兼容 |
| 模型 / Chat | 模型 ID 可解析；工具调用及结果关联；图片 / 推理等已宣称功能保持语义 |
| 流 | 正常收尾；多行与大帧；空流、坏 JSON、断流和无终态报错；客户端取消后停止出站 |
| 用量 | 缓存读写与非缓存输入无重复；reasoning 不重复计入输出 |
| 实例 / 账号 | 两个实例和账号使用各自地址、凭据、代理与状态 |
| Lua 宿主回调 | 配置更新可读；跨插件实例被拒绝；空值与不存在可区分；状态跨 VM / 重启可读；存储失败不假成功、取消能终止回调 |
| 任务 | 成功、业务跳过与失败可区分；只在凭据变更时返回 changed 与 blob |

没有真实上游账号时继续完成离线验证，清楚标出未做真实登录或调用；不要将打包、编译或旧样本通过描述为线上接入通过。

## 生成可检查的插件包

```sh
go run ./tools/pack -only myplugin -out build/myplugin
```

产物为 `dist/myplugin/<name>-<version>.cphplugin` 与该输出目录内的 `index.json`。`-only` 避免构建无关插件；该索引仅用于本次产物检查，不能覆盖仓库根的市场索引。

检查 ZIP 内容与解出的清单：

- Go 包：身份清单、可选图标、`plugin-windows-amd64.exe`、Linux amd64/arm64 与 macOS amd64/arm64 对应二进制。打包器优先从 `cmd/` 构建，注入 `main.version`。
- Lua 包：`runtime: "lua"`、`entry: "main.lua"`、脚本与实际引用的 `lib/*.lua`、可选图标；没有 LuaHost 二进制。
- 包内协议取自构建 SDK；版本与源清单、运行时握手一致。包内不携带账号文件、私钥、构建缓存或测试数据。

`.cphplugin` 是业务插件包。`.cphext` 功能扩展和 `.cphhost` 运行时有另外的元数据与安装流程，不互相改扩展名复用。

## 开发安装与联调

任务包含开发安装时，先确定目标核心配置的插件目录并停止目标插件；Windows 运行中的二进制可能被锁定。核心仓库内的子模块布局下，以下相对路径指向 `C/data/plugins`：

```sh
go run ./tools/pack -only myplugin -install ../data/plugins
```

独立插件仓库或自定义数据目录要替换成实际路径，不能照抄。`-install` 只生成当前平台二进制或复制脚本、清单和图标，不打包或生成索引。

安装后启动或重载目标插件；Lua 先确认已安装且启用兼容 LuaHost。检查显示版本与声明能力，再按需求创建实例、登录账号、同步模型、配置测试路由并调用所声明入口。用本次请求 ID 对照 `request_logs` 与插件运行日志，验证失败映射、事件收尾和用量；不要从旧日志推断本次产物正常。

## Android 与发布（仅需要时）

Android Go 插件复用业务工厂，通过 `P/android/` 的独立工程构建，不把桌面进程打包成 APK。先读 `P/android/README.md` 中当前 JDK / NDK / SDK 要求。

```sh
sh android/gradlew -p android -PcphPlugins=myplugin -PcphABI=arm64-v8a packageCphPlugin
python android/verify_packages.py android/build/plugin-packages
```

Windows 第一条改用 `./android/gradlew.bat`，保留其余参数。原生包须通过签名、摘要、ABI 和 ELF 校验；发行签名须与宿主 APP 证书一致。Lua 包平台无关，但目标平台仍需安装 LuaHost。

发布相关修改遵循以下来源与边界：

- `manifest.json` 是插件版本来源；已发布插件变更按仓库约定升版本，已发布资产不可覆盖。SDK 升级同步依赖和受影响已发布插件版本。
- 桌面 / Lua 包、平台发布清单与市场索引是不同层次；生成与校验按 `P/PACKAGING.md` 和 CI 实现，不手造下载地址、摘要或平台支持声明。
- `P/index.json` 由 CI 在资产成功上传后回写；`-skip` 依赖现有索引基线，不删除或重置它。`P` 的发布分支为 `main`；主库提交遵循主库分支约定，不混用。
- 实际上传或发布须在用户任务范围内。仅开发插件时，交付源码、验证结果和本地包即可。
