# ClawProxyHub

Claw 类客户端（LobsterAI / WorkBuddy 等）的统一管理反代网关：核心提供网关、路由、账号、分组、代理、密钥、任务调度与仪表盘，具体客户端实现以**插件**形式接入，插件源码与发布在独立仓库 [ClawProxyHubPlugins](https://github.com/ShadowSmallBaby/ClawProxyHubPlugins)。

## 架构

```
客户端（Claude Code / Codex CLI / Cherry Studio ...）
   │  /v1/messages · /v1/chat/completions · /v1/responses
   ▼
┌─ ClawProxyHub 核心（单二进制，内嵌仪表盘）───────────────────┐
│  网关：三协议归一化 → 统一信封（自动协议转换）               │
│  路由：对外模型别名 → 分组（权重+真实模型映射）→ 账号        │
│       策略：round_robin / random / least_used / sticky       │
│  账号：多步登录 / 刷新 / 401 自动续期与换号 / 分组代理出站   │
│  任务：interval / daily / once 调度（签到等维护任务）        │
│  存储：SQLite（golang-migrate 启动自动迁移）                 │
│  市场：index.json 索引 → 下载 .cphplugin → 校验 → 安装       │
└────────────┬─────────────────────────────────────────────────┘
             │ hashicorp/go-plugin（子进程 gRPC）
   ┌─────────┴─────────┐
   ▼                   ▼
 lobsterai 插件     workbuddy 插件   （← ClawProxyHubPlugins 仓库构建发布）
```

## 快速开始

### 本地运行

```bash
# 仪表盘（go:embed 嵌入，需先构建）
cd web && pnpm install && pnpm build && cd ..

go build -o cph ./cmd/cph
./cph
```

浏览器打开 `http://127.0.0.1:8080` → 首次进入引导页创建管理员账号 → 「插件」页从插件市场安装 lobsterai / workbuddy（离线环境可上传 `.cphplugin` 包）。

### Docker

```bash
docker compose up -d   # 管理密码在 docker-compose.yml 中配置
```

镜像只含核心；插件在仪表盘安装后落在 `data/` 卷中持久化。

### 使用流程

1. **装插件**：「插件」→ 插件市场 → 安装（GitHub 不通时可在「设置」配置加速代理或自建市场地址）
2. **添加账号**：「账号」→ 添加 → 选择插件与授权方式
   - lobsterai：浏览器 OAuth / 凭据文件导入
   - workbuddy：手机验证码 / 浏览器授权 / 凭据文件导入
3. **建分组**：把账号划入某插件的分组（分组 = 单插件账号池）
4. **（可选）绑代理**：分组绑定出站代理，账号全部上游流量经代理
5. **建路由**：对外模型别名（如 `deepseek-flash`）→ 分组 + 真实模型 + 权重
6. **建密钥**：客户端调用凭据（明文只显示一次），可限定路由范围
7. **接入客户端**：

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
- 通用上游适配：`sdk/openaiup`（OpenAI 方言）、`sdk/anthropicup`（Anthropic 方言）
- 宿主回调（日志 / 存储 / 代理查询）：实现 `sdk.HostAware` 接收 `*sdk.Host`
- 参考实现：`examples/stub`（演示插件）与 [ClawProxyHubPlugins](https://github.com/ShadowSmallBaby/ClawProxyHubPlugins) 中的正式插件
- 包格式 `.cphplugin`：zip，含 `manifest.json`（name / version / author / protocol_version / icon）+ `plugin-<os>-<arch>[.exe]`；打包器与发布流程见插件仓库

## 项目结构

```
cmd/cph          核心入口
internal/        网关 / 路由 / 账号 / 任务 / 插件管理 / 管理 API（database/migrations 为 SQL 迁移）
sdk/             插件开发工具包（契约生成代码 + 上游适配器）
examples/stub    演示插件（开发参照）
web/             仪表盘（Vue3 + TDesign，go:embed 嵌入）
```

## 社区

Linux DO: [学AI上L站](https://linux.do)

## 许可证

本项目基于 [AGPL-3.0](LICENSE) 协议开源。
