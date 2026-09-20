# Changelog

## v1.0.3

- Added 账号级出站代理：优先级 账号 > 分组（`account_proxies` 表），账号编辑弹窗可绑定
- Added 账号模型目录落库（`accounts.models_json`）：首次登录自动拉取，`?refresh=1` 手动同步，读取默认走库，以用户勾选为准
- Added 账号在线测试：选端点 / 模型 / 问题直调插件 Chat，绕过路由与密钥，输出响应与事件日志，不落 `request_logs`
- Added 密钥改名接口（`PUT /admin/keys/{id}`）与账号编辑弹窗（改名 / 分组 / 代理 / 模型）
- Added 进程内事件总线（`internal/event`）：任务成功完成后自动刷新该账号积分
- Added 前端通用可搜索绑定组件 `BindSelect`，统一账号 / 密钥 / 分组 / 路由的多选绑定
- Added 出站代理编辑（`PUT /admin/proxies/{id}`，密码留空保留原值）与连通性测试（`POST /admin/proxies/{id}/test`，经代理拨中立目标回时延，socks5 走 `x/net/proxy`）
- Added 核心版本机制（`internal/version`）与检查更新：`GET /admin/version` 对比仓库 `version.json`，侧栏底部显 `v1.0.3 · 有更新`
- Changed 管理后台路由按资源分组重构 + `authed` 注册器统一鉴权，杜绝逐条漏包

## v1.0.2

- Fixed Codex CLI 工具调用失败（表现为对话正常、涉及工具即上游 500）：上游流式工具调用首块带 `id`+`name`、续块 `id` 置空只带 `arguments`（`openaiup` 契约），而各协议编码器/聚合器按 `id` 索引，空 `id` 被误当新调用开出无名孤儿块，Codex 收到残缺 `function_call` 报 `failed to parse arguments: EOF`，坏历史回传再触发上游 500
- Fixed 出口层新增工具调用 `id` 补齐，一处覆盖 `responses` / `chat_completions` / `messages` × 流式/非流式 × 协议间转换（如 Anthropic 客户端 × OpenAI 上游同样受影响）
- Fixed Responses 流式收尾不完整：`response.completed` 缺 `output` 数组（正文只剩 preamble）、工具调用缺 `function_call_arguments.done` / `output_item.done`（工具永不执行）、`output_text.done` 文本为空
- Fixed 工具历史消息序列错位：相邻 `assistant` 与 `function_call` 未合并进同一条 `tool_calls`，并行调用时 `tool` 消息与声明它的 `assistant` 错位被上游拒绝；`arguments` 缺省补 `{}`
- Changed 不再透传 `reasoning.effort`（v1.0.1 引入）：Codex 的 `xhigh` 等私有值上游不认会 500，Responses 本无顶层 `reasoning_effort`
- Added `TestToolIDFillerAllEncoders`（含反证）、`TestResponsesSSEToolCallSplit`、工具历史合并回归测试

## v1.0.1

- Fixed Codex CLI（`/v1/responses`）请求失败（表现为 502 / 上游 500）：`extractText` 未识别 `input_text` / `output_text` 内容块，导致消息文本被静默丢空、上游收到空请求
- Fixed `developer` 角色（Responses / Chat Completions）被透传给只认 `system/user/assistant/tool` 的上游而被拒；现归一为 `system`
- Fixed Codex 流式中断 `missing field input_tokens`：`response.completed` 的 `usage` 缺失时补零值（`input_tokens` / `output_tokens` / `total_tokens`）
- Added 从 Responses 请求提取 `reasoning.effort` 并透传上游 `reasoning_effort`（此前被丢弃）
- Added `TestParseResponsesRequestCodex` 回归测试，复刻 Codex CLI 真实请求

## v1.0.0

- 首个正式版本：Claw 类客户端统一管理反代网关，单二进制核心 + 插件化实现
- 三协议归一化入口（`/v1/messages`、`/v1/chat/completions`、`/v1/responses`），任意入口 × 任意上游方言全矩阵转换
- 路由（模型别名 → 分组 → 账号，多策略）、401 刷新换号、4xx/5xx 降级、首事件超时
- 账号多步登录、AES-256-GCM 凭据加密、429/无积分自动暂停、分组级出站代理
- 任务调度（interval/daily/once）、插件市场（在线索引 + sha256 校验 + 离线兜底）
- Vue3 + TDesign 双语暗色仪表盘，Docker 部署，SQLite + golang-migrate 自动迁移
