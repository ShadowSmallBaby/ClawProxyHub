# ClawProxyHub

Claw 类客户端（LobsterAI / WorkBuddy 等）的统一管理反代网关：核心提供网关、路由、账号、分组、代理、密钥、任务调度与仪表盘，具体客户端实现以**插件**形式接入，插件源码与发布在独立仓库 [ClawProxyHubPlugins](https://github.com/ShadowSmallBaby/ClawProxyHubPlugins)。

同时提供 [Android 主 APP](android/README.md)。Lua Host 与 Lua 编辑器通过签名包安装，发行组合与可选扩展由 [build-config.json](build-config.json) 配置，详见[统一发行](internal/distribution/README.md)和[扩展打包](extensions/README.md)。[CLI](cmd/cphctl/README.md) 随 full ZIP 提供，用于连接已有核心进行命令行管理。

## 架构

```
客户端（Claude Code / Codex CLI / Cherry Studio ...）
   │  /v1/messages · /v1/chat/completions · /v1/responses
   ▼
┌─ ClawProxyHub 核心────────────────────────────────────────┐
│  网关：三协议归一化 → 统一信封（自动协议转换）               │
│  实例：插件 → 实例（站点地址 + 站点级配置）→ 账号 / 分组     │
│  路由：对外模型别名 → 分组（权重+真实模型映射）→ 账号        │
│       策略：round_robin / random / least_used / sticky       │
│  账号：多步登录 / 刷新 / 401 自动续期与换号 / 账号或分组代理 │
│  任务：interval / daily / once 调度（签到等维护任务）        │
│  存储：SQLite（golang-migrate 启动自动迁移）                 │
│  市场：多源索引 → 下载 .cphplugin → 校验 → 安装              │
│  运行时：Go 插件二进制 / Lua 插件（已安装 Lua Host 沙箱）      │
└────────────┬─────────────────────────────────────────────────┘
             │ hashicorp/go-plugin（子进程 gRPC，契约 protocol v2）
   ┌─────────┴─────────┐
   ▼                   ▼
 lobsterai 插件     autoclaw 插件    （← ClawProxyHubPlugins 仓库构建发布，Go/Lua 双运行时）
```

## 快速开始

### 本地运行

需要 Go、Node.js、pnpm、Python 3.11+ 和本机 C 编译器。组件版本与更新仓库统一配置在 [project.toml](project.toml)。

```bash
# 仪表盘（go:embed 嵌入，需先构建）
cd web && pnpm install && pnpm build && cd ..

go build -o cph ./cmd/cph
./cph
```

浏览器打开 `http://127.0.0.1:8080` → 首次进入引导页创建管理员账号 → 「插件」页从插件市场安装 lobsterai / workbuddy（离线环境可上传 `.cphplugin` 包）。

需要 Lua 插件和编辑器时，按[本地打包说明](extensions/lua-editor/README.md)准备签名包并配置 `CPH_PACKAGE_DIRS` 与 `CPH_EXTENSION_TRUST`。也可直接使用 Release 的 full 发行包，其中已携带所需包及信任配置，首次启动自动安装。

### Docker

```bash
docker compose up -d   # 管理密码在 docker-compose.yml 中配置
```

可用 `CPH_IMAGE=账号/镜像:v1.5.2` 选择 Docker Hub 版本。`docker compose up -d --build` 从本地源码构建；Release CI 的镜像直接复用同次构建的 Linux 发行包，配置见[统一发行](internal/distribution/README.md#docker-镜像)。

镜像使用 full 发行组合，携带 Web、Lua Host 和 Lua 编辑器包，默认首次启动自动安装。设置 `CPH_INSTALL_PACKAGES=false` 可改为从扩展中心手动安装；直接运行容器也可在镜像名后传入 `--install-packages=false`。开关不影响已有安装、停用或卸载状态。已安装的扩展、业务插件及配置在 `data/` 卷中持久化。

### 使用流程

1. **装插件**：「插件」→ 插件市场 → 安装（GitHub 不通时可在「插件源」加自建源，或上传离线包）
2. **（多实例插件）建实例**：「实例」→ 选插件 → 填站点地址与站点级配置（单例插件首次使用自动落默认实例）
3. **添加账号**：「账号」→ 添加 → 选择插件（多实例插件再选实例）与授权方式
   - lobsterai：浏览器 OAuth / 凭据文件导入
   - workbuddy：手机验证码 / 浏览器授权 / 凭据文件导入
   - newapi：API 密钥 / 密码 / 凭据文件
4. **建分组**：把同实例账号划入分组（分组 = 单实例账号池）
5. **（可选）绑代理**：账号或分组绑定出站代理（账号级优先），上游流量经代理
6. **建路由**：对外模型别名（如 `deepseek-flash`）→ 分组 + 真实模型 + 权重
7. **建密钥**：客户端调用凭据（明文只显示一次），可限定路由范围
8. **接入客户端**：

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer cph-xxxx" \
  -d '{"model":"deepseek-flash","messages":[{"role":"user","content":"你好"}]}'
```

Claude Code 等客户端把 base URL 指向 `http://127.0.0.1:8080`，任意协议入口自动转换。

## 配置（环境变量）

见 [.env.example](.env.example)。核心项：`CPH_ADDR`、`CPH_DATA_DIR`、`CPH_ADMIN_USERNAME/PASSWORD`（仅首启引导，之后以数据库为准）、`CPH_MARKETPLACE_URL`（自建市场索引）。

## 插件开发

插件是独立 Go 二进制，引用本仓库的 `sdk` 模块，实现 `pb.ClawPluginServer` 后一行启动：

```go
import "github.com/ShadowSmallBaby/ClawProxyHub/sdk"

func main() { sdk.Serve(&myPlugin{}) }
```

- 契约：`sdk/proto/cph.proto`（Handshake / Login 多步登录 / Refresh / ListModels / Chat 统一信封 / RunTask）
- 通用上游适配：`sdk/openaiup`（OpenAI 方言）、`sdk/anthropicup`（Anthropic 方言）、`sdk/responsesup`（Responses 方言）；SSE 分帧助手 `sdk/sse`
- 宿主回调（日志 / 存储 / 代理查询）：实现 `sdk.HostAware` 接收 `*sdk.Host`
- 参考实现：`examples/stub`（演示插件）与 [ClawProxyHubPlugins](https://github.com/ShadowSmallBaby/ClawProxyHubPlugins) 中的正式插件；**Lua 插件**通过已安装的 Lua Host 运行，无需编译脚本（见插件仓库 AGENTS.md §11）
- 包格式 `.cphplugin`（zip 容器）：含 `manifest.json`（name / version / author / protocol_version / icon）+ `plugin-<os>-<arch>[.exe]`（Lua 插件为平台无关的 `main.lua`）；打包器与发布流程见插件仓库

## 项目结构

```
cmd/cph          核心入口
internal/        网关 / 路由 / 账号 / 任务 / 插件管理 / 管理 API（database/migrations 为 SQL 迁移）
sdk/             插件开发工具包（契约生成代码 + 上游适配器 + SSE/传输层）
hosts/luahost    Lua 插件运行时（独立 module，签名 .cphhost）
examples/stub    演示插件（开发参照）
web/             仪表盘（Vue3 + TDesign，Web / Android 工作台）
extensions/      功能扩展（签名 .cphext）
```

## 社区

Linux DO: [学AI上L站](https://linux.do)

## 许可证

本项目基于 [AGPL-3.0](LICENSE) 协议开源。
