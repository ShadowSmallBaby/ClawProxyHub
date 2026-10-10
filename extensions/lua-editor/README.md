# Lua 编辑器

Lua 编辑器以 `lua-editor` 扩展独立安装，版本取自 [project.toml](../../project.toml)。Vue / CodeMirror 前端通过宿主桥调用 Go 后端，提供语法检查、元数据解析和持久化草稿；新建、读取、保存及手动运行复用核心工作区动作。编辑器与核心共用[静态解析器](../../sdk/luasource/metadata.go)，跳过注释，识别 Lua 字符串字面量，不创建 Lua VM。保存不依赖 Lua Host，运行需要已启用的运行时。

桌面与 Android 共用前端、业务处理器、动作和表声明。桌面入口连接 stdin/stdout；Android 入口通过 [扩展 SDK](../../sdk/extension/backend.md)注册处理器，由 APP 私有 Service 加载 NDK 原生库。清单及构建定义见 [manifest.json](manifest.json) 和 [build.json](build.json)。

## 使用与草稿

编辑区运行于沙箱，标题栏、保存与运行按钮、新建表单通过[扩展 UI 协议](../../sdk/extension/README.md)交给宿主渲染。首次保存时，抽屉展示 Go 后端从 Lua 语法树提取的插件名、显示名及可选图标；不执行源码，未完成的函数也可以保存。语法问题显示行列号。

启用编辑器后，Web 插件页与 Android 原生“已安装”页提供“新建 Lua 插件”，可编辑的 Lua 插件提供“编辑”。扩展中心负责安装、更新、设置、启停和卸载。Android App 不预装编辑器，需要单独导入受信任的 `.cphext`。

修改源码后自动保存草稿；底部“草稿已保存”表示后端持久化完成。`drafts` 和 `draft_chunks` 两张声明式表由宿主保存在 `cph.ext.db`，随系统备份和恢复。草稿分块保存，最后提交修订；中断写入保留上一份完整草稿。修订号检查拒绝旧页面覆盖或删除其他页面更新后的草稿。重启、停用和升级保留草稿，显式保存源码后清除对应草稿；卸载后的“清理数据”删除该扩展全部草稿。正式保存仍将 `main.lua`、插件清单及图标写入插件目录，源码分析本身不读写数据库。

重新打开某个插件时恢复其草稿；若对应源码也有更新，会提示先检查内容。尚未创建的插件使用独立草稿。宿主当前会话的临时草稿仍用于保留尚未提交的输入。语法检查不会触发保存或运行，运行始终需要单独点击。

主题可选“跟随系统／浅色／深色”，默认为深色。设置、语言和主题更新不会重建编辑器或覆盖源码。

## 构建

需要 Go、Python 3.11+、Node.js、pnpm 和 Android NDK 28.2.13676358；设置 `ANDROID_HOME` 或 `ANDROID_NDK_HOME`。在主仓库根目录执行：

```powershell
python scripts/build-packages.py --platform windows/amd64 --only lua-editor --out build/packages --development-key
```

输出 `lua-editor-<version>.cphext`、`index.json` 和 `trust.json`。包内包含公共前端、Windows amd64、Linux amd64/arm64、macOS amd64/arm64 程序，以及 Android `arm64-v8a` 库。CI 和正式包只带 ARM64 Android 库，本地模拟器测试额外传入 `--android-test-abi x86_64`。该参数参与缓存键，不会把测试包复用为正式包。

也可使用通用构建器：

```powershell
python scripts/build-extension.py --project extensions/lua-editor/build.json --out build/editor --development-key
```

清单已经通过通用构建器统一生成：组件版本来自 `project.toml`，平台、二进制入口、执行方式及 Android 最低版本来自 `build.json`，文件摘要由签名打包阶段生成。源 `manifest.json` 中的权限、页面贡献、动作输入输出和存储表是需要人工评审的契约，无法仅从 Go 函数或前端代码可靠推导。新增扩展可复用同一构建入口；无需各自编写清单生成脚本。

`assets` 的字符串值表示静态源文件，自动参与缓存指纹。生成资源使用 `{ "source": "package/icon.svg", "inputs": ["prepare.mjs"] }`，`inputs` 相对构建定义目录指向生产脚本及其读取的源文件（可引用仓库公共文件）；不把生成结果加入缓存。`frontend.sdk` 注入的公共桥也自动参与缓存。

`pnpm --dir extensions/lua-editor build` 只构建前端，完整包须通过上述构建器补齐原生入口、签名并验签。包内不含私钥。开发签名复用忽略目录 `.cache/development-signing/` 下的身份；正式签名使用 `--key/--key-id/--trust` 或 `CPH_EXTENSION_PRIVATE_KEY/CPH_EXTENSION_KEY_ID/CPH_EXTENSION_TRUST_JSON`。

签名身份必须具有 `native: true`，允许 `lua-editor` 和清单中全部 `workspace.read/write/execute`、`service.execute`、`storage.read/write` 权限；上传包不会自动增加信任根。由旧的纯前端版本升级时需授权新增权限。

本地核心从挂载目录安装：

```powershell
$env:CPH_PACKAGE_DIRS = (Resolve-Path build/packages).Path
$env:CPH_EXTENSION_TRUST = (Resolve-Path build/packages/trust.json).Path
pnpm --dir web build
go build -o build/cph.exe ./cmd/cph
./build/cph.exe
```

## 验证

`go test ./...`（在 `backend/` 内）覆盖语法、元数据、分块草稿、修订冲突、中断写入和重启恢复。`pnpm --dir web test:browser` 使用真实核心与签名包验证编辑、诊断、草稿、离线保存和生命周期；先构建扩展包与 Web / App 前端。Android 私有 Service / JNI 测试入口及构建参数见 [android/README.md](../../android/README.md)。
