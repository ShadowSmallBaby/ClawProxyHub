# 统一发行

`build-config.json` 定义桌面发行、前端资源、Android 预配和签名包。桌面只发布 `cph-<系统>-<架构>-full.zip`，Web 通过 `go:embed` 嵌入核心；网关、路由、账号和任务均由核心提供，可选能力通过扩展安装。

ZIP 的一级目录为 `ClawProxyHub/`：

```text
ClawProxyHub/
  cph[.exe]
  cli[.exe]
  distribution.lock.json
  README.md
  LICENSE
  data/packages/
    lua-runtime-<版本>.cphhost
    lua-editor-<版本>.cphext
```

前端提供 `web-full/app-full`，App 不预配 Lua 编辑器。独立前端仅发布 `app-<版本>.cphui`，Android 使用同一构建配置和签名包，构建方法见 [Android](../../android/README.md)。Android 核心使用 `cph_no_web` 构建，由原生宿主提供工作台界面。

## 本地构建

在主库根目录执行，需要 Go、Python 3.11+、Node.js、pnpm 和本机 C 编译器：

```sh
python scripts/build-release.py --platform windows/amd64 --development-key --out build/distributions
```

Linux/macOS 换成当前平台，如 `linux/amd64`、`darwin/arm64`。核心启用 CGO，跨平台构建还需相应 C 工具链；CI 使用各目标平台的原生 runner。`--prebuilt-web` 使用已经构建并校验的 `web/build-web` 资源；资源仍编入本次核心二进制。ZIP 输出路径不得已有同名文件。

`--development-key` 创建并复用 `.cache/development-signing/extension.key`，仅供本地开发。正式构建设置 `CPH_EXTENSION_PRIVATE_KEY`、`CPH_EXTENSION_KEY_ID`、`CPH_EXTENSION_TRUST_JSON`，或使用 `--key/--key-id/--trust` 提供签名身份。私钥不进入发行包或组件缓存。

`CPH_EXTENSION_TRUST_JSON` 是构建时的原始信任 JSON，包含发布者公钥、允许的包 ID 与权限，装配后写入 `distribution.lock.json`。`CPH_EXTENSION_TRUST` 是运行时额外信任文件的路径，默认 `<CPH_DATA_DIR>/extensions-trust.json`；管理员也可在扩展中心添加身份并持久化到数据库。直接 `go build` 不会把信任文件嵌入二进制。使用完整发行包或镜像时，已携带包的信任配置无需另建文件。

`scripts/build-packages.py --only frontend.app` 构建 App 资源并调用 Gradle 签名，要求 Android 构建环境；`--prebuilt-web` 复用已构建的资源。包 ID 为 `frontend.app`，权限为 `client.ui`，使用与 Android APP、Lua Host 及业务插件相同的 RSA 证书。功能扩展和桌面 Lua Host 使用 Ed25519，App 界面无需加入其信任配置。当前不单独发布 Web `.cphui`。

CLI 随 full ZIP 以 `cli[.exe]` 提供，解压后连接已有核心，命令用法见 [CLI](../../cmd/cphctl/README.md)。`.cphtool/.cphcore/.cphsdk` 不作为已启用的安装格式。

## 装配与启动

`scripts/build-packages.py` 独立生成签名组件。`scripts/build-release.py` 编译内嵌 Web 的核心与 CLI，再由 `scripts/assemble-release.py` 验证所选组件并装配 full ZIP。`cph-assemble --manifest request.json --out new-directory` 接受 profile、core、cli、version、platform、frontend、packages、trust 及可选的 readme/license 路径；输出目录必须不存在。

核心启动时发现可执行文件旁的 `distribution.lock.json`，也可使用 `--distribution` 指定；程序校验失败拒绝启动。发行目录须来自可信渠道，哈希不能代替来源认证。未设置 `CPH_DATA_DIR` 时，发行包的数据保存到发行目录中的 `data/`，不依赖启动工作目录；显式数据库和插件路径仍按原配置解析。

`data/packages/` 保存原始 `.cphhost/.cphext`，不是安装目录。核心默认通过统一安装管理器验签、检查平台和依赖、首次安装；Host 的版本文件位于 `data/hosts/`，功能扩展位于 `data/extensions/`，业务插件位于配置的插件目录。发行锁记录初始包清单，原始包在安装时校验，安装后移走原始包不影响核心启动。`CPH_INSTALL_PACKAGES=false` 或命令行 `--install-packages=false` 关闭自动安装，挂载目录仍可查看和手动安装。升级需显式安装，已有停用及卸载状态跨重启保留。

## 版本与更新

[project.toml](../../project.toml) 是组件版本的唯一配置入口，分别维护核心、Android APP、Lua Host、Lua 编辑器和前端版本。Go 直接构建会嵌入这份配置，Gradle 与 Python 构建脚本也读取同一文件。包源码清单保留兼容要求等元数据，构建时注入对应组件版本。Android APK 使用 `ClawProxyHub-<APP版本>-<versionCode>-android-<ABI>.apk` 命名；APK 内容变化后必须增加 versionCode，包括只更新所含核心的情况。

`release.yml` 在推送 `vX.Y.Z` tag 时自动发布。手动运行时可选择分支、留空 `tag` 并保持 `publish=false`，使用正式签名完成同一套全平台构建和清单校验，产物仅上传到 Actions 附件。需要补发时指定已有 tag，并设置 `publish=true`。所有任务使用解析后的同一源码提交；手动构建不创建 tag。

仅更新组件时，选择 `mode=components`，`tag` 指向已有 Release，`source_ref` 指向新的源码分支或提交，`components` 填写要构建的 ID（逗号分隔）。该模式要求目标已有 `update-manual.json`，只构建所选 `.cphhost/.cphext/.cphui` 并合并清单，保留核心 ZIP、APK、Docker、版本日志和 Release 正文。未变化的组件可复用缓存；内容变化须升对应组件版本，降级、同版本不同内容及不兼容目标核心的包会被拒绝。`publish=false` 可先验证合并结果；正式上传先完成组件附件，再替换清单。

根 [version.json](../../version.json) 保留旧桌面客户端读取的 `version`、`release_url`、`changelog`。其他平台只保存 `platform.android.changelog` 等日志，不重复维护版本。修改 TOML 版本后执行 `python scripts/project_config.py sync` 自动同步桌面兼容字段；CI 使用 `check` 检查同步状态。新增平台日志可使用 `platform.<平台>.changelog`，无需影响桌面字段。

CI 在全部产物构建完成后执行：

```sh
python scripts/update-manual.py --assets build --tag v1.5.2
```

生成的 `update-manual.json` 使用桌面顶层字段和 `platform.android` 应用字段，合并各自日志、版本、实际附件的下载地址、大小和 SHA-256；`artifacts` 按桌面系统/架构选择 full，Android 按 applicationId、versionCode、ABI、minSdk 选择。`packages`、`frontends` 描述独立附件。生成器核对 ZIP 根目录、全部文件摘要、发行锁、前端与 APK 构建元数据，拒绝混入旧版本产物。

桌面和 Android 均读取 `<更新仓库>/releases/latest/download/update-manual.json`。默认更新仓库在 TOML 的 `updates.repository` 配置，构建可用 `CPH_REPOSITORY_URL` 覆盖；GitHub Actions 自动使用当前仓库地址。Docker 构建可传入 `--build-arg CPH_REPOSITORY_URL=https://github.com/owner/repo`。项目主页按钮始终链接原项目，与更新仓库配置独立。

## 组件缓存

缓存位于 `.cache/cph-packages`，键包含组件源文件及实际本地 Go 依赖、注入组件版本后的清单、打包规则、目标平台、工具链与签名公钥身份。复用前校验摘要和签名，缓存缺失或损坏时重建。核心版本或源码变化不要求重编没有变化的 Lua Host 或编辑器。

Android 使用 Gradle 构建缓存；各原生任务通过入口、tags 和目标环境运行 `go list -deps` 自动获得实际源码、模块文件及嵌入资源，无需按库名维护文件白名单。缓存同时包含 ABI、Go/NDK 版本及编译参数，签名包任务包含发行证书指纹。APK 中的核心或资源变化仍会触发应用重新装配。

Release 附件目录统一由 `update-manual.json` 描述，供桌面更新检查、Android 更新器、扩展中心与构建校验使用。扩展中心读取其中的 `packages` 并持久化目录缓存，同时直接扫描挂载包提供离线目录；插件市场的 `index.json` 属于插件子库。

## Docker 镜像

本地 `Dockerfile` 从源码构建 full，自动使用独立的开发签名身份。`Dockerfile.release` 安装运行依赖并复制预构建发行包；CI 复用本次 `cph-linux-amd64-full.zip` 和 `cph-linux-arm64-full.zip`。上下文准备脚本按 `update-manual.json` 校验版本、平台和摘要，去掉 ZIP 的 `ClawProxyHub/` 外层，恢复核心与 CLI 的执行权限。CI 运行镜像使用与发行构建兼容的 Ubuntu 24.04。

镜像将携带的原始包放在 `/opt/clawproxyhub/packages`，与数据卷中的 `/app/data/packages` 一起交给统一安装器；这样挂载用户数据卷后仍能发现预配包。安装状态、Host、扩展和插件写入 `/app/data`。

在主库 Actions 配置以下三项即可启用 Docker Hub 任务：

| 类型 | 名称 | 值 |
| --- | --- | --- |
| Repository variable | `DOCKERHUB_IMAGE` | `账号或组织/镜像名`，不带 registry 和 tag |
| Repository secret | `DOCKERHUB_USERNAME` | Docker Hub 登录账号 |
| Repository secret | `DOCKERHUB_TOKEN` | 对目标镜像仓库有写权限的访问令牌 |

未设置 `DOCKERHUB_IMAGE` 时跳过镜像任务。手动 `publish=false` 会验证登录并构建两个架构，使用 BuildKit 缓存，不推送镜像。正式发布在 Release 附件上传成功后，先推送 `<镜像>:<Release tag>`（如 `v1.5.2`），成功后再将同一镜像摘要发布为 `<镜像>:latest`。两个标签均支持 `linux/amd64` 和 `linux/arm64`；手动补发也遵循此顺序。

使用 Compose 时可通过 `CPH_IMAGE` 选择上述镜像或固定版本。`CPH_INSTALL_PACKAGES` 默认 `true`，关闭后仍可在扩展中心手动安装镜像携带的包。
