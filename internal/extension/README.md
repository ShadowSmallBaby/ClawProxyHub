# 扩展与运行时安装

`.cphext` 和 `.cphhost` 共用签名校验、依赖检查、版本规则和生命周期。包是 ZIP：`manifest.json`、`signature.json` 及逐项声明 SHA-256 的资源。Ed25519 或 Android RSA 签名覆盖原始清单字节；宿主限制签名者可发布的 ID、权限及原生执行资格。

## 挂载目录

`CPH_PACKAGE_DIRS` 配置一个或多个包目录，Linux/macOS 用冒号分隔，Windows 用分号分隔；缺省为 `<data>/packages`。桌面 ZIP 将原始包放在 `ClawProxyHub/data/packages/`；Android 将 APK asset 中的原始包复制到自己的挂载目录。目录可只读。

安装后的 `.cphhost` 保存到 `<data>/hosts/versions/<id>/<hash>/`，`.cphext` 保存到 `<data>/extensions/versions/<id>/<hash>/`；业务插件由插件管理器安装到配置的插件目录，默认 `<data>/plugins/`。Host 与功能扩展共用 `<data>/extensions/state.json` 和卸载收据。旧目录中的 Host 在验签后迁入 `hosts/`，保留启停状态。

启动默认按依赖顺序首次安装已受信且兼容的组件；`CPH_INSTALL_PACKAGES=false` 或 `--install-packages=false` 可关闭自动安装，目录清单和手动安装仍可用。已有安装照常加载，不自动升级或改变停用状态。卸载保存收据，避免原始包仍在挂载目录时被重新安装。无效、冲突或不兼容的可选包只在目录清单显示错误，不阻止核心启动。

`GET /admin/extensions/catalog` 返回扫描结果与安装状态。`POST /admin/extensions/install-mounted` 接受已扫描包的 SHA-256 和权限 grants，执行前重新验签，不接受任意服务器文件路径。上传安装仍通过短期票据及动作注册器授权和审计。

Android 的 `.cphhost` 由原生运行时页管理，复用本管理器的安装目录、依赖检查和状态；私有 JNI 入口授予运行时操作权限，HTTP 与公共动作接口只允许功能扩展。Android 扩展中心的已安装、挂载与在线目录均排除运行时；`.cphui` 由原生界面更新器管理。桌面扩展中心同时管理 `.cphext/.cphhost`。

扩展中心的在线目录来自配置的更新仓库 latest Release 中 `update-manual.json.packages`，按当前平台筛选。有效清单缓存在数据库，断网时仍可浏览；离线安装使用挂载目录或本地文件。在线下载逐项验证大小、SHA-256 和包签名，权限确认展示包内的真实清单，安装时再次验证短期票据。

## 信任与生命周期

额外信任根来自 `CPH_EXTENSION_TRUST` 指定文件，缺省 `<data>/extensions-trust.json`。格式为 `key_id → { public_key, publisher, ids, permissions, native }`；公钥为 Base64 Ed25519。Android 原生运行时的受信身份来自当前 APP 证书，不能仅凭包内证书通过安装。

安装第三方 `.cphext/.cphhost` 前，管理员在扩展中心添加信任身份：唯一 `code` 对应 `signature.key_id`，JSON 配置声明发布者公钥、允许的包 ID 与权限；原生运行时还需 `native: true`。这些配置持久化到数据库，保存后立即生效。撤销仍被启用扩展使用的身份或权限时，须先停用相关扩展。发行包、APP 证书及文件配置提供的身份只读，不能被数据库条目覆盖。

`extensions-trust.json` 保留为部署配置入口，在启动时加载；使用默认路径无需设置环境变量。Docker 可维护数据卷中的该文件，或挂载其他文件并将 `CPH_EXTENSION_TRUST` 指向容器内路径。包仍须满足当前平台、最低核心版本、依赖与执行器要求。Android Lua Host 和 App 界面包还必须使用 APP 认可的签名证书，增加 Ed25519 公钥不能绕过该要求；Go 功能扩展使用同一套 `.cphext` 签名身份与 `native` 授权。

此信任文件用于扩展与运行时，不控制业务 `.cphplugin` 的安装。构建时的 `CPH_EXTENSION_TRUST_JSON` 接受相同结构的 JSON 文本，用于验签并随发行锁分发；不包含私钥，也不会由普通 `go build` 自动嵌入核心。

安装验证有界快照并按摘要保存包；签名、权限、兼容性、依赖、激活或持久化失败时保留当前版本。同版本内容更新和更高版本升级均须手动确认，保留数据及启停状态；降级被拒绝。兼容范围默认只有最低版本，解析器支持可选的 `max_exclusive`。

运行时更新先独立验证候选包，再暂停正在使用它的插件。安装状态写入后恢复这些插件；安装或插件恢复失败时，先回滚运行时及安装状态，再恢复原有会话。原先停止的插件保持停止，其他运行时的插件不重启；更新期间的启停请求串行处理，避免覆盖用户操作。

热加载支持 `frontend-sandbox`、`data`、Go 功能扩展的 `service/trusted-process` 与 Lua Host 的 `trusted-process/android-service`。Lua Host 在安装切换、启动和重新启用前，通过临时脚本验证协议握手及 Lua VM 执行；Android 使用独立私有探测进程加载候选库，额外校验 ABI、minSdk、证书和 ELF。探测失败不切换现用版本。桌面原生进程具有当前用户权限。其他执行器无法激活时明确拒绝；`restart/system-install` 保留等待状态。停用运行时会停止正在使用它的 Lua 插件，失败时恢复原状态和会话；启用的依赖扩展及计划任务需先停用。卸载仍要求先停止依赖插件。

## Go 功能扩展

Go 功能扩展使用 `service/trusted-process` 执行器，共用一个 `.cphext` 的公共前端和五个桌面入口；声明 Android 时额外包含 ARM64 库，本地测试可加入 x86_64。宿主验签并检查入口的 Go 平台、构建模式与 CGO 设置：桌面使用无 CGO 的可执行文件和私有管道，Android 使用 NDK 原生库、私有 Service / JNI 与 socket，二者共用握手、动作和存储协议。初始化失败回退原版本，停用和卸载终止进程并撤销会话。原生执行需要 `service.execute` 与签名身份的 `native: true`；私有进程不表示操作系统权限沙箱。开发契约见 [Go 扩展 SDK](../../sdk/extension/backend.md)。

## 扩展设置

签名清单的 `settings` 声明设置项，宿主使用统一控件渲染、校验和保存。支持文本、多行文本、选择和开关，字段具有中英文名称、提示、默认值及只读约束。实际配置保存在系统设置表的 `extensions.config.<id>`，独立于包版本；升级、停用和卸载保留配置，卸载后的显式数据清理才删除配置和数据目录。

`GET /admin/extensions/{id}/settings` 返回声明、当前有效值和包摘要，`PUT` 使用 `{hash, values}` 保存。服务端拒绝未知字段、类型错误、只读修改和已经过期的包摘要。未声明或不再适用的历史值不会交付给扩展。CLI/MCP 可使用 `core.extensions.settings/configure`，受 `extensions.manage` 权限约束。Android 运行时设置仅经原生私有入口管理。

Lua Host 的启用状态是 Lua 运行能力的唯一开关，隔离运行当前固定开启。旧系统 Lua 开关只在升级启动时迁移：已安装运行时保留关闭选择；尚未安装的运行时不会自动预装，之后可显式安装。系统设置不再提供独立 Lua 开关。

卸载删除版本代码，保留用户数据；清理缓存与清理数据是独立操作。安装、资源交付和原生入口加载均重新校验文件。生命周期回调可以读取管理器状态，不能递归修改生命周期；失败时须清理部分注册。

## 业务表与清理

主程序使用 `cph.db`，所有功能扩展的业务表共用 `<data>/cph.ext.db`。一个扩展可以声明多张表，宿主保存结构版本、表归属与用量，统一迁移和执行结构化读写；扩展不持有 SQL、连接或数据库路径。设置继续保存在主库，原有数据目录保持独立。实现位于 `internal/extstore`，其宿主登记迁移使用独立递增序列；[存储契约](../../sdk/extension/storage.md) 说明类型、权限和额度。

成功安装后才根据新清单更新废弃表标记。新版本省略的旧表保留记录，失败切换恢复旧程序及原有标记；失败候选新建但未提交的表不进入废弃列表。兼容的新增列或索引可能保留，旧版本访问仍受自己的声明限制。安装状态与扩展库通过阶段登记协调，启动按已提交的 `state.json` 恢复，不回滚整个共享库。

扩展中心“清理”支持预览并删除废弃表，调用 `core.extensions.obsolete({id})` 和 `core.extensions.clean-obsolete({id,hash})`。清理在安装锁内复核包摘要并保护活跃表，只删除该扩展已登记的废弃表；清理后依赖它们的旧结构无法绑定。主动降级安装仍被拒绝。

升级、停用和卸载保留业务数据；卸载后的显式“清理数据”同时清理该扩展业务表、设置和数据目录。共享库文件及其他扩展的数据不删除。系统设置的备份导出同时生成主库和扩展库快照；导入校验后在重启前成组恢复，中断时回滚。旧备份不含扩展库时保留现有扩展数据。

## 前端与动作

GUI、CLI 与 MCP 共用动作注册器，校验 schema、用户权限、扩展 grants、超时和取消，并写入 SQLite 审计。扩展可以贡献已授权核心动作、`host.storage.*` 的别名，或声明自身 Go 后端动作。不能别名其他扩展动作及 `extensions.manage` 管理动作。停用先注销动作及取消调用，再完成生命周期变更。

`/extension-assets/{id}/{hash}/{path}` 只交付已签名代码。iframe 仅允许脚本，不授予同源权限；桥校验来源、随机 nonce、消息体积、并发数和动作白名单，Token 不进入消息。代码资源允许匿名 CORS 供沙箱加载，管理 API 保持鉴权与精确来源策略。停用后停止资源交付并撤销页面会话。

工作区 `scaffold/read/create/save/reload` 只操作本地插件的 `main.lua`。保存不执行或下载运行时，执行由独立动作触发。

运行 `go test ./internal/extension ./internal/extstore ./internal/app ./internal/database ./sdk/extension` 检查包校验、依赖、结构、状态与备份恢复；前端集成见 [Lua 编辑器](../../extensions/lua-editor/README.md) 与 [原生 JS + Go 笔记](../../examples/extensions/notes/README.md)。
