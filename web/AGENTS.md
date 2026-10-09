# AGENTS.md — web

前端子项目上下文。根级 `AGENTS.md` 仍适用，本文件补充 web 专属约定。

## 技术栈

Vue 3 `<script setup>` + TypeScript + Vite + TDesign Vue Next（全量注册）+ vue-i18n（zh/en）+ pinia + echarts（Dashboard）。桌面核心将 `build-web/` 经 `go:embed` 嵌入 Go 二进制；App 工作台资源交给原生宿主，配置定义于根 `build-config.json`。

## 目录结构

```
src/
  main.ts            入口（TDesign 全量注册 + theme.css）
  App.vue
  api/               API 分域层（视图禁止直连 /admin）
    client.ts        仅核心请求器 + token 管理（JWT Bearer、401 跳登录）
    auth.ts          登录 / 当前用户 / 密码 / 通知 / 版本
    stats.ts         统计 / 趋势 / 积分
    logs.ts          请求日志（列表 / CSV 导出 / 清空）+ 运行日志（/admin/run-logs，展开明细抽屉 / 导出）
    entities.ts      插件 / 插件源 / 实例 / 账号 / 分组 / 代理 / 路由 / 密钥 / OAuth / 任务
                     （插件条目带 runtime 字段：空=Go 插件、"lua"=脚本插件，插件页渲染 Go/Lua 徽章）
    settings.ts      系统设置（网关/网络/日志/品牌）/ 备份恢复 / 系统信息
    extensions.ts    扩展包、运行时、动态设置、离线/在线目录与签名信任源
    types.ts         与后端 admin API 对齐的类型
  views/             按功能域归一，每域一个文件夹
    auth/            Login / Setup
    dashboard/ plugins/ instances/ accounts/ groups/
    proxies/ routes/ keys/ oauth/ tasks/ logs/ settings/ profile/
  layouts/           布局层
    AppLayout.vue    组装层（菜单配置 + /admin/me 用户信息）
    AppSidebar.vue   侧栏（logo + 菜单 + 收起，collapsed 自持）
    AppHeader.vue    头部（页面标题/描述 + 功能区）
    header/          功能按钮子组件：VersionChip / NotifBell / LangSwitch / ThemeSwitch / UserMenu
    types.ts         MenuItem 共享类型
  components/        通用组件
    index.ts         统一出口（PageHeader/EllipsisCell/EntityIcon/CTabs 等）
    base/            TDesign 二次封装：CButton/CCard/CTable/CTabs/CDialog（质感集中管理）
  composables/       hooks：useTheme/usePagination/useDialogVisible/useChart/useLocale/useAsync
  utils/             format（时间/数字）+ lookup（pluginLabelOf/instanceNameOf 等）+ common + dict + branding + logfmt
  i18n/              zh/en（menu/menuDesc/common/<域> 分组）
  assets/theme.css   明暗双主题变量（品牌蓝 #4c7dff 体系）+ 全局质感
  router/index.ts    懒加载路由 + guest RBAC 守卫
```

## 硬性约定

1. **API 调用走分域模块**：视图不写 `api.get('/admin/...')`；需鉴权头的下载/上传也在 API 层
2. **组件引用统一出口**：`components/index.ts`；改 TDesign 质感只改 `components/base/`，不散改视图
3. **复用优先**：格式化 `utils/format`、关联字段 `utils/lookup`、弹窗 `useDialogVisible`、加载态 `useAsync`、主题 `useTheme`、图表 `useChart`
4. **t-select 空值用 `undefined`**（0/null 显示成 "0"）
5. **样式**：遵循 `STYLE_PROMPTS.md` 双主题；禁止渐变背景/光晕/悬浮上浮/装饰动画；过渡 duration-200 仅颜色；圆角最大 12px
6. **新视图**：放 `views/<域>/`，页面用 `.page` class，页头用 `PageHeader`，表格/弹窗用 base 封装组件；路由加进 `router/index.ts` + `layouts/types.ts` 菜单
7. **注释**：中文、只讲目的、优先文件头一行

## 命令

```bash
npx vue-tsc --noEmit   # 类型检查（零错误才算完成）
npx vite build         # 构建（警告 chunk > 700kB 才提示，vendor 拆分见 vite.config.ts）
npx vite dev           # 开发（5173，代理 /admin /v1 /assets 到 127.0.0.1:8080）
```

## 构建产物 chunk 约定

支持 `web-full/app-full`，使用 `pnpm run build:profiles` 完成类型检查和两种产物构建。构建需 Python 3.11+ 读取根 `project.toml`。两者均保留网关、路由、账号和任务入口；Web 发行预配 Lua 编辑器，App 不预配。App 不携带业务插件管理页，插件操作由 Android 原生页负责。编辑器位于独立 `extensions/lua-editor`，主 Web 通过扩展贡献入口与沙箱桥调用，避免重复编入 CodeMirror。

`vite.config.ts` manualChunks：vue / tdesign / echarts / vendor 独立 chunk。改业务代码不应触碰这些 chunk 的哈希；新增大型第三方依赖时评估是否归入 vendor。
