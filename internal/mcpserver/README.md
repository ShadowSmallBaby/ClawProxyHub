# CLI 与 MCP

MCP 在核心进程中运行，使用认证的 Streamable HTTP JSON 响应，地址为 `/admin/mcp`，协议版本 `2025-03-26`。支持 initialize、ping、tools/list、tools/call、取消和 DELETE 会话关闭，不提供 resources、prompts 或 SSE 推送。

## 管理与连接

在**系统设置 → MCP**中启用服务、生成密钥并选择工具。默认停止、没有密钥、不授权任何工具。当前仅管理员可以管理和连接，每位管理员只有一个独立 MCP 密钥。

客户端选择 Streamable HTTP，地址填写 `https://你的站点/admin/mcp`，请求头设置 `Authorization: Bearer <MCP 密钥>`。Android 本地核心监听 loopback，页面展示的地址只适用于手机本机，不直接对局域网开放。

密钥仅在生成或重置时显示一次，数据库只保存 SHA-256 摘要。重置和撤销立即使旧密钥失效；降级角色、删除用户或修改密码同样撤销访问。普通网页登录 Token 和 CLI 受限会话不能代替 MCP 密钥，MCP 密钥也不能用于管理 API。

配置存储于 SQLite，修改后立即生效，不再读取 `CPH_MCP_ACTIONS`。停用、重置/撤销密钥或调整授权会取消现有 MCP 请求；已由工具提交到任务引擎的异步任务需要另行取消。运行中请求每 5 秒复核管理员身份与认证版本。

工具授权按动作 ID 精确匹配，和注册器当前可用状态取交集。具有同一权限名称的其他动作不会被自动开放；扩展框架保留停用时撤销工具的机制；当前产品不启用扩展，也不注册扩展管理动作。GUI、CLI 与 MCP 共用参数检查、执行及 SQLite 审计。

MCP 在核心进程内运行，协议和管理逻辑集中在 `internal/mcpserver`，管理页提供服务启停。

## CLI

`cphctl` 连接已有后端，适合终端查询和自动化脚本。设置 `CPH_BACKEND=https://...`、`CPH_TOKEN` 或 `--token-file`，运行 `status`、`actions`、`describe ID`、`invoke ID JSON`；`invoke ID -` 从 stdin 读取 JSON。

管理员可 POST `/admin/tokens/scoped`，如 `{"scopes":["status.read","tasks.read"],"ttl_seconds":3600}`。该临时令牌仅访问动作 API，不能管理设置或签发令牌。`cli mcp` 是 stdio 到 HTTP 的桥接，使用独立 MCP 密钥作为 `CPH_TOKEN`；stdout 仅输出协议，诊断走 stderr。

核心不包含模型调用、AI 草稿生成或外部 MCP 客户端功能。

常用命令：

```sh
cphctl --url http://127.0.0.1:8080 --token-file ./admin-token status
cphctl --url http://127.0.0.1:8080 --token-file ./admin-token actions
cphctl --url http://127.0.0.1:8080 --token-file ./admin-token describe core.tasks.run
cphctl --url http://127.0.0.1:8080 --token-file ./admin-token invoke core.tasks.list
```

普通管理命令使用管理员登录令牌或受限动作令牌；`mcp` 子命令使用专用 MCP 密钥。产品关闭功能扩展入口，`install/enable/disable/uninstall` 返回不可用。仅使用网页或 Android APP 时无需运行 cphctl。

初始化成功返回 `Mcp-Session-Id`；后续请求和取消通知须携带该头，服务端将会话绑定到当前密钥。共享密钥的客户端分别初始化，各自的请求 ID 互不干扰。空闲 24 小时、核心重启或权限/密钥变更后需重新初始化；未知会话返回 404，缺少会话返回 400。客户端可 DELETE 同一端点结束会话并取消其中调用，CLI 桥自动转发并关闭会话；会话失效不会自动重放工具调用。
