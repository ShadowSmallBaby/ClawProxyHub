# 构建、签名与验证

本页的 `C` 是核心根目录，`E` 是扩展源码目录。前端加 Go 后端使用 [统一扩展构建器](../../../scripts/build-extension.py)；核心发行预配的纯前端包继续使用 [包构建配置](../../../build-config.json)、[组件版本](../../../project.toml) 和 [发行包构建器](../../../scripts/build-packages.py)。

## 前端与 Go 后端的统一构建

在 `E/build.json` 声明开发期构建步骤，示例：

```json
{
  "format": 1,
  "manifest": "manifest.json",
  "frontend": {
    "directory": "frontend",
    "install": ["pnpm", "install", "--frozen-lockfile"],
    "build": ["pnpm", "run", "build"],
    "output": "build",
    "entry": "index.html",
    "sdk": "cph.js"
  },
  "backend": {"language": "go", "module": "backend", "package": ".", "cgo": false}
}
```

路径相对配置文件目录，`output` 相对前端目录，`entry/sdk` 相对前端产物。`sdk` 仅在产物尚未包含同名桥文件时使用。前端构建器不限 Vite 或 Vue；已有静态资源可省略 `install/build` 并使用 `output: "."`。有打包步骤时把框架自身的类型检查放进 `build`。Go 模块须含 `go.mod`，`package` 选择模块内单个 `main` 包。

在 `C` 构建示例：

```sh
python scripts/build-extension.py --project examples/extensions/notes/build.json --out build/extension-example --development-key
```

构建器运行一次前端构建，使用 `CGO_ENABLED=0` 生成 Windows amd64、Linux amd64/arm64、macOS amd64/arm64 程序，再统一签名和验签。产物为一个 `<id>-<version>.cphext`，另有 `trust.json` 和 `index.json`。前端文件在 `frontend/`，后端入口在 `backend/<os>-<arch>/`；清单的 Go 入口、执行模式和平台矩阵由构建器补齐，源清单仍需合法 ID、版本、动作和权限。

需要 Android 后端时，在 `backend.android` 声明 `package/min_sdk/ndk`，并用 `sdk/extension/android.Register` 注册服务工厂。构建器额外生成 arm64 JNI 库；本地模拟器用 `--android-test-abi x86_64`。完整入口与工具链约定见 [后端 SDK](../../../sdk/extension/backend.md#android-入口)。

`build.json` 是开发者或 CI 的执行配置，不随包成为安装指令。安装宿主不运行前端构建、`go build` 或迁移脚本。Go 扩展需 `service.execute` 和签名身份的 `native: true`；它是受信原生程序，不是操作系统沙箱。签名参数与下节一致，不自动加入核心发行预配或市场。

## 纯前端包与核心发行配置

主库扩展通常放在 `C/extensions/<id>/`，包含源 `manifest.json`、独立前端源码、`package.json`、依赖锁及构建配置。参考 [Lua 编辑器](../../../extensions/lua-editor/) 的结构；产物目录通常为 `E/package/`，含注入版本后的清单、`frontend/` 资源和必要许可证。

以新增 `source-viewer` 为例，在 `project.toml` 注册组件版本：

```toml
[components.source-viewer]
version = "0.1.0"
```

在 `build-config.json` 的 `packages` 对象中加入以下条目，包 ID 与源清单 `id` 一致：

```json
{
  "source-viewer": {
    "manifest": "extensions/source-viewer/manifest.json",
    "builder": "frontend",
    "directory": "extensions/source-viewer",
    "output": "package",
    "inputs": ["sdk/extension/ui.ts"],
    "suffix": ".cphext"
  }
}
```

`component` 缺省为包 ID，确需共享组件版本时才指定。`inputs` 补充扩展目录之外的构建输入，让依赖变动参与缓存判断。只注册独立包即可构建；加入 `frontends.*.packages` 或发行预配列表属于额外的发行行为，按用户需求处理。

前端构建遵循以下顺序：

1. 配置相对资源路径，如 Vite `base: './'`；输出到 `package/frontend`，对应清单的页面 entry。
2. 在 `package.json` 的 `build` 中完成类型检查、Vite 构建和产物准备。示例使用 `vue-tsc --noEmit && vite build && node prepare.mjs`；按作者技术栈适配。
3. 参考 [prepare.mjs](../../../extensions/lua-editor/prepare.mjs)，通过 `C/scripts/project_config.py manifest <id>` 获得注入版本的清单，复制必要资源；替换示例中的 `lua-editor` ID。需要 Python 时尊重打包脚本传入的 `CPH_PYTHON`。
4. 维护依赖锁和干净的产物目录，仅把本次需要的资源放入签名输入。不要把私钥、源码目录、`node_modules` 或旧签名文件放入包。

在 `E` 验证已有锁文件对应的构建：

```sh
pnpm install --frozen-lockfile
pnpm run build
```

新项目首次生成锁文件或确需更新依赖时使用正常的依赖安装流程。独立扩展仓库不假定有 `../../scripts`；显式定位核心工具，保持产物清单包含合法版本及资源入口。

## 签名包

在 `C` 为已注册的上述示例生成开发包：

```sh
python scripts/build-packages.py --platform windows/amd64 --only source-viewer --out build/packages --development-key
```

替换为目标包 ID 和实际平台；已有例子可用 `--only lua-editor`。`--only` 可重复，省略会构建更多已配置组件。通用前端包不会仅因构建参数 `--platform` 被自动限定为该平台。

脚本负责安装依赖、构建、注入版本、生成资源摘要、签名与验签，也会验证可复用缓存。输出 `<id>-<version>.cphext`、`index.json` 和 `trust.json`。`--development-key` 在 `.cache/development-signing/extension.key` 创建或复用本地身份；它仅用于开发。

包是 ZIP，含 `manifest.json`、`signature.json` 和资源。普通功能扩展使用 Ed25519 签名，签名覆盖原始清单字节，清单中的 SHA-256 绑定资源。不要在签名后改写包内容。

单独检查一个已构建包时，在 `C` 执行：

```sh
go run ./cmd/cphext verify --package build/packages/source-viewer-0.1.0.cphext --trust build/packages/trust.json
```

正式签名使用调用方提供的 `--key/--key-id/--trust`，或 `CPH_EXTENSION_PRIVATE_KEY/CPH_EXTENSION_KEY_ID/CPH_EXTENSION_TRUST_JSON`。信任声明约束发布者公钥、允许的包 ID 和权限；包内签名或证书不能自行建立信任。开发身份、私钥和安装包按各自用途保存，私钥不进入仓库或交付包。

`CPH_EXTENSION_TRUST_JSON` 是构建时的 JSON 文本，`CPH_EXTENSION_TRUST` 是运行时信任文件路径。具体格式及正式发行流程见 [安装与信任](../../../internal/extension/README.md) 和 [发行说明](../../../internal/distribution/README.md)。

## 安装联调

按任务授权在测试环境使用产物。在 `C` 的 PowerShell 中可配置：

```powershell
$env:CPH_PACKAGE_DIRS = 'build/packages'
$env:CPH_EXTENSION_TRUST = 'build/packages/trust.json'
```

然后按项目启动方式运行目标核心，并从扩展中心检查包清单、权限和安装结果。多挂载目录在 Windows 用 `;`、Linux/macOS 用 `:` 分隔。优先使用测试数据目录，保留现有安装和用户配置。

功能扩展安装在 `<data>/extensions/versions/<id>/<hash>/`；`<data>/packages` 是默认挂载目录。首次启动可自动安装受信兼容包，`CPH_INSTALL_PACKAGES=false` 只关闭首次自动安装，不阻止已有安装加载或手动安装。

已有安装不会因重新构建或重启自动替换；停用状态和卸载收据也会保留。同版本不同内容需要显式“更新”，更高版本需要显式“升级”，降级被拒绝。联调应确认安装摘要对应本次产物，升级保留数据与启停状态，切换失败保留旧版本。准备正式发布时按组件版本规则升版本，不用同版本替换发布内容。

## 按变更范围验证

| 变更 | 必要验证 |
| --- | --- |
| 扩展前端、清单或设置声明 | 扩展类型检查与构建、签名包验签；在目标宿主检查页面入口、字段和动作 |
| 消息桥或交互 | 初始化顺序、语言/主题/设置实时更新、成功与错误、取消、停用后旧会话失效；编辑功能覆盖草稿恢复及保存与执行 |
| 设置行为 | 默认值、`false`、只读约束、无效选择值、升级保留有效配置及过期包摘要拒绝 |
| Go 后端或业务表 | 全部平台构建、实际平台启动与 RPC；权限、原子批次、升级失败回退、成功标记废弃表、显式清理和重启持久化 |
| 核心扩展 SDK、动作或安装器 | 按变更选择 `go test ./sdk/extension ./internal/extension ./internal/extservice ./internal/extstore ./internal/action ./internal/app` 中相关包，再重编核心；备份变化覆盖 `internal/database` 与系统设置 HTTP 端点 |
| 宿主 Web 代码 | 在 `C/web` 跑 `npx vue-tsc --noEmit`、`npx vite build`；涉及桥或管理 UI 时选跑 `tests/extension-ui.test.ts`、`tests/extension-management.test.ts`、`tests/extensions.spec.ts` |
| 仅技能或说明文档 | 检查格式、链接及命令与当前代码是否一致，无需重编应用 |

普通扩展改动优先验证自身构建与实际包，不要求改核心或重新构建整个发行包。需要新增测试时覆盖有意义的交互、权限或持久化边界，避免仅重复静态声明。构建、验签和宿主联调分别报告；没有运行目标宿主时明确其尚未验证。
