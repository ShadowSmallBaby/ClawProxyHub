-- 000009 — 任务规则来源标记：auto=1 系统按账号能力自动生成（编辑受限，触发类型锁定）；0 用户手动创建
ALTER TABLE task_rules ADD COLUMN auto INTEGER NOT NULL DEFAULT 0;
