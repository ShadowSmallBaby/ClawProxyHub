# 清单与持久化设置

字段以 [manifest.go](../../../sdk/extension/manifest.go)、[settings.go](../../../sdk/extension/settings.go) 为准，完整例子见 [Lua 编辑器清单](../../../extensions/lua-editor/manifest.json)。本页的 `C`、`E` 沿用技能入口的路径记号。

## 清单决策

| 字段 | 编写规则 |
| --- | --- |
| `id`、`name` | ID 稳定且唯一，最长 96 字符，匹配 `^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`；不能使用 `core`。`name` 为默认名称 |
| `label`、`desc` | 用 `{"zh":"…","en":"…"}` 提供展示名称与说明 |
| `version` | 稳定三段版本，不使用预发布后缀；主库从 `project.toml` 注入，源清单可以省略 |
| `api`、`core` | 当前 `api: 1`；按实际所用能力设置 `core.min`，不随意添加 `max_exclusive` |
| `kind`、`target`、`activation` | 普通页面使用 `frontend-sandbox/backend/hot`；Go 后端包使用 `service/backend/hot`；`target` 不填系统名 |
| `platforms` | 纯前端省略或空数组表示通用包；Go 后端按构建器生成的五平台矩阵填写 |
| `dependencies` | 以组件 ID 为键声明 `{min, max_exclusive?}`；所依赖组件须已启用、可用且版本兼容，不能有环 |
| `permissions`、`actions` | 只请求实际使用的权限；动作可别名核心动作、`host.storage.*`，或声明自身 Go 处理器及 schema |
| `pages`、`contributions` | 前者定义可加载页面，后者定义宿主入口；管理卡片不会自动列出所有页面 |
| `settings` | 声明宿主持久化配置，见下节 |
| `backend`、`storage` | Go 平台入口、后台权限和多表结构，见 [后端与存储](backend-storage.md) |
| `files` | 打包工具生成的资源路径到 SHA-256 映射，源模板可为 `{}`；不要手填摘要 |

`pages[].entry` 必须指向包内文件，最终存在于 `files` 中；路径使用 `/`，不含绝对路径、反斜杠、盘符或 `..`。打包时还会拒绝符号链接及不安全路径。Go 功能扩展使用 `execution: "trusted-process"` 和 `backend.entries`；顶层 `entry/android` 不复制 LuaHost 配置。未知清单字段被拒绝，安装或迁移命令不是支持的声明。

## 入口与动作片段

以下片段可合并到一个读取本地 Lua 插件源码的页面扩展清单；身份、兼容性等字段仍需按上表补全，产物需包含 `frontend/index.html`：

```json
{
  "permissions": ["workspace.read"],
  "pages": [
    {"id": "viewer", "title": "Source", "labels": {"zh": "源码", "en": "Source"}, "entry": "frontend/index.html"}
  ],
  "contributions": [
    {"id": "view", "location": "plugins.item.actions", "label": "View source", "labels": {"zh": "查看源码", "en": "View source"}, "page": "viewer", "when": "editable", "order": 20}
  ],
  "actions": [
    {"id": "read", "target": "core.workspace.read", "title": "Read source", "labels": {"zh": "读取源码", "en": "Read source"}}
  ]
}
```

当前注入位置仅有 `plugins.toolbar` 与 `plugins.item.actions`。后者激活时带插件 `name`；`when` 仅接受省略、空串或 `editable`，不支持任意表达式。贡献和动作的 ID 不能互相重复。`when` 控制展示条件，真正的操作资格仍由核心校验。

## 设置声明

设置在签名清单的 `settings` 数组内声明。下面是设置片段，宿主会提供设置入口和表单：

```json
{
  "settings": [
    {
      "id": "theme",
      "kind": "select",
      "label": {"zh": "主题", "en": "Theme"},
      "default": "system",
      "required": true,
      "options": [
        {"value": "system", "label": {"zh": "跟随工作台", "en": "Follow workspace"}},
        {"value": "light", "label": {"zh": "浅色", "en": "Light"}},
        {"value": "dark", "label": {"zh": "深色", "en": "Dark"}}
      ]
    },
    {"id": "compact", "kind": "toggle", "label": {"zh": "紧凑显示", "en": "Compact view"}, "default": false}
  ]
}
```

- 持久化设置支持 `text/textarea/select/toggle`，最多 24 项；不包含抽屉的 `image` 字段类型。
- `label` 和可选 `hint` 是语言对象。`default` 是持久化设置默认值；抽屉字段使用 `value`，两者不可互换。
- 文本可声明 `max_length`，省略或为 0 时默认 4096 字符，上限 16384。选择项为 `{value, label}`，最多 64 项。
- 值严格区分字符串和布尔值；`false` 是有效开关值。必填文本拒绝空白，必填选择项必须属于选项集合。
- `readonly` 必须有有效默认值；宿主拒绝改成其他值。设置 ID 唯一，不用 `constructor/prototype` 等保留标识。

## 使用与更新

从 `HostContext.settings` 读取本扩展的有效配置，同时处理初始化和 `cph:context` 更新。`context.dark` 始终表示工作台主题；专用内容可以根据自己的主题选项覆盖外观，宿主标准控件仍遵循工作台主题。不要只保留首次 `ready` 的配置快照。

核心按扩展 ID 将配置保存到 `extensions.config.<id>`，与包版本分离；升级、停用和卸载保留配置，显式清理数据才删除。当前清单未声明的历史字段和不再有效的值不会交付给扩展；有效配置回退到当前声明的默认值。持久化 JSON 上限 64 KiB，配置写入失败必须保留错误反馈。

Go 后端从 `Host.Initial.Settings` 读取经过宿主校验的启动配置快照。目前设置更新会推送给页面；后端需要停用后重新启用以读取新配置。

宿主设置接口供管理界面与管理客户端使用：

| 操作 | 契约 |
| --- | --- |
| `GET /admin/extensions/{id}/settings` | 返回 `{hash, fields, values}` |
| `PUT /admin/extensions/{id}/settings` | 发送 `{hash, values}`；更新提供的字段，校验合并后的有效值，拒绝未知字段、错误类型、只读修改和过期摘要 |
| `core.extensions.settings` | 管理动作，输入 `{id}` |
| `core.extensions.configure` | 管理动作，输入 `{id, hash, values}` |

这两个动作需要 `extensions.manage`，且不能由扩展声明为自身动作别名。普通扩展通过宿主设置入口配置、通过上下文读取，不为读取自身设置申请管理权限或直连管理 API。实现细节见 [设置管理器](../../../internal/extension/settings.go) 与 [管理接口](../../../internal/admin/extensions.go)。
