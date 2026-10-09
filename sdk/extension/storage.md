# 声明式业务存储

主程序继续使用 `<data>/cph.db`。所有功能扩展的业务表和宿主结构登记共用 `<data>/cph.ext.db`。简单配置仍使用清单 `settings`；业务记录用 `storage`，一个扩展可以声明多张表。

## 清单

```json
{
  "permissions": ["storage.read", "storage.write"],
  "storage": {
    "schema_version": 1,
    "tables": [{
      "name": "notes",
      "columns": [
        {"name": "id", "type": "string", "primary_key": true},
        {"name": "content", "type": "string", "default": ""},
        {"name": "created_at", "type": "datetime"}
      ],
      "indexes": [{"name": "by_created_at", "columns": ["created_at"]}]
    }]
  }
}
```

表、字段、索引名匹配 `^[a-z][a-z0-9_]{0,47}$`，保留 `sqlite_/cph_/extension_` 前缀及内部名称。宿主分配随机 namespace 和物理表名，在 `extension_storage_schemas` 保存结构、映射、版本历史、使用量及安装阶段。不同扩展可使用相同逻辑表名。

每表一个 `string/integer` 主键；主键不可空、不可设默认值或修改。插入时可提供主键；省略时宿主生成字符串 ID 或 SQLite 整数 ID。其他字段默认不可空，可设置 `nullable`；默认值只接受对应类型的字面量。

| 类型 | JSON 编码 |
| --- | --- |
| `string` | 字符串，拒绝 NUL |
| `integer` | JS 可精确表示的整数范围 ±9007199254740991 |
| `number` | 有限浮点数 |
| `boolean` | `true/false` |
| `datetime` | RFC3339 时间，宿主规范化为 UTC 和固定纳秒精度，文本排序等同时间排序 |
| `bytes` | 标准 Base64 字符串 |

索引支持 1–4 个已声明字段；新表可以声明唯一索引，已有表不能新增唯一约束。未知结构字段、SQL 类型片段、SQL 表达式、触发器、外键和迁移脚本均不开放。

## 请求

Go 处理器使用当前调用上下文：

```go
result, err := host.Storage(ctx, extension.StorageRequest{
    Op: "query", Table: "notes", Columns: []string{"id", "content"},
    Where: []extension.Predicate{{Column: "content", Op: "ne", Value: json.RawMessage(`""`)}},
    Order: []extension.Order{{Column: "created_at", Descending: true}},
    Limit: 50,
})
```

`Op` 为 `query/insert/update/delete`。写入通过 `Values: map[string]json.RawMessage`，所有值绑定 SQL 参数。条件最多 16 个，按 AND 合并；支持 `eq/ne/lt/lte/gt/gte/in/is_null/not_null`，`in` 为最多 100 个值，空值条件不携带 `value`。不接受任意表达式、连接或跨表查询。

查询可选择字段、最多 4 项排序及分页；默认 50 行、自动补主键排序。更新和删除必须带筛选条件，不接受查询选项；一次最多影响 200 行。`Host.Batch(ctx, []StorageRequest{...})` 在宿主事务中原子执行，任意请求失败则整批回滚。

单个 `StorageResult` 返回可选 `rows/id` 及 `affected`。无结果时 `rows` 可能省略，前端使用 `rows ?? []`。扩展不获取连接、数据库路径或 SQL 文本。

纯前端或 `data` 扩展也可声明业务表，通过动作别名使用存储，例如 `{"id":"list","target":"host.storage.query","title":"List"}`。页面用 `invoke('list', {table:'notes',limit:50})`；操作由别名确定，传入 `op` 时必须一致。单操作别名返回 `{results:[StorageResult]}`；`host.storage.batch` 接受 `{requests:[StorageRequest]}`，返回同一结构。

## 授权与额度

身份取自当前页面或进程会话，不能用请求字段选择其他扩展。只访问当前声明中的自身逻辑表与字段，宿主登记表、系统表、主库和废弃表都不可访问。权限来自用户 scopes 与扩展声明的交集；查询需要 `storage.read`，写入需要 `storage.write`。`read` 动作的普通 Go 回调无写权限；宿主 `batch` 动作别名自身需要写权限，批内查询还需要读权限。

Go 后台权限另行声明，见 [backend.md](backend.md)。调用结束或会话撤销后，保留的调用 capability 不能继续使用。当前管理动作面向管理员及其 scoped session；首版没有按终端用户或站点自动过滤行的声明。

| 资源 | 上限 |
| --- | --- |
| 当前声明的表 / 保留的物理表 | 每扩展 16 / 64 |
| 每表字段 / 索引 / 行数 | 64 / 16 / 10000 |
| 单行 JSON / 每扩展逻辑数据 | 64 KiB / 64 MiB |
| 单次返回或修改行数 / 批次请求数 | 200 / 32 |
| 请求或结果 | 2 MiB |
| 存储执行 / 迁移事务 | 5 秒 / 10 秒 |
| 共享库 / 结构历史 | 1 GiB / 每扩展 128 个结构版本 |

宿主用有界队列和单连接串行事务协调竞争；每扩展独立记账，保留表也计入其用量。WAL 通过检查点及日志保留限制回收，扩展不能持有事务或连接。

## 更新、废弃与清理

结构变化须递增 `schema_version`，同一版本必须对应相同规范化摘要。只自动新增表、安全新增列和普通索引；新增列必须可空或有字面量默认值。删除或变更现有列、索引或约束会被拒绝。

新版省略整张旧表时保留物理表和记录，成功安装后标记废弃。废弃标记基于最终提交的版本，不在候选结构准备时变更；失败回退保留原版本及原标记，失败候选未提交的新表被清除。兼容新增列等物理结构可能保留，旧版本仍只能访问其声明的字段。宿主不会恢复整个共享库来回滚单个扩展。

管理员在扩展中心“清理”中预览废弃表，再以预览包摘要提交清理。`core.extensions.obsolete` 输入 `{id}`，返回 `{hash,tables}`；`core.extensions.clean-obsolete` 输入 `{id,hash}`，拒绝过期摘要或仍被会话使用的表。清理只删除该扩展的已废弃表，依赖它们的历史结构不再允许绑定。主动降级安装仍被拒绝。

停用、升级和卸载保留数据；卸载后“清理数据”删除该扩展全部业务表、设置和数据目录，共享库及其他扩展保持可用。

系统设置的备份包含主库快照、扩展库快照及凭据密钥；扩展库快照同时包含业务记录与结构登记。恢复校验后暂存，重启前成组切换数据库和密钥；恢复中断通过日志回滚。旧备份没有 `cph.ext.db` 时保留现有扩展库。备份不包含扩展程序，需要保留对应签名包。
