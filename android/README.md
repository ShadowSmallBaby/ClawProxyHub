# ClawProxyHub Android

ClawProxyHub（`github.shadowbaby.clawproxyhub.core`）是包含 Miuix 原生界面、Web 工作台、Go 核心和 SQLite 的单一 APP。原生页面负责插件管理、工作台切换和应用设置；Web 工作台承载账号、实例、网关和任务等业务页面。Lua Host 与可选扩展以签名包携带，启动后由核心统一安装。

## 工程结构

| 路径 | 职责 |
| --- | --- |
| `app/` | 主应用、原生页面、插件加载器和设备测试 |
| `app/src/main/go/` | 核心 JNI 入口，复用根 Go module |
| `native.gradle` | 核心、Lua Host 和最小测试插件的 ABI 编译 |
| `app/src/androidTest/go/`、`plugin-test.gradle` | 仅测试 APK 携带的最小签名插件 |
| `../plugins/android/` | 业务插件的独立编译、签名和包校验 |
| `../hosts/luahost/` | 桌面和 Android 共用的 Lua 运行时 |

## 构建

依赖 JDK 21、Python 3.11+、根 `go.mod` 指定的 Go、Android platform 37.0、build-tools 37.0.0 和 NDK 28.2.13676358。使用 Gradle wrapper 9.7.1、AGP 9.4.1；默认 minSdk 24、targetSdk 36，取自根 `project.toml`。设置 `JAVA_HOME` 和 `ANDROID_HOME` 后，在主仓库根目录执行：

```powershell
pnpm --dir web install --frozen-lockfile
pnpm --dir web run build:app
python scripts/build-packages.py --platform android/arm64 --only frontend.app --prebuilt-web --out build/packages --development-key
./android/gradlew.bat -p android -PcphABI=arm64-v8a :app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintDebug
```

Linux/macOS 使用 `sh android/gradlew`。模拟器使用 `-PcphABI=x86_64`；每个 APK 只包含一个 ABI。主包输出到 `build/android/app/outputs/apk/debug/`，测试包输出到 `build/android/app/outputs/apk/androidTest/debug/`。Go 原生库使用 16 KiB ELF 对齐。

默认前端与运行时预配取自 `build-config.json.android`。App 使用 `app-full`，不预配 Lua 编辑器，保留完整网关、账号和任务能力；包目录可用 `-PcphPackageDir` 指定。Gradle 构建缓存覆盖原生库、签名包及应用任务。

界面更新附件为 `app-<前端版本>.cphui`，与 APP、Android Lua Host 和业务插件使用相同的 RSA 发行证书。用户在“设置 → 工作台界面”导入签名包，校验平台、核心版本、文件哈希和证书后启用；启动未完成时回退上一界面，APK 自带更高界面版本时恢复内置资源。后端扩展信任配置不能改变原生界面的签名要求。已有前端资源时可单独运行 `:app:packageAppFrontend`，输出到 `build/android/frontend-packages/`，无需编译核心或 APK。

主 APP 从插件子库读取 `index.json` 作为市场目录快照，业务插件按[子库构建说明](https://github.com/ShadowSmallBaby/ClawProxyHubPlugins/blob/develop/android/README.md)打包。最小 `androidtest` 插件使用当前核心 SDK 和 APP debug 证书，在构建测试 APK 时生成。

## 签名与发行

Debug 默认使用 Android 标准 `~/.android/debug.keystore`；设置 `ANDROID_USER_HOME` 时使用该目录。正式 APP、App 界面包、官方原生插件和 Lua Host 更新包必须使用同一套 RSA 发行证书，各工程独立读取签名配置。

本地正式构建设置 `CPH_ANDROID_KEYSTORE`、`CPH_ANDROID_STORE_PASSWORD`、`CPH_ANDROID_KEY_ALIAS`、`CPH_ANDROID_KEY_PASSWORD`，执行：

```powershell
./android/gradlew.bat -p android -PcphABI=arm64-v8a -PcphReleaseSigning=true :app:assembleRelease :app:lintRelease :app:packageLuaHost :app:packageAppFrontend
```

两个仓库的 GitHub Actions 配置相同的 `CPH_ANDROID_KEYSTORE_BASE64` 及上述后三项 Secrets。`prepare_signing.py` 在 runner 临时目录还原 keystore，工作流结束后删除。正式构建缺少配置或使用非 RSA 密钥时失败。

APP 的版本、versionCode、applicationId 和 SDK 要求取自根 `project.toml`，Lua Host 及其他组件也从该文件读取各自独立版本。更新与桌面共用 latest Release 的 `update-manual.json`，APP 读取 `platform.android` 并按 applicationId、versionCode、ABI 和 minSdk 匹配；安卓日志源为 `version.json` 的 `platform.android.changelog`。APK 文件名包含 APP 自身版本和 versionCode，项目 tag 仅用于 Release 地址。APK 内容变化后须增加 versionCode，连接核心的版本不参与 APP 更新比较。

- 主仓库 `release.yml` 构建 arm64-v8a 正式 APK 和 `.cphhost`，复用 `app-full` 前端并上传到核心 Release。
- 主仓库 `android.yml` 在相关 PR 或手动触发时运行宿主、SDK、Lua Host 及 APP 更新选择单测，编译 debug APP 和测试 APK，执行 lint 并上传报告。设备测试需另行运行。
- 插件仓库 `build.yml` 构建、校验和发布 `.cphplugin`，维护市场索引与平台清单。

## 插件与运行时

Go 业务插件以签名 `.cphplugin` 导入主 APP，由私有 worker Service 加载。包内包含 `manifest.json`、`signature.json`、`lib/<abi>/libcphplugin.so` 和可选图标；包格式与市场索引见[插件包格式](https://github.com/ShadowSmallBaby/ClawProxyHubPlugins/blob/develop/PACKAGING.md)。加载器验证发行证书、路径、文件摘要、ABI、系统版本和协议，保留当前及上一版本供回滚。

每个 Go worker 进程只装载一个库，会话结束后退出进程；同一 UID 下的进程提供崩溃隔离。Lua 脚本使用 APP 的私有服务，运行时库来自已安装的 `.cphhost`。`.cphhost` 与 `.cphext` 共用扩展管理器及安装状态，独立 Lua 编辑器的打包方法见[扩展说明](../extensions/lua-editor/README.md)。

Lua 插件使用市场条目的通用脚本包，不单独发布 Android 版本；安装时校验 SHA-256、包内身份和协议。市场及已安装列表按 Lua Host 状态显示运行时未安装、已停止或不可用，运行时可用后显示插件自身的安装及运行状态。Go 插件仍按 `platforms.android` 和发布清单选择匹配 ABI 的签名原生包。

APK assets 中的原始包复制到 `<files>/packages/bundled`，核心验签后将 Host 安装到 `<files>/hosts`、功能扩展安装到 `<files>/extensions`；已有安装及停用、卸载状态保留。“设置 → 运行时管理”提供 `.cphhost` 导入、启停、卸载、挂载包重装和在线更新。原生入口复用核心安装状态，安装切换前在独立私有进程验证候选库、协议握手和 Lua VM 执行，失败保留现用版本。调试运行时包使用 `:app:packageLuaHost` 生成，输出在 `build/android/host-packages/`。

“设置 → 功能扩展中心”管理 `.cphext`，支持签名信任源、离线和在线目录。Android 核心的 HTTP、CLI 与动作接口不能修改原生运行时；完整 App 界面也只能从原生设置导入。连接桌面核心时，工作台按该核心的能力管理远端扩展与运行时。

Go 功能扩展由 APP 不导出的 `ExtensionService` 加载，最多同时运行 8 个私有进程；SDK 通过 JNI 接管 socket，与桌面共用动作和声明式存储。加载前检查受信签名、安装路径、Go 构建信息、文件摘要、ELF ABI 与 minSdk；停用、卸载和加载超时回收进程。CI 和正式扩展包只带 `arm64-v8a` 库，本地模拟器测试可额外构建 `x86_64`。安装 Lua 编辑器后，原生“已安装”页显示扩展声明的新建与编辑入口，编辑页由工作台沙箱承载。

## 工作台与设置

内置工作台首次使用时由用户设置账号和密码。远程工作台要求 HTTPS（loopback 除外），凭据由 Android Keystore 加密并绑定连接；切换连接会结束旧 WebView 和消息桥。插件管理始终操作手机本机。仅远程模式停止本机核心与系统任务并保留数据。

应用设置管理语言、主题、下载代理、通知、后台运行、运行时和外壳日志。外壳日志默认关闭，与核心请求日志独立；应用更新从官方稳定 Release 选择匹配 ABI 的 APK，由系统浏览器下载。系统通知需要 Android 通知权限；后台任务受系统调度和省电策略约束。

## 设备测试

安装匹配 ABI 的主包与测试 APK 后执行：

```sh
adb -s <serial> shell am instrument -w -e native fixture github.shadowbaby.clawproxyhub.core.test/github.shadowbaby.clawproxyhub.core.CoreInstrumentation
```

默认 instrumentation 也使用测试 APK 内的最小插件，检查签名篡改拒绝、加载、协议握手、启停、移除和重装。其他测试参数：

| 参数 | 内容 |
| --- | --- |
| `-e onboarding true` | 初始化、自选凭据、Lua 上传与重启持久化 |
| `-e workspace true` | Keystore、连接隔离、地址变更、仅远程模式与市场过滤 |
| `-e market true` | 只读检查市场中的 Lua 包及设备运行时状态，不初始化核心或修改用户设置 |
| `-e host_package true` | 显式安装 APP 内置 Lua Host，检查签名、候选 worker、安装状态和 Lua 市场；保留该安装，不改业务插件和系统设置 |
| `-e application true` | Lua 服务、任务、回调、取消、关闭与恢复 |
| `-e window true` | 安全区、原生导航、搜索、导入与侧栏 |
| `-e frontend true` | 测试 APK 自动携带使用同一 APP 证书签名的界面夹具，验证更新、篡改拒绝和回退 |
| `-e extension true` | 可选完整 Lua 编辑器夹具，验证签名安装、Service / JNI、源码动作、草稿、修订冲突、9 次启停、篡改拒绝及重启恢复 |

批量验证业务插件使用 `python android/smoke_plugins.py plugins/build/android/plugin-packages --adb <adb-path> --serial <serial> --abi x86_64`；测试包需与设备 ABI 一致。测试在临时目录执行并清理夹具，恢复安装偏好。

Lua 编辑器测试使用尚未安装编辑器的专用模拟器；全新模拟器首次运行时创建测试账号，已有账号不变，测试后移除夹具及其数据。先构建带额外测试 ABI 的扩展，再把包路径传给测试 APK：

```powershell
python scripts/build-packages.py --platform windows/amd64 --only lua-editor --out build/packages --development-key --android-test-abi x86_64
$editorVersion = python scripts/project_config.py get components.lua-editor.version
$editorPackage = (Resolve-Path "build/packages/lua-editor-$editorVersion.cphext").Path
./android/gradlew.bat -p android -PcphABI=x86_64 "-PcphExtensionTestPackage=$editorPackage" :app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintDebug
# 安装生成的主 APK 和测试 APK 后执行
adb -s emulator-5554 shell am instrument -w -e extension true github.shadowbaby.clawproxyhub.core.test/github.shadowbaby.clawproxyhub.core.CoreInstrumentation
```

测试包仅加入测试 APK；正式 App 仍不预装编辑器。主 APK 从 `build/packages/trust.json` 读取这次开发构建的受信身份。测试覆盖签名安装、JNI 动作、分块草稿及重启恢复、连续启停与篡改拒绝，并从原生贡献入口打开真实 WebView，验证恢复草稿后点击保存。
