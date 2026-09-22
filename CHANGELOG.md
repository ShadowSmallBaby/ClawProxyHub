# Changelog

## v1.1.3

后端功能版。插件启停持久化、客户端指纹注入、市场安装进度流，账号刷新健壮性修复，前端配套。

- Added 插件启停持久化：`Stop(name, persists)` 停用写 `enabled=0`，重启核心保持停止；`Resume` 清除持久化状态；`AutoStarts` 开机自启排除 `enabled=0` 插件（替代原 `Scan` + `syncPluginRecords` 兜底）
- Added `internal/fingerprint` 客户端指纹模块：`ClaudeHeaders`（Claude Code）/ `CodexHeaders`（Codex，`codex_meta` 用 uuid 生成会话标识）；`injectFingerprint` 按入口协议给 `ChatRequest.extra` 注入指纹头
- Added 市场安装进度流：`installZip` 走 NDJSON 进度流（下载分块上报 `received`/`total`，安装阶段回传 `phase`），`downloadToTemp` 带 context + 进度回调；前端 `requestStream` 读流，`OpProgressDialog` 展示 downloading/stopping/installing/starting
- Added 多语言品牌 label：`labelOf`/`brandName` 支持 `Manifest.Label` 回退
- Added 删除影响提醒：卸载返回 `DeleteImpact`，前端 `impact.ts` 弹窗列出需复核的路由/密钥引用
- Fixed 账号刷新健壮性：插件返回 `Unimplemented` 时给友好提示（API 密钥类账号无需刷新），不透出 gRPC 原始错误；`display_name` 仅在为空时写入插件值，避免刷新覆盖用户手动改名

## v1.1.2

SDK 增量版。新增 OpenAI Responses API 上游方言包，插件可直接反代 Responses 形态的上游（gpt / o 系官方端点、New API responses 转发等）。

- Added `sdk/responsesup`：与 `anthropicup` / `openaiup` 并列的第三个上游适配包，公共 API 对齐（`ChatBody` / `NewParser`）——system 归入 `instructions`、工具往返展开为 `function_call` / `function_call_output`、图片走 `input_image`；解析 `output_text` / reasoning 增量、`function_call` 首块与参数增量（done 只给全量时兜底补发）、`completed` / `incomplete`（`max_output_tokens` → `length`）/ `failed`；usage 按信封语义（input 剥离缓存读，带 cached / reasoning）
- Added SDK 常量 `ExtraFingerprintHeaders`：`ChatRequest.extra` 键，核心按入口协议生成的客户端指纹头（JSON map），插件按需采用

## v1.1.1

前端架构重构版。API 层分域、视图按域归一、布局与通用逻辑组件化、构建 vendor 拆分；界面遵循克制的明暗双主题。

- Refactor API 层分域（`web/src/api/`）：`auth` / `stats` / `logs` / `entities`（插件/实例/账号/分组/代理/路由/密钥/OAuth/任务 9 域）/ `settings` 五模块，`client.ts` 精简为核心请求器；各视图散落的 `api.get('/admin/...')` 全部收口
- Refactor 视图按域归一：`views/` 拆 14 个功能域子目录（git mv 保留历史），`Layout` 移至 `layouts/`
- Refactor 布局拆分：`AppLayout`（709→95 行）拆出 `AppSidebar` / `AppHeader` / `header/`（版本徽标、通知铃铛、语言、主题、用户菜单各按钮独立组件化，状态与副作用下沉子组件）
- Added 通用 hooks（`composables/`）：`useTheme` / `usePagination` / `useDialogVisible` / `useChart` / `useLocale` / `useAsync`（加载态 + 统一错误提示）
- Added 通用组件：`PageHeader` / `EllipsisCell` / `EntityIcon` / `GroupPicker`（Accounts 行内分组选择）；`utils/` 新增 `format` / `lookup` / `common` 去重（timeAgo / pluginLabelOf 等原本 3~4 份重复实现）
- Added TDesign 二次封装（`components/base/`）：`CButton` / `CCard` / `CTable` / `CTabs` / `CDialog` 质感集中管理，全部视图统一引用，调风格只改一处
- Changed 主题重构：按 STYLE_PROMPTS 克制风格——去除全站渐变背景/光斑（Dashboard 统计卡、Login 品牌区、品牌名渐变字）、卡片悬浮阴影与菜单图标光晕，过渡统一 duration-200 仅颜色
- Perf 构建优化：vite `manualChunks` 拆 vue / tdesign / echarts / vendor 独立 chunk，首屏入口 1512 kB → 44 kB，业务改动不影响 vendor 缓存命中
- Docs `web/STYLE_PROMPTS.md`：基于项目主题色的明/暗双模式风格提示词（明亮用品牌色驱动高亮，暗色仅深蓝灰面板系统）

## v1.1.0

大版本，含破坏性变更。引入实例层与插件契约 v2，插件市场改多源，新增 New API 插件。

- Breaking 插件契约 v2（`instance_id` / `account_id` / `instance_schema`，`ProtocolVersion` 1→2）；核心按 `[1,2]` 协商向后兼容 v1 插件，v1 插件仅默认实例
- Breaking 系统设置移除 `marketplace_url`，改由 `/admin/plugin-sources` 管理源列表（旧值导入为 `custom` 源）
- Breaking 分组归属实例（`groups.instance_id`），账号只能进同实例分组；新增 `PUT /admin/groups/{id}`；`POST /admin/accounts/login` 新增可选 `instance_id`
- Added 实例层（迁移 000005）：Plugin → Instance → Account，`instances` 表 + `accounts/groups.instance_id`；单例插件自动落默认实例，多实例插件由 `Manifest.capabilities:instances` + `instance_schema` 自声明；`/admin/instances` CRUD + 前端「实例」页
- Added 插件市场多源（`network.plugin_sources`）：按源懒加载、可达探测、每源计数、命名空间目录隔离、同名跨源拒装；前端「插件源」抽屉管理
- Added New API 插件（`plugins/newapi`，protocol v2 多实例）：api_key / password / cred_file / oauth 四种授权，余额折算与每日签到
- Added 站内通知（迁移 000006）：任务提醒落库，头部铃铛展示未读
- Added 调用日志缓存写入 token（迁移 000007，`cache_creation_tokens`）：区分缓存写入与读取，三协议 usage 信封统一带出
- Added 日志保留清理（`internal/janitor`，`logs.retention_days`）、CSV 导出（`GET /admin/logs/export`）与清空
- Added 系统信息 / 备份 / 恢复（`/admin/system/info|backup|restore`）：`VACUUM INTO` 快照 + `secret.key`，恢复重启换入
- Added 删除影响面预览与级联（`/admin/*/impact`）：预览受影响实例/分组/账号/规则/执行历史 + 引用路由与密钥名，事务内按层级级联
- Added 路由级 User-Agent（迁移 000008）+ 全局网关 UA / 浏览器 UA 设置
- Added 站点品牌自定义（`site.name/abbr/logo`，`GET /admin/branding` 免鉴权）
- Added 任务规则自动生成（迁移 000009，`task_rules.auto`）+ 规则去重 + `PUT /admin/task-rules/{id}` + 规则/执行历史分页
- Added 个人资料页（用户名 / 角色 / 可访问菜单，改密弹窗）；「设置」移至头像下拉，guest 可改自己密码
- Added 流式错误帧：上游中途失败按入口协议下发合规错误帧，不再静默断流
- Added 插件 `GetProxy` 支持 `account_id`：账号级代理优先，回退分组
- Changed 握手 `core_version` 改为真实版本号（原写死 0.1.0）

## v1.0.5

鉴权底座升级为 JWT + 角色访问控制；新增第三方平台凭证托管；日志页重构。

- Added JWT 鉴权 + RBAC：登录签发 HS256 JWT（claims 带 role/exp）替代每请求 bcrypt；`admin` 全量、`guest` 只读（隐藏系统设置、非 GET 拦 403）；`/admin/me` 按角色下发可见菜单，前端路由守卫 + 菜单过滤
- Added 第三方平台凭证托管（`oauth_credentials` 表，迁移 000003）：集中管理 LinuxDo / GitHub 等平台登录态，`TokenBlob` 经 AES-256-GCM 加密；`/admin/oauth-credentials` CRUD + 前端「凭证」管理页
- Added 日志页多条件搜索（密钥 / 模型 / 路由模糊 + 插件 / 协议 / 状态类下拉 + 日期区间）与分页（页大小 10 / 30 / 50 / 100 / 200）
- Added 日期区间快捷选项：今天 / 昨天 / 本周 / 上周 / 最近 7 天 / 最近 30 天
- Added 路由名列（`request_logs.route_name`，迁移 000004）：区分对外路由名与真实模型
- Fixed 日志延迟统计：网关统一 `startedAt` 计时源，杜绝「首字 3s 总耗时 300ms」的反常数据
- Fixed 版本检查误判：`GET /admin/version` 改用语义化比较（`compareSemver`），仅远端严格大于本机时才提示更新
- Changed 移除旧 Basic Auth 兼容（升级后需重新登录换取 JWT）；有更新时点击版本徽标先弹更新日志（中英双语）

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
