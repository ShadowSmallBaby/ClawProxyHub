# 前端构建与工作台

根 `build-config.json` 定义 Web 与 App 两种前端；`__CPH_PROFILE__` 标识当前构建。

| 模式 | 产物 | 界面与预配 |
| --- | --- | --- |
| `web-full` | `build-web/` | 同源 Web 工作台，内嵌到桌面核心，发行时预配 Lua 编辑器扩展 |
| `app-full` | `build-app/` | Android WebView 工作台，不预配 Lua 编辑器 |

所有模式均包含账号、实例、网关、路由、密钥、日志和任务。App 的业务插件管理由原生页提供。可选扩展以独立签名包随预配交付，CodeMirror 和编辑器实现不编入主前端。

```sh
pnpm build                 # web-full
pnpm build:app             # app-full
pnpm build:profiles        # 类型检查、Web/App 前端及模块清单
pnpm test
pnpm test:browser          # 需先准备 build/packages 的已签名编辑器与 trust.json
```

Vite 默认 `web-full`，可通过 `--mode app-full` 或 `CPH_WEB_PROFILE` 选择 App。`profile.json` 描述界面类型和预配包。桌面统一发布 full 并内嵌 Web；Android 核心排除 Web 资源，由 App 工作台连接。

独立前端只发布 `app-<前端版本>.cphui`。准备 Android 构建环境后，在主库根目录运行 `python scripts/build-packages.py --platform android/arm64 --only frontend.app --development-key --out build/packages` 构建并签名；已有 App 前端时可加 `--prebuilt-web`。Gradle 使用 APP 的 RSA 证书，Android 原生页直接校验并导入该包，后端扩展信任配置不能更换该证书。

## Android 与 Web

Android 原生页面负责插件、工作台、设置三 Tab、连接选择及应用设置。App 构建不包含 `Plugins.vue`、连接选择器或重复的原生外壳；扩展中心位于个人资料菜单，操作当前连接的后端。Vue 业务导航及账号、实例、任务配置继续复用。

连接 Android 核心时，扩展中心仅管理功能扩展，运行时与完整界面由原生设置管理；连接桌面核心时可管理其远端运行时。管理范围由后端声明并在安装接口校验。

连接目录和凭据由原生保存。受信页面通过 `workspace-state` 读取连接，通过 `workspace-token` 持久化登录状态；旧 localStorage 连接只用于一次迁移。切换连接结束旧 WebView 生命周期，取消请求和流，避免旧响应影响新工作台。页面不自行启动本机核心或切换连接。

PC 固定同源。App 的远程后端要求 HTTPS，loopback 可用 HTTP；后端配置精确来源 `CPH_ADMIN_ORIGINS=https://appassets.androidplatform.net`，不接受通配符或 null。鉴权和来源规则同样适用于 JSON、流、上传和下载。

## 扩展页面

[Lua 编辑器](../extensions/lua-editor/README.md)通过签名扩展贡献插件页入口；读取、保存和执行使用受控动作。iframe 不获得同源权限或 Token，停用后撤销页面与动作。Lua Host 尚未安装或被停用时仍可保存源码，执行需先启用运行时。

扩展中心显示已安装包、挂载目录和 Release 在线目录，安装或升级须确认权限。管理员可用唯一 code 和 JSON 配置信任身份，保存到数据库后立即生效；发行身份只读。构建和浏览器临时产物均不入库。
