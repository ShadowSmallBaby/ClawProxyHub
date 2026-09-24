# AGENTS.md — ClawProxyHub

给 AI 编码代理的项目上下文。改代码前先读这份，遵循既有约定。

## 项目概述

ClawProxyHub：自托管的 Claude/OpenAI 兼容代理网关。核心职责：插件化管理上游账号 → 多实例（站点）维度 → 路由/分组/密钥 → 多协议转发（Anthropic/OpenAI/Responses）→ 调用日志与任务调度。

**双仓库结构**：主仓库（本仓库）+ 插件仓库（submodule，`plugins/`）。前端 `web/`，构建产物 `web/dist` 经 `go:embed` 嵌入二进制。

## 技术栈

- **后端**：Go（go-plugin 多进程插件、SQLite、chi 路由）
- **前端**：Vue 3 `<script setup>` + TypeScript + Vite + TDesign Vue Next + vue-i18n（zh/en）+ pinia
- **插件**：go-plugin RPC，契约 protocol v2（核心接受 v2 区间内协商）；两种运行时：**Go 插件**（编译二进制）与 **Lua 插件**（脚本，核心内置 LuaHost 沙箱，`hosts/luahost` 独立 module）

## 目录约定

```
internal/            核心后端（gateway/core/version/janitor/...）
hosts/luahost/       Lua 插件运行时（独立 module，gopher-lua 沙箱 VM）
web/
  src/api/           API 分域层：auth / stats / logs / entities / settings + client.ts（仅核心请求器）
  src/views/         视图按功能域归一：auth/dashboard/plugins/instances/accounts/groups/
                     proxies/routes/keys/oauth/tasks/logs/settings/profile/
  src/layouts/       布局：AppLayout（组装层）+ AppSidebar + AppHeader + header/（功能按钮子组件）
  src/components/    通用组件（PageHeader/EllipsisCell/EntityIcon）+ base/（TDesign 二次封装）
  src/composables/   通用 hooks：useTheme/usePagination/useDialogVisible/useChart/useLocale/useAsync
  src/utils/         format（时间/数字格式化）+ lookup（关联字段映射）+ common + dict + branding
  src/i18n/          zh/en 词条（menu/menuDesc/common/<域> 分组）
```

## 前端硬性约定

- **API 调用必须走分域模块**（`web/src/api/*`），视图里不直接写 `api.get('/admin/...')`
- **组件引用统一出口**：`components/index.ts`（通用）与 `components/base/`（TDesign 封装），调质感只改封装层，不散改视图
- **不重复造轮子**：时间格式化用 `utils/format`，关联字段显示用 `utils/lookup`，弹窗绑定用 `useDialogVisible`，加载态用 `useAsync`
- **t-select 空值用 `undefined`**（0/null 会显示成 "0"）
- **样式**：遵循 `web/STYLE_PROMPTS.md`（明暗双主题）；禁止渐变背景、光晕、悬浮上浮、装饰动画；过渡 duration-200 仅颜色
- **注释**：只讲目的，优先文件/方法头一行；与既有代码同语言（本项目注释为中文）

## 后端约定

- 请求日志/调试证据看 `request_logs` 表；改网关/核心后必须重编二进制（`go build`）
- 插件改动须重编插件二进制；插件仓库依赖核心 pseudo-version，proto 变更后同步升 go.mod
- 数据库迁移在 `internal/` 迁移文件序列，只增不改

## 流程

- 提交：develop 分支、显式 `git add`、内聚分组提交（conventional commit：feat/fix/refactor/chore(scope)）
- 发版：升 `internal/version/version.go` + `version.json` + `CHANGELOG.md` + `release/NOTES_v*.md`；tag 与 GitHub Release 手动
- 前端验证：`npx vue-tsc --noEmit` 零错误 + `npx vite build` 通过才算完成
