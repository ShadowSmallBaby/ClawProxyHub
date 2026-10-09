# Go 后端与业务存储

先看目标宿主的 [Go 协议](../../../sdk/extension/backend.md)、[存储契约](../../../sdk/extension/storage.md)，可运行例子是 [notes](../../../examples/extensions/notes/)。本文的 `C`、`E` 沿用技能入口。

## 选择动作实现

| 需要的能力 | 清单与实现 |
| --- | --- |
| 已有核心业务操作 | `actions[].target: "core.…"`，申请目标动作权限 |
| 页面直接提交受限存储请求 | `actions[].target: "host.storage.query/insert/update/delete/batch"`，声明表及读写权限 |
| 扩展自己的业务校验、组合或计算 | 不填 `target`，声明 `permission/effect/timeout_ms/input_schema/output_schema`，在 Go `Service.Actions` 注册同名处理器 |

Go 程序调用 `extension.Serve(Service)`；使用 `github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension`，不使用业务插件的 go-plugin/`sdk.ClawPluginServer`。处理器名称必须与清单中的后端动作完全对应，缺失或多出的处理器会导致握手失败并触发安装回退。

`stdout` 专用于宿主创建的双向 RPC 管道，诊断写 `stderr`。宿主处理超时、取消、停止与进程退出。不要另开前端直连的 HTTP 端口。

```go
"list": func(ctx context.Context, host *extension.Host, _ json.RawMessage) (any, error) {
    result, err := host.Storage(ctx, extension.StorageRequest{
        Op: "query", Table: "notes", Limit: 100,
    })
    return map[string]any{"notes": result.Rows}, err
},
```

存储回调沿用传入的 `ctx`，不通过保存调用上下文获得永久授权。宿主签发的调用 capability 在调用结束时撤销；当前动作的权限来自用户 scopes 与扩展声明的交集，`read` 动作的普通回调不获写权限。

后台存储需单独声明 `backend.background_permissions`，只支持已申请的 `storage.read/write`。它是扩展自身的后台授权，不是当前用户授权。`Service.Start` 收到服务生命周期上下文；安装提交前只允许读，不能在初始化时改业务数据。没有后台需求时省略该字段。

## 声明多表

设置项用 `settings`；多行记录用 `storage: {schema_version, tables}`。同一扩展可有多张表，不需要一个 SQLite 文件对应一张表或一个扩展。

- `cph.db`：主程序与扩展配置。`cph.ext.db`：所有功能扩展的业务表及宿主结构登记。
- 表名、字段、索引使用扩展内部逻辑名；物理命名空间与连接由宿主分配。
- 支持 `string/integer/number/boolean/datetime/bytes`，单主键、可空性、字面量默认值及索引。具体额度见存储契约和 `sdk/extension/storage.go`，不使用 SQL 类型片段或默认值表达式。
- `Host.Storage` 提交单个请求，`Host.Batch` 提交有界原子批次。`Query/Insert/Update/Delete` 用 `StorageRequest.Op` 表达；没有原始 SQL API。
- 只访问当前声明的自身表与字段。当前业务动作仅面向管理员及其授权的 scoped session；尚无按终端用户或站点自动附加行过滤的声明，不把扩展 ID 隔离当作用户隔离。
- 返回记录、生成 ID、影响行数或错误；不向扩展交付数据库路径、连接或 SQLite 系统表。

## 升级、失败与清理

声明改变时递增 `schema_version`。同一结构版本不能对应不同规范化内容；安全新增表、列和普通索引由宿主迁移。删除或改变现有列、收紧约束、修改或删除索引会被拒绝，不编写扩展自带迁移命令。

新版本不再需要整张表时，从声明省略该表。安装成功后宿主标记废弃并保留记录；安装失败恢复旧程序及原有废弃标记，失败候选的新表不会成为待清理的历史业务表。兼容的新增列可能留在物理结构中，旧版本仍受自身声明限制；不要描述为完整恢复数据库快照。

管理员在扩展中心预览并显式清理废弃表。管理动作是 `core.extensions.obsolete({id})` 与 `core.extensions.clean-obsolete({id,hash})`，后者使用预览时的包摘要，不能由扩展别名调用。清理只删除该扩展已废弃的表；依赖已清理表的历史结构不能重新绑定。

主动降级安装仍被拒绝。自动回退仅处理失败的安装切换；不要为实现失败回退放开降级检查。停用、升级、卸载保留业务数据；卸载后的“清理数据”删除该扩展全部业务表、设置与数据目录，其他扩展不受影响。

系统设置的备份与恢复包含 `cph.ext.db`，恢复在重启时应用。旧备份未包含扩展库时保留现有扩展数据。不要以覆盖整个共享库实现单扩展回退或清理。

## 验证重点

新增后端时构建并验签全部五个平台入口，在实际运行的平台验证页面调用与进程生命周期。新增存储时覆盖读写授权、同名表隔离、失败批次无部分提交、升级失败标记不变、成功后的废弃表清理及重启持久化。修改宿主安装或存储逻辑时补充适当的 `internal/extstore`、`internal/app` 回归；备份变化验证系统设置使用的两个 HTTP 端点及重启恢复。
