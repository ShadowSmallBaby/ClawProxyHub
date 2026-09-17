# migrations — SQL 迁移（SQLite）

本目录是**编译期嵌入**的迁移源（`internal/database/database.go` 中 `//go:embed migrations/*.sql`），
启动时经 [golang-migrate](https://github.com/golang-migrate/migrate) 自动执行到最新版本。
模型定义（`internal/model/model.go`）与这里的 SQL **必须同步维护**：schema 变更只经迁移文件，
禁止 GORM AutoMigrate。

## 文件命名

严格 up/down 配对，序号递增，不可改动已有文件（已应用的迁移不会被重跑）：

```
000001_init.up.sql / 000001_init.down.sql
000002_<描述>.up.sql / 000002_<描述>.down.sql
...
```

- `*.up.sql`：升级脚本（必写）
- `*.down.sql`：回滚脚本（必写，对称还原 up 的变更）

## 新增迁移

1. 在本目录新建 `000NNN_<描述>.up.sql` 与对应 `.down.sql`（序号取现有最大值 +1）
2. 同步更新 `internal/model/model.go` 中对应实体的字段映射
3. 重新编译（`go:embed` 只在编译时打包，改文件不重编译不生效）
4. 启动验证：日志输出 `[database] migrated <old> -> <new>`

## 运行时行为

- 首次启动从零执行全部 up；存量库只执行新版本
- 迁移失败库会标记 dirty，服务拒绝启动；按提示 `cph migrate force <N>` 修复后重试

## 扩展其他数据库（PostgreSQL / MySQL ...）

迁移脚本与方言绑定（当前为 SQLite 方言：`AUTOINCREMENT` / `date('now',...)` 等）。
扩展时需要：

1. 将本目录拷贝为 `internal/database/migrations_<dialect>/`，改写为目标数据库方言
2. 在 `internal/database/database.go` 增加对应的嵌入源与 migrate driver（如
   `postgres.WithInstance`），按 `CPH_DSN` 的 scheme 分发
3. 模型层无需变动（GORM 映射与方言无关，字段类型按方言在 SQL 里控制）
