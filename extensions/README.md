# 功能扩展与运行时

功能扩展 `.cphext` 与运行时 `.cphhost` 共用[安装管理器](../internal/extension/README.md)，支持挂载发现、验签、安装、升级、启停和卸载。Lua 编辑器是独立扩展，Lua Host 是运行时包；主核心与 Web 不重复嵌入它们的实现。

## 元数据与包格式

源码根目录使用 `manifest.json` 模板，示例见 [Lua 编辑器](lua-editor/manifest.json)与 [Lua Host](../hosts/luahost/manifest.json)。组件版本在根 [project.toml](../project.toml) 维护，构建时注入完整清单；源码模板不重复保存版本号。字段定义以 [sdk/extension/manifest.go](../sdk/extension/manifest.go) 为准。

| 字段 | 用途 |
| --- | --- |
| `id`、`name`、`version` | 标识、默认名称、组件发行版本 |
| `label`、`desc` | 展示名称与说明，使用包含 `zh`、`en` 的语言对象 |
| `api`、`core` | 扩展 API 版本、最低核心版本 |
| `kind`、`target`、`activation` | 类型、安装位置和生效方式 |
| `permissions`、`dependencies` | 所需权限与组件版本依赖 |
| `pages`、`contributions`、`actions` | 页面、宿主入口和受控动作 |
| `platforms`、`execution`、`entry` | 操作系统/架构限制与执行入口 |
| `environments` | 支持的界面：`app`、`web-desktop`、`web-mobile` |
| `android` | Android 原生服务的 ABI、minSdk 和库路径 |
| `files` | 包内资源的 SHA-256，由打包器生成 |

兼容范围默认仅声明 `core.min`；省略 `max_exclusive` 表示没有预设上界，解析器保留可选上界支持。`version` 是组件版本，`api` 与包格式版本分别描述协议和文件结构。

`platforms` 为空表示通用包；`windows/amd64` 等限定桌面平台，`android/arm64` 等限定 Android 平台。`target` 表示安装在核心或客户端，与操作系统无关。包格式保持 v1 表示文件结构仍兼容，不随组件版本一起递增。

`environments` 省略时不限制界面。App 与 Web 按各自范围检查安装；Web 的桌面与手机界面都能安装和管理 Web 扩展，仅按对应声明显示功能入口与页面。切换断点不改变后端的安装、设置或启用状态。字段受签名保护，在线索引携带相同信息，安装预览以验签后的清单为准。

包使用 ZIP 容器，包含 `manifest.json`、`signature.json` 和清单列出的资源。桌面组件及通用前端扩展使用 Ed25519，Android 原生运行时使用与 APP 一致的 RSA 发行证书。签名覆盖原始清单，发布者身份和权限来自宿主的信任配置；包内证书不能自行建立信任。

业务插件 `.cphplugin` 使用独立契约，其平台清单和市场索引见[插件子库](../plugins/PACKAGING.md)。

扩展通过[公共 UI 协议](../sdk/extension/README.md)声明注入点、标准按钮与表单，由宿主渲染并回传操作事件。扩展不引用宿主组件；专用界面保留在独立沙箱中。清单中的 `contributions.labels` 提供注入入口的多语言文案，`label` 作为默认文案兼容旧包。

## 构建与使用

配置入口为根 [build-config.json](../build-config.json)。仅打包编辑器：

```sh
python scripts/build-packages.py --platform windows/amd64 --only lua-editor --out build/packages --development-key
```

省略 `--only` 构建已配置的桌面运行时和功能扩展；App `.cphui` 使用 `--only frontend.app` 单独选择。可重复传入 `--only` 选择多个包。编辑器本身不绑定操作系统；Android 原生 Lua Host 使用 `android/gradlew :app:packageLuaHost` 构建。正式签名及统一发行命令见[发行说明](../internal/distribution/README.md)。

开发时设置 `CPH_PACKAGE_DIRS=build/packages`、`CPH_EXTENSION_TRUST=build/packages/trust.json` 再启动核心；生产发行携带相应包和信任配置。挂载目录可只读，功能扩展安装到 `<data>/extensions`，运行时安装到 `<data>/hosts`。扩展中心以卡片合并展示内置包和已安装扩展，内置包带标识，未安装或卸载后仍保留卡片。没有可展示的包时显示暂无数据。已安装或卸载的组件不会因重启重复安装。`CPH_INSTALL_PACKAGES=false` 或 `--install-packages=false` 关闭首次自动安装，仍可手动安装挂载包。

手动上传或从目录安装须确认所需权限。同版本不同内容显示“更新”，更高版本显示“升级”；确认后重新验签并切换包，保留数据和启停状态，失败时恢复旧包。不支持降级，启动时不自动替换已有安装。完整的本地开发用法见 [Lua 编辑器](lua-editor/README.md)。

扩展中心可从 Release 的 `update-manual.json.packages` 获取在线目录，断网时使用数据库中的目录缓存或本地挂载包。第三方发布者的信任配置可在扩展中心按唯一 code 添加，code 对应包签名中的 `key_id`，不表示下载地址。

扩展中心提供“信任源”“扩展市场”“本地安装”三个入口。信任源表单接受一个签名身份的 JSON，形如 `{"key_id":"publisher-key","public_key":"<Base64 公钥>","publisher":"Publisher","ids":["example"],"permissions":[]}`；`key_id` 自动提取为 Code，也兼容 `trust.json` 中以签名 ID 为键的单身份映射。保存前检查格式、公钥或证书、发布者、扩展 ID 和权限字段，预置信任源只读。扩展市场打开时更新在线目录，无数据或无法访问时显示相应空状态；本地安装选择 `.cphext`，桌面还可选择 `.cphhost`，安装时须核对签名并授权所需权限。
