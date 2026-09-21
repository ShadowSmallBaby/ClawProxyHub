-- 000005 — 实例层 + 插件安装来源

-- 实例：插件下的一个站点/部署（Plugin → Instance → Account）。
-- base_url 由核心固定提供；站点特有字段按 manifest.instance_schema 存 settings_json。
CREATE TABLE IF NOT EXISTS instances (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id     INTEGER NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    name          TEXT    NOT NULL DEFAULT '',
    base_url      TEXT    NOT NULL DEFAULT '',
    settings_json TEXT    NOT NULL DEFAULT '{}',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_instances_plugin ON instances(plugin_id);

-- 账号/分组强制归属实例：仅为已有账号或分组的插件建「默认」实例并回填（无数据的插件不预建，
-- 多实例插件由用户手动创建实例，单例插件首次使用时由核心自动落默认实例）
ALTER TABLE accounts ADD COLUMN instance_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE groups ADD COLUMN instance_id INTEGER NOT NULL DEFAULT 0;
INSERT INTO instances (plugin_id, name)
    SELECT id, '默认' FROM plugins
    WHERE id IN (SELECT plugin_id FROM accounts UNION SELECT plugin_id FROM groups)
      AND id NOT IN (SELECT plugin_id FROM instances);
UPDATE accounts SET instance_id = (
    SELECT MIN(i.id) FROM instances i WHERE i.plugin_id = accounts.plugin_id
) WHERE instance_id = 0;
CREATE INDEX IF NOT EXISTS idx_accounts_instance ON accounts(instance_id);

UPDATE groups SET instance_id = (
    SELECT MIN(i.id) FROM instances i WHERE i.plugin_id = groups.plugin_id
) WHERE instance_id = 0;
CREATE INDEX IF NOT EXISTS idx_groups_instance ON groups(instance_id);

-- 插件安装来源（插件源名；官方源为空 = 安装在根目录，其他源按源名建命名空间目录）
ALTER TABLE plugins ADD COLUMN source TEXT NOT NULL DEFAULT '';
