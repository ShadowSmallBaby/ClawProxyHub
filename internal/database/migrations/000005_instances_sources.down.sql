-- 000005 回滚（索引在前，子列在前）
ALTER TABLE plugins DROP COLUMN source;
DROP INDEX IF EXISTS idx_groups_instance;
ALTER TABLE groups DROP COLUMN instance_id;
DROP INDEX IF EXISTS idx_accounts_instance;
ALTER TABLE accounts DROP COLUMN instance_id;
DROP INDEX IF EXISTS idx_instances_plugin;
DROP TABLE IF EXISTS instances;
